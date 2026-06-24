// Package service — Telegram Bot 交互命令服务。
//
// 处理通过 Telegram Bot API 接收的用户命令，提供系统状态查询、
// 媒体搜索、下载管理等功能。同时支持 Webhook 和 Long Polling 两种模式。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// TelegramUpdate 是 Telegram Bot API 推送的 update 对象。
type TelegramUpdate struct {
	UpdateID      int                    `json:"update_id"`
	Message       *TelegramMessage       `json:"message,omitempty"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query,omitempty"`
}

// TelegramMessage 是 Telegram 消息对象。
type TelegramMessage struct {
	MessageID int          `json:"message_id"`
	From      TelegramUser `json:"from"`
	Chat      TelegramChat `json:"chat"`
	Text      string       `json:"text,omitempty"`
	Date      int          `json:"date"`
}

type TelegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    TelegramUser     `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
}

// TelegramUser 是 Telegram 用户对象。
type TelegramUser struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

// TelegramChat 是 Telegram 聊天对象。
type TelegramChat struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
}

type telegramCommandReply struct {
	Text    string
	Buttons [][]telegramInlineButton
}

type telegramInlineButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

// TelegramBotService 处理 Telegram Bot 的交互命令。
type TelegramBotService struct {
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService
	auth   *AuthService
	device *DeviceService
	backup *BackupService

	pollingMu     sync.Mutex
	pollingCancel map[string]context.CancelFunc // bot_token -> cancel

	pendingMu sync.Mutex
	pending   map[int64]pendingInput // telegram_user_id -> awaited text input
}

// pendingInput tracks a button-initiated action that awaits the user's next
// text message (e.g. tapping「注册」then sending "用户名 密码").
type pendingInput struct {
	Kind      string // register / redeem_register / redeem_renew / setname / setpass / openreg_limit / gencode_user
	CreatedAt time.Time
}

// SetDeviceService wires the device-management service used by the device
// menu (list / kick) and enforcement notifications.
func (s *TelegramBotService) SetDeviceService(d *DeviceService) { s.device = d }

// SetBackupService wires database backup/restore commands.
func (s *TelegramBotService) SetBackupService(b *BackupService) { s.backup = b }

// NotifyUserByID sends a Telegram message to the local user identified by
// userID, resolved through their Telegram binding. Used by enforcement to warn
// users before destructive actions. No-op when the user has no binding.
func (s *TelegramBotService) NotifyUserByID(ctx context.Context, userID, text string) {
	if userID == "" || strings.TrimSpace(text) == "" {
		return
	}
	var binding model.TelegramBinding
	if err := s.repo.DB.WithContext(ctx).Where("user_id = ?", userID).First(&binding).Error; err != nil {
		return
	}
	targetChatID := telegramPrivateChatIDFromBinding(binding)
	if targetChatID == 0 {
		return
	}
	channel := s.findChannelByChatID(ctx, int(binding.ChatID))
	if channel == nil {
		channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
		if err != nil || len(channels) == 0 {
			return
		}
		channel = &channels[0]
	}
	_ = s.reply(ctx, channel, int(targetChatID), telegramCommandReply{Text: text})
}

// NewTelegramBotService 创建 Telegram Bot 服务。
func NewTelegramBotService(log *zap.Logger, repo *repository.Container, crypto *CryptoService, auth *AuthService) *TelegramBotService {
	return &TelegramBotService{
		log:           log,
		repo:          repo,
		crypto:        crypto,
		auth:          auth,
		pollingCancel: make(map[string]context.CancelFunc),
		pending:       make(map[int64]pendingInput),
	}
}

// TelegramRegistrationSettingKey 控制普通用户是否可以通过 Bot 注册新账号。
// 默认关闭，只有管理员在系统设置 / Bot 管理命令中显式开启后才允许注册。
const TelegramRegistrationSettingKey = "telegram.registration_enabled"

var errTelegramAccountAlreadyBound = errors.New("该媒体账号已绑定其他 Telegram，请联系管理员解绑")

// registrationEnabled 读取注册开关；默认关闭。
func (s *TelegramBotService) registrationEnabled(ctx context.Context) bool {
	v, err := s.repo.Setting.Get(ctx, TelegramRegistrationSettingKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

// setRegistrationEnabled 持久化注册开关。
func (s *TelegramBotService) setRegistrationEnabled(ctx context.Context, enabled bool) error {
	return s.repo.Setting.Set(ctx, TelegramRegistrationSettingKey, strconv.FormatBool(enabled))
}

// HandleWebhook 处理 Telegram 推送的 Webhook/Polling 消息。
func (s *TelegramBotService) HandleWebhook(ctx context.Context, body []byte) error {
	var update TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		return fmt.Errorf("invalid update: %w", err)
	}
	return s.handleTelegramUpdate(ctx, update, nil)
}

func (s *TelegramBotService) handleTelegramUpdate(ctx context.Context, update TelegramUpdate, channelHint *model.NotifyChannel) error {
	if update.CallbackQuery != nil {
		return s.handleCallback(ctx, update.CallbackQuery, channelHint)
	}

	if update.Message == nil || update.Message.Text == "" {
		return nil
	}

	msg := update.Message
	text := strings.TrimSpace(msg.Text)

	// Button-initiated text prompts (register / redeem / change name·password /
	// open-reg limit) arrive as ordinary messages. Consume them here before the
	// command gate so the button-driven menu can collect free-form input.
	if !telegramIsCommandText(text) {
		if msg.Chat.Type == "" || msg.Chat.Type == "private" {
			if channel := s.channelForMessage(ctx, msg, channelHint); channel != nil {
				if reply, handled := s.handlePendingText(ctx, channel, msg, text); handled {
					if reply.Text != "" {
						if err := s.reply(ctx, channel, msg.Chat.ID, reply); err != nil {
							s.log.Error("reply failed", zap.Error(err))
						}
					}
					s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
					return nil
				}
				if looksLikeRedemptionCode(text) {
					reply := s.cmdRedeem(ctx, channel, msg, []string{text})
					if reply.Text != "" {
						if err := s.reply(ctx, channel, msg.Chat.ID, reply); err != nil {
							s.log.Error("reply failed", zap.Error(err))
						}
					}
					s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
					return nil
				}
			}
		}
		return nil
	}
	if msg.Chat.Type != "" && msg.Chat.Type != "private" && !telegramSupportedCommand(telegramCommandName(text)) {
		return nil
	}

	s.log.Info("telegram command received",
		zap.Int("chat_id", msg.Chat.ID),
		zap.String("user", msg.From.Username),
		zap.String("text", text),
	)

	// 获取该消息可使用的 Telegram 通知渠道配置。群组/频道消息必须来自
	// 已配置的群组/频道；私聊消息会选择一个可验证该用户成员身份的 Bot。
	channel := s.channelForMessage(ctx, msg, channelHint)
	if channel == nil {
		s.log.Warn("telegram channel not allowed or not configured",
			zap.Int("chat_id", msg.Chat.ID),
			zap.String("chat_type", msg.Chat.Type),
			zap.Int("telegram_user_id", msg.From.ID),
		)
		return nil
	}

	// 解析并执行命令
	reply, err := s.executeCommand(ctx, channel, msg, text)
	if err != nil {
		s.log.Error("command failed", zap.Error(err))
		_ = s.replyForMessage(ctx, channel, msg, telegramCommandReply{Text: "命令执行失败: " + err.Error()})
		s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
		return nil
	}

	if reply.Text != "" {
		if err := s.replyForMessage(ctx, channel, msg, reply); err != nil {
			s.log.Error("reply failed", zap.Error(err))
		}
		s.deleteTelegramSourceMessage(channel, msg.Chat.ID, msg.MessageID)
	}

	return nil
}

func telegramIsCommandText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/") && telegramCommandName(text) != ""
}

func telegramCommandName(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return ""
	}
	cmd := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(cmd, "/") {
		return ""
	}
	if at := strings.Index(cmd, "@"); at > 0 {
		cmd = cmd[:at]
	}
	return cmd
}

func telegramIsGroupChat(chatType string) bool {
	return chatType != "" && chatType != "private"
}

func telegramPrivateMessageForUser(msg *TelegramMessage) *TelegramMessage {
	if msg == nil || !telegramIsGroupChat(msg.Chat.Type) {
		return msg
	}
	copied := *msg
	copied.Chat = TelegramChat{ID: msg.From.ID, Type: "private"}
	return &copied
}

func telegramGroupPrivateAdminHint() string {
	return "群组内不展示管理面板；管理员可在已绑定群组直接发送文本管理命令，涉及账号凭据的操作仍请私聊 Bot。"
}

func telegramGroupPrivateUserHint(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "此操作"
	}
	return action + "包含账号凭据或敏感信息，请私聊 Bot 操作；群组内仅开放账号状态、签到、设备与成人目录开关。"
}

func telegramGroupPrivateDeliverySentHint() string {
	return "已把你的 Bot 面板/执行结果私聊发送给你。若没收到，请先私聊 Bot 发送 <code>/start</code>。"
}

func telegramGroupPrivateDeliveryFailedHint() string {
	return "无法私聊发送给你。请先打开 Bot 私聊窗口发送 <code>/start</code>，再回群里使用命令。"
}

// cmdStart 处理 /start 命令。
func (s *TelegramBotService) cmdStart(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	name := msg.From.FirstName
	if msg.From.Username != "" {
		name = "@" + msg.From.Username
	}
	if telegramIsGroupChat(msg.Chat.Type) && len(args) > 0 {
		return telegramCommandReply{Text: telegramGroupPrivateUserHint("绑定账号")}
	}
	if len(args) == 0 {
		if binding := s.telegramBinding(ctx, msg.From.ID); binding != nil {
			user, _ := s.repo.User.FindByID(ctx, binding.UserID)
			if user == nil {
				_ = s.repo.DB.WithContext(ctx).Unscoped().Delete(&model.TelegramBinding{}, "id = ?", binding.ID).Error
				return telegramCommandReply{Text: "之前绑定的媒体中心账号已不存在，请重新绑定：\n<code>/start 用户名 密码</code>"}
			}
			status := "未隐藏"
			if user.HideAdult {
				status = "已隐藏"
			}
			return telegramCommandReply{
				Text: fmt.Sprintf("<b>MediaStationGo 已绑定</b>\n\n你好 %s，当前账号：<b>%s</b>\n成人目录：<b>%s</b>", name, userNameOrFallback(user), status),
				Buttons: [][]telegramInlineButton{{{
					Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[user.HideAdult],
					Data: "adult_toggle",
				}}},
			}
		}
		hint := "如果没有账号，请联系管理员注册。"
		if s.openRegEnabled(ctx) {
			hint = "如果还没有账号，可直接注册：\n<code>/register 用户名 密码</code>\n或：<code>/register 用户名-密码</code>"
		}
		return telegramCommandReply{Text: "<b>欢迎使用 MediaStationGo</b>\n\n普通用户请先绑定账号：\n<code>/start 用户名 密码</code>\n或：<code>/start 用户名-密码</code>\n\n" + hint}
	}
	channel := s.findChannelForMessage(ctx, msg)
	if dec := s.telegramUserBindDecision(ctx, channel, msg.From.ID); dec != bindAllowed {
		return telegramCommandReply{Text: telegramBindRejectText(dec, "绑定媒体中心账号")}
	}
	username, password := parseStartCredentials(args)
	if username == "" || password == "" {
		return telegramCommandReply{Text: "绑定格式不正确，请使用：\n<code>/start 用户名 密码</code>\n或：<code>/start 用户名-密码</code>"}
	}
	existingBinding := s.telegramBinding(ctx, msg.From.ID)
	user, err := s.repo.User.FindByUsername(ctx, username)
	if err != nil || user == nil {
		if existingBinding != nil {
			_ = s.unbindTelegramUser(ctx, msg.From.ID)
			return telegramCommandReply{Text: "当前绑定的媒体账号信息已失效，已自动解绑。请使用新的用户名和密码重新绑定。"}
		}
		return telegramCommandReply{Text: "未找到此用户，请联系管理员注册。"}
	}
	if !user.IsActive {
		return telegramCommandReply{Text: "此账号已被禁用，请联系管理员。"}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		if existingBinding != nil && existingBinding.UserID == user.ID {
			_ = s.unbindTelegramUser(ctx, msg.From.ID)
			return telegramCommandReply{Text: "当前绑定账号的密码已失效，已自动解绑。请使用新密码重新绑定。"}
		}
		return telegramCommandReply{Text: "账号或密码错误。"}
	}
	if err := s.upsertTelegramBinding(ctx, msg, user.ID); err != nil {
		return telegramCommandReply{Text: "绑定失败：" + err.Error()}
	}
	return telegramCommandReply{
		Text: fmt.Sprintf("绑定成功：<b>%s</b>\n\n普通用户只能使用此 Bot 管理自己的成人目录隐藏状态；系统状态、搜索、下载和统计命令仅管理员可用。", user.Username),
		Buttons: [][]telegramInlineButton{{{
			Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[user.HideAdult],
			Data: "adult_toggle",
		}}},
	}
}

// cmdRegister 处理 /register 命令：在管理员开启注册后，普通用户可通过 Bot
// 注册一个新的媒体中心账号，并自动绑定到当前 Telegram 账号。
func (s *TelegramBotService) cmdRegister(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) == 1 && looksLikeRedemptionCode(args[0]) {
		return s.redeemRegisterFlow(ctx, channel, msg, args[0])
	}
	if !s.openRegEnabled(ctx) {
		return telegramCommandReply{Text: "注册功能未开放，请联系管理员开启后再试。"}
	}
	// 开注名额已用尽则拦截（容量随凭证授权实时变化，名额单独计数）。
	if c := s.loadCapacity(ctx); c.Remaining() <= 0 {
		return telegramCommandReply{Text: "注册名额已满，请等待管理员重新开放或扩容授权。"}
	}
	if s.auth == nil {
		return telegramCommandReply{Text: "注册功能暂不可用，请联系管理员。"}
	}
	if channel == nil {
		channel = s.findChannelForMessage(ctx, msg)
	}
	if dec := s.telegramUserBindDecision(ctx, channel, msg.From.ID); dec != bindAllowed {
		return telegramCommandReply{Text: telegramBindRejectText(dec, "注册账号")}
	}
	if binding := s.telegramBinding(ctx, msg.From.ID); binding != nil {
		if user, _ := s.repo.User.FindByID(ctx, binding.UserID); user != nil {
			return telegramCommandReply{Text: fmt.Sprintf("当前 Telegram 已绑定账号：<b>%s</b>，无需重复注册。\n如需切换账号请使用 <code>/start 用户名 密码</code>。", userNameOrFallback(user))}
		}
	}
	username, password := parseStartCredentials(args)
	if username == "" || password == "" {
		return telegramCommandReply{Text: "注册格式不正确，请使用：\n<code>/register 用户名 密码</code>\n或：<code>/register 用户名-密码</code>"}
	}
	user, _, err := s.auth.Register(ctx, username, password)
	if err != nil {
		switch {
		case errors.Is(err, ErrUsernameTaken):
			return telegramCommandReply{Text: "该用户名已被占用，请换一个；如果是你本人的账号，请改用 <code>/start 用户名 密码</code> 绑定。"}
		case errors.Is(err, ErrUserLimitReached):
			return telegramCommandReply{Text: "注册失败：已达到用户数量上限，请联系管理员。"}
		default:
			return telegramCommandReply{Text: "注册失败：" + err.Error()}
		}
	}
	// 注册成功，扣减一个开注名额（名额用尽自动关闭注册）。
	s.consumeOpenRegSlot(ctx)
	if err := s.upsertTelegramBinding(ctx, msg, user.ID); err != nil {
		return telegramCommandReply{Text: fmt.Sprintf("账号 <b>%s</b> 注册成功，但自动绑定失败：%s\n请稍后使用 <code>/start %s 密码</code> 重新绑定。", user.Username, err.Error(), user.Username)}
	}
	return telegramCommandReply{
		Text: fmt.Sprintf("注册并绑定成功：<b>%s</b>\n\n你现在可以用此账号登录网页与第三方客户端。普通用户只能在此 Bot 管理成人目录显隐；其他功能仅管理员可用。", user.Username),
		Buttons: [][]telegramInlineButton{{{
			Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[user.HideAdult],
			Data: "adult_toggle",
		}}},
	}
}

// cmdRegistrationToggle handles /registration and /openreg. It uses the same
// quota-aware open-registration state as the inline Bot menu.
func (s *TelegramBotService) cmdRegistrationToggle(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 || strings.EqualFold(strings.TrimSpace(args[0]), "status") {
		c := s.loadCapacity(ctx)
		state := "已关闭"
		if c.OpenRegOn {
			if c.OpenRegLimit > 0 {
				state = fmt.Sprintf("已开启（%d/%d 名额）", c.OpenRegUsed, c.OpenRegLimit)
			} else {
				state = "已开启（不限名额，受授权上限约束）"
			}
		}
		return telegramCommandReply{Text: fmt.Sprintf("普通用户 Bot 注册功能当前<b>%s</b>。\n剩余可注册：<b>%d</b> 人。\n\n开启：<code>/registration on 10</code>\n不限：<code>/registration on 0</code>\n关闭：<code>/registration off</code>", state, c.Remaining())}
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on", "true", "1", "open", "enable", "enabled", "开启", "打开", "开":
		limit := 0
		if len(args) > 1 {
			n, err := strconv.Atoi(strings.TrimSpace(args[1]))
			if err != nil || n < 0 {
				return telegramCommandReply{Text: "名额必须是非负整数，0 表示不限名额。"}
			}
			limit = n
		}
		if err := s.openRegistration(ctx, limit); err != nil {
			return telegramCommandReply{Text: "开启失败：" + err.Error()}
		}
		label := "不限名额"
		if limit > 0 {
			label = fmt.Sprintf("%d 个名额", limit)
		}
		return telegramCommandReply{Text: "普通用户 Bot 注册功能已开启：" + label + "。"}
	case "off", "false", "0", "close", "disable", "disabled", "关闭", "关":
		if err := s.closeRegistration(ctx); err != nil {
			return telegramCommandReply{Text: "关闭失败：" + err.Error()}
		}
		return telegramCommandReply{Text: "普通用户 Bot 注册功能已关闭。"}
	default:
		return telegramCommandReply{Text: "参数无效，请使用 <code>/registration on [名额]</code> 或 <code>/registration off</code>。"}
	}
}

// cmdHelp 处理 /help 命令。
func (s *TelegramBotService) cmdHelp(ctx context.Context, msg *TelegramMessage) string {
	channel := s.findChannelForMessage(ctx, msg)
	if telegramIsGroupChat(msg.Chat.Type) {
		adminHint := ""
		if s.telegramUserIsAdmin(ctx, channel, msg.From.ID) {
			adminHint = "\n\n管理员可在已绑定群组直接发送文本管理命令；管理面板和账号凭据操作请私聊 Bot。"
		}
		return "<b>MediaStationGo 群组可用命令</b>\n\n" +
			"<b>/menu</b> — 打开群组自助菜单\n" +
			"<b>/account</b> — 查看账号状态\n" +
			"<b>/signin</b> — 签到\n" +
			"<b>/devices</b> — 查看登录设备\n" +
			"<b>/kick all|编号</b> — 踢下线设备\n" +
			"<b>/hideadult on|off</b> — 隐藏或显示成人目录\n\n" +
			"绑定、注册、兑换、改名、改密等包含敏感信息的操作请私聊 Bot。" +
			adminHint
	}
	if !s.telegramUserIsAdmin(ctx, channel, msg.From.ID) {
		register := ""
		if s.openRegEnabled(ctx) {
			register = "<b>/register 用户名 密码</b> — 注册新账号\n"
		}
		return "<b>MediaStationGo 用户命令</b>\n\n" +
			register +
			"<b>/start 用户名 密码</b> — 绑定账号\n" +
			"<b>/account</b> — 查看账号状态\n" +
			"<b>/signin</b> — 签到\n" +
			"<b>/devices</b> — 查看登录设备\n" +
			"<b>/kick all|编号</b> — 踢下线设备\n" +
			"<b>/setname 当前密码 新用户名</b> — 修改用户名\n" +
			"<b>/setpass 当前密码 新密码</b> — 修改密码\n" +
			"<b>/redeem 兑换码</b> — 注册或续期兑换\n" +
			"<b>/hideadult on|off</b> — 隐藏或显示成人目录\n\n" +
			"系统状态、搜索、下载列表与统计命令仅管理员可用。"
	}
	return "<b>MediaStationGo 命令列表</b>\n\n" +
		"<b>/start</b> — 开始使用\n" +
		"<b>/help</b> — 帮助信息\n" +
		"<b>/account</b> / <b>/devices</b> / <b>/kick all|编号</b> — 用户自助设备管理\n" +
		"<b>/signin</b> / <b>/redeem 兑换码</b> — 签到与兑换\n" +
		"<b>/setname 当前密码 新用户名</b> / <b>/setpass 当前密码 新密码</b> — 用户自助改名改密\n" +
		"<b>/register 用户名 密码</b> — 注册新账号（需管理员开启）\n" +
		"<b>/registration on [名额]|off</b> — 开启/关闭普通用户注册（管理员）\n" +
		"<b>/capacity</b> / <b>/users</b> — 容量与用户管理（管理员）\n" +
		"<b>/gencode register|renew 天数 [有效天数]</b> — 生成兑换码（管理员）\n" +
		"<b>/renew_user 用户名 天数</b> / <b>/delete_user 用户名 confirm</b> — 续期/删除用户（管理员）\n" +
		"<b>/unbind 用户1 用户2</b> — 批量解绑 Telegram 绑定（管理员）\n" +
		"<b>/unbind_duplicates</b> / <b>/unbind_inactive 天数</b> — 清理重复/无效绑定或久未登录绑定（管理员）\n" +
		"<b>/antishare on play=3 login=3 warn=2</b> — 防共享策略（管理员）\n" +
		"<b>/cleanup run</b> — 预览保号清理候选（管理员）\n" +
		"<b>/cleanup run confirm</b> — 确认清理候选账号（管理员）\n" +
		"<b>/cleanup on|off</b> — 保号规则开关（管理员）\n" +
		"<b>/cleanup_rule list|add|edit|修改|del|enable|disable</b> — Mgo 保号规则（管理员）\n" +
		"<b>/ban 用户名</b> / <b>/unban 用户名</b> — 禁用/解禁用户（管理员）\n" +
		"<b>/hideadult on|off</b> — 隐藏/显示当前绑定账号的成人目录\n" +
		"<b>/status</b> — 系统运行状态\n" +
		"<b>/search 关键词</b> — 搜索媒体库\n" +
		"<b>/downloads</b> — 下载列表\n" +
		"<b>/stats</b> — 媒体库统计\n\n" +
		telegramMgoAdminCommandHelp() + "\n\n" +
		"<b>自动推送事件：</b>\n" +
		"• 订阅命中新资源\n" +
		"• 下载任务完成\n" +
		"• 刮削失败告警\n" +
		"• 系统异常通知"
}

func telegramMgoAdminCommandHelp() string {
	return "<b>Mgo 管理命令（管理员可用，已注册到命令栏）：</b>\n" +
		"用户：<code>/ucr 用户名 密码 [天数]</code> 创建账号；<code>/uinfo 用户名</code> 查询账号；<code>/rmemby 用户名 confirm</code> 删除账号；<code>/only_rm_record tg:ID|用户名</code> 仅删 Bot 绑定；<code>/renewall 天数 confirm</code> 批量续期。\n" +
		"审计：<code>/userip 用户名</code> 查用户 IP；<code>/auditip IP</code> 按 IP 审计；<code>/auditdevice 关键词</code> 按终端设备审计；<code>/auditclient 关键词</code> 按客户端审计；<code>/udeviceid 设备ID</code> 按设备指纹审计。\n" +
		"清理：<code>/syncunbound</code> 检查未绑定账号；<code>/syncgroupm</code> 校验群成员；<code>/check_ex</code> 检查过期账号；<code>/deleted</code> 按保号规则预览清理候选。\n" +
		"权限：<code>/embyadmin 用户名 on|off</code> 设置管理员；<code>/banall confirm</code>/<code>/unbanall confirm</code> 批量禁用/解禁；<code>/prouser 用户名</code>/<code>/revuser 用户名</code> 管理保护名单；<code>/embylibs_blockall</code>/<code>/embylibs_unblockall</code> 批量禁用/开放媒体库权限。\n" +
		"运维：<code>/proadmin TelegramID</code>/<code>/revadmin TelegramID</code> 管理 Bot 管理员；<code>/backup_db</code> 备份数据库；<code>/restore_from_db 文件名 confirm</code> 恢复数据库。\n" +
		"说明：重复别名如 <code>/low_activity</code>、<code>/urm</code> 仍可兼容识别，但不显示在命令栏。"
}

// cmdStatus 处理 /status 命令。
func (s *TelegramBotService) cmdHideAdult(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	channel := s.findChannelForMessage(ctx, msg)
	if dec := s.telegramUserBindDecision(ctx, channel, msg.From.ID); dec != bindAllowed {
		return telegramCommandReply{Text: telegramBindRejectText(dec, "使用成人目录隐藏开关")}
	}
	binding := s.telegramBinding(ctx, msg.From.ID)
	if binding == nil {
		return telegramCommandReply{Text: "请先绑定账号：<code>/start 用户名 密码</code>"}
	}
	user, err := s.repo.User.FindByID(ctx, binding.UserID)
	if err != nil || user == nil {
		return telegramCommandReply{Text: "绑定用户不存在，请重新 /start 绑定。"}
	}
	next := true
	if len(args) > 0 {
		switch strings.ToLower(strings.TrimSpace(args[0])) {
		case "off", "false", "0", "show", "显示", "关闭":
			next = false
		case "on", "true", "1", "hide", "隐藏", "开启":
			next = true
		default:
			next = !user.HideAdult
		}
	} else {
		next = !user.HideAdult
	}
	if err := s.repo.User.UpdateFields(ctx, user.ID, map[string]any{"hide_adult": next}); err != nil {
		return telegramCommandReply{Text: "更新失败：" + err.Error()}
	}
	status := map[bool]string{true: "已隐藏", false: "已显示"}[next]
	return telegramCommandReply{
		Text: "成人目录" + status + "。此设置会同步影响网页与第三方客户端。",
		Buttons: [][]telegramInlineButton{{{
			Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[next],
			Data: "adult_toggle",
		}}},
	}
}

// ── Webhook Management ──

// SetWebhook 注册 Telegram Bot Webhook URL。
func (s *TelegramBotService) SetWebhook(ctx context.Context, botToken, webhookURL string) error {
	cfg := map[string]string{"bot_token": botToken}
	if err := registerTelegramBotCommands(ctx, cfg); err != nil && s.log != nil {
		s.log.Warn("telegram setMyCommands failed", zap.Error(sanitizeTelegramError(err)))
	}
	payload := map[string]interface{}{
		"url":             webhookURL,
		"allowed_updates": []string{"message", "callback_query"},
	}
	return telegramPostJSON(ctx, cfg, "setWebhook", payload, 15*time.Second)
}

// GetWebhookInfo 获取 Webhook 配置信息。
func (s *TelegramBotService) GetWebhookInfo(ctx context.Context, botToken string) (map[string]interface{}, error) {
	cfg := map[string]string{"bot_token": botToken}
	var result map[string]interface{}
	if err := telegramGetJSONDecode(ctx, cfg, "getWebhookInfo", 10*time.Second, &result); err != nil {
		return nil, err
	}
	return result, nil
}
