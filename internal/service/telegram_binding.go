package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// findChannelByChatID 根据 chat_id 查找已配置的通知渠道。
func (s *TelegramBotService) findChannelByChatID(ctx context.Context, chatID int) *model.NotifyChannel {
	channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
	if err != nil {
		return nil
	}
	target := strconv.Itoa(chatID)
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		configStr := ch.Config
		if s.crypto != nil && configStr != "" {
			configStr = s.crypto.Decrypt(configStr)
		}
		var cfg map[string]string
		if err := json.Unmarshal([]byte(configStr), &cfg); err != nil {
			continue
		}
		if cfg["chat_id"] == target || cfg["command_chat_id"] == target ||
			cfg["group_chat_id"] == target || cfg["channel_chat_id"] == target {
			return &ch
		}
	}
	if len(channels) == 1 && channels[0].Enabled {
		return &channels[0]
	}
	return nil
}

func (s *TelegramBotService) findChannelForMessage(ctx context.Context, msg *TelegramMessage) *model.NotifyChannel {
	if msg == nil {
		return nil
	}
	if msg.Chat.Type != "" && msg.Chat.Type != "private" {
		return s.findChannelByChatID(ctx, msg.Chat.ID)
	}
	channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
	if err != nil {
		return nil
	}
	var first *model.NotifyChannel
	for i := range channels {
		ch := channels[i]
		if !ch.Enabled {
			continue
		}
		if first == nil {
			first = &ch
		}
		if s.telegramUserIsAdmin(ctx, &ch, msg.From.ID) || s.telegramUserCanBind(ctx, &ch, msg.From.ID) {
			return &ch
		}
	}
	return first
}

func (s *TelegramBotService) channelForMessage(ctx context.Context, msg *TelegramMessage, hint *model.NotifyChannel) *model.NotifyChannel {
	if hint == nil {
		return s.findChannelForMessage(ctx, msg)
	}
	if msg == nil {
		return hint
	}
	if msg.Chat.Type != "" && msg.Chat.Type != "private" && !s.telegramChatAllowed(hint, msg.Chat.ID) {
		return nil
	}
	return hint
}

func (s *TelegramBotService) handleCallback(ctx context.Context, cb *TelegramCallbackQuery, channelHint *model.NotifyChannel) error {
	if cb == nil || cb.Message == nil {
		return nil
	}
	msg := *cb.Message
	msg.From = cb.From
	channel := s.channelForMessage(ctx, &msg, channelHint)
	if channel == nil {
		channel = s.findChannelByChatID(ctx, cb.Message.Chat.ID)
	}
	// 立即应答回调，关闭按钮上的加载状态，避免客户端长时间转圈。
	if telegramIsGroupChat(cb.Message.Chat.Type) {
		s.answerCallbackWithText(ctx, channel, cb.ID, "为了隐私，群组内按钮面板已禁用。请私聊 Bot 或在群里发送 /menu，我会把面板私聊给你。", true)
		s.deleteTelegramSourceMessage(channel, cb.Message.Chat.ID, cb.Message.MessageID)
		return nil
	}
	if cb.Message.Chat.Type == "private" && cb.Message.Chat.ID != cb.From.ID {
		s.answerCallbackWithText(ctx, channel, cb.ID, "这个面板不属于你，请发送 /menu 打开自己的面板。", true)
		return nil
	}
	s.answerCallback(ctx, channel, cb.ID)
	data := strings.TrimSpace(cb.Data)
	if data == "adult_toggle" {
		reply := s.cmdHideAdult(ctx, &msg, nil)
		if reply.Text != "" {
			err := s.reply(ctx, channel, cb.Message.Chat.ID, reply)
			s.deleteTelegramSourceMessage(channel, cb.Message.Chat.ID, cb.Message.MessageID)
			return err
		}
		return nil
	}
	if reply, handled := s.handleMenuCallback(ctx, channel, &msg, data); handled {
		if reply.Text != "" {
			err := s.reply(ctx, channel, cb.Message.Chat.ID, reply)
			s.deleteTelegramSourceMessage(channel, cb.Message.Chat.ID, cb.Message.MessageID)
			return err
		}
	}
	return nil
}

// answerCallback 应答 Telegram 回调查询，关闭按钮上的加载提示。
func (s *TelegramBotService) answerCallback(ctx context.Context, channel *model.NotifyChannel, callbackID string) {
	s.answerCallbackWithText(ctx, channel, callbackID, "", false)
}

func (s *TelegramBotService) answerCallbackWithText(ctx context.Context, channel *model.NotifyChannel, callbackID, text string, showAlert bool) {
	if channel == nil || strings.TrimSpace(callbackID) == "" {
		return
	}
	cfg := s.telegramChannelConfig(channel)
	if strings.TrimSpace(cfg["bot_token"]) == "" {
		return
	}
	payload := map[string]interface{}{
		"callback_query_id": callbackID,
	}
	if strings.TrimSpace(text) != "" {
		payload["text"] = text
		payload["show_alert"] = showAlert
	}
	if err := telegramPostJSON(ctx, cfg, "answerCallbackQuery", payload, 8*time.Second); err != nil {
		s.log.Debug("telegram answerCallbackQuery failed", zap.Error(sanitizeTelegramError(err)))
	}
}

func (s *TelegramBotService) telegramBinding(ctx context.Context, telegramUserID int) *model.TelegramBinding {
	if telegramUserID == 0 {
		return nil
	}
	var binding model.TelegramBinding
	err := s.repo.DB.WithContext(ctx).Where("telegram_user_id = ?", int64(telegramUserID)).First(&binding).Error
	if err != nil {
		return nil
	}
	return &binding
}

func (s *TelegramBotService) unbindTelegramUser(ctx context.Context, telegramUserID int) error {
	if s == nil || s.repo == nil || s.repo.DB == nil || telegramUserID == 0 {
		return nil
	}
	return s.repo.DB.WithContext(ctx).Unscoped().
		Where("telegram_user_id = ?", int64(telegramUserID)).
		Delete(&model.TelegramBinding{}).Error
}

func (s *TelegramBotService) telegramUserIsAdmin(ctx context.Context, channel *model.NotifyChannel, telegramUserID int) bool {
	if s.telegramUserIDConfigured(channel, telegramUserID) {
		return true
	}
	binding := s.telegramBinding(ctx, telegramUserID)
	if binding == nil {
		return false
	}
	user, err := s.repo.User.FindByID(ctx, binding.UserID)
	return err == nil && user != nil && user.Role == "admin" && user.IsActive
}

func (s *TelegramBotService) telegramChatAllowed(channel *model.NotifyChannel, chatID int) bool {
	if channel == nil {
		return false
	}
	configStr := channel.Config
	if s.crypto != nil && configStr != "" {
		configStr = s.crypto.Decrypt(configStr)
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(configStr), &cfg); err != nil {
		return false
	}
	target := strconv.Itoa(chatID)
	for _, key := range []string{"group_chat_id", "channel_chat_id", "command_chat_id"} {
		if configured := strings.TrimSpace(cfg[key]); configured != "" && configured == target {
			return true
		}
	}
	if strings.TrimSpace(cfg["group_chat_id"]) != "" || strings.TrimSpace(cfg["channel_chat_id"]) != "" || strings.TrimSpace(cfg["command_chat_id"]) != "" {
		return false
	}
	return strings.TrimSpace(cfg["chat_id"]) == target
}

// telegramBindDecision 表示成员资格校验的三态结果：通过 / 明确不通过 /
// 无法验证（getChatMember 出错，如 Bot 不在群、群 ID 失效、网络或代理不可达）。
// 区分「明确不是成员」和「查不了」，是为了避免把验证失败误报成「你不在群」。
type telegramBindDecision int

const (
	bindDenied       telegramBindDecision = iota // 已查实：不在任何绑定群组/频道
	bindAllowed                                  // 管理员，或查实是某绑定群组/频道成员
	bindUnverifiable                             // 配了群组/频道但 getChatMember 全部失败
)

// telegramMembership 表示单个 chat 的成员资格三态。
type telegramMembership int

const (
	membershipNo      telegramMembership = iota // 查实不是成员（left/kicked 等）
	membershipYes                               // 查实是成员
	membershipUnknown                           // getChatMember 出错，无法判定
)

func (s *TelegramBotService) telegramUserBindDecision(ctx context.Context, channel *model.NotifyChannel, telegramUserID int) telegramBindDecision {
	if telegramUserID == 0 || channel == nil {
		return bindDenied
	}
	if s.telegramUserIDConfigured(channel, telegramUserID) {
		return bindAllowed
	}
	chatIDs := s.telegramMembershipChatIDs(channel)
	if len(chatIDs) == 0 {
		return bindDenied
	}
	sawUnknown := false
	for _, chatID := range chatIDs {
		switch s.telegramChatMembership(ctx, channel, chatID, telegramUserID) {
		case membershipYes:
			return bindAllowed
		case membershipUnknown:
			sawUnknown = true
		}
	}
	if sawUnknown {
		return bindUnverifiable
	}
	return bindDenied
}

// telegramUserCanBind 是 telegramUserBindDecision 的布尔包装，供尽力而为的场景
// 使用（如私聊时挑选可用渠道）：只有查实通过才返回 true。
func (s *TelegramBotService) telegramUserCanBind(ctx context.Context, channel *model.NotifyChannel, telegramUserID int) bool {
	return s.telegramUserBindDecision(ctx, channel, telegramUserID) == bindAllowed
}

// telegramBindRejectText 根据三态结果生成面向用户的提示。action 形如「兑换注册账号」
// 「绑定媒体中心账号」。bindUnverifiable 时不再误导用户「你不在群」，而是提示
// 管理员检查 Bot 权限与群组 ID。
func telegramBindRejectText(decision telegramBindDecision, action string) string {
	if decision == bindUnverifiable {
		return fmt.Sprintf("暂时无法验证你的群组/频道成员身份，%s未成功。这通常是因为 Bot 未加入绑定群组、在频道中不是管理员，或群组 ID 配置有误（如超级群需带 -100 前缀）。请联系管理员检查 Bot 权限与「绑定群组/频道 ID」。", action)
	}
	return fmt.Sprintf("当前 Telegram 账号不在管理员配置的绑定群组/频道中，无法%s。请先加入管理员配置的群组或频道；如果尚未配置，请联系管理员。", action)
}

func (s *TelegramBotService) telegramChatMembership(ctx context.Context, channel *model.NotifyChannel, chatID string, telegramUserID int) telegramMembership {
	cfg := s.telegramChannelConfig(channel)
	if strings.TrimSpace(cfg["bot_token"]) == "" || chatID == "" || telegramUserID == 0 {
		return membershipUnknown
	}
	payload := map[string]interface{}{
		"chat_id": chatID,
		"user_id": telegramUserID,
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := telegramPostJSONDecode(ctx, cfg, "getChatMember", payload, 15*time.Second, &result); err != nil {
		s.log.Warn("telegram getChatMember failed", zap.String("chat_id", chatID), zap.Int("telegram_user_id", telegramUserID), zap.Error(sanitizeTelegramError(err)))
		return membershipUnknown
	}
	if !result.OK {
		return membershipUnknown
	}
	switch strings.ToLower(result.Result.Status) {
	case "creator", "administrator", "member", "restricted":
		return membershipYes
	default:
		return membershipNo
	}
}

// telegramUserIsChatMember 是 telegramChatMembership 的布尔包装，仅在查实是成员时
// 返回 true（查不了也视为非成员，供尽力而为的场景使用）。
func (s *TelegramBotService) telegramUserIsChatMember(ctx context.Context, channel *model.NotifyChannel, chatID string, telegramUserID int) bool {
	return s.telegramChatMembership(ctx, channel, chatID, telegramUserID) == membershipYes
}

func (s *TelegramBotService) telegramUserIDConfigured(channel *model.NotifyChannel, telegramUserID int) bool {
	if channel == nil || telegramUserID == 0 {
		return false
	}
	cfg := s.telegramChannelConfig(channel)
	target := strconv.Itoa(telegramUserID)
	for _, value := range telegramConfiguredUserIDs(cfg["admin_user_ids"]) {
		if value == target {
			return true
		}
	}
	if strings.TrimSpace(cfg["admin_user_ids"]) == "" && strings.TrimSpace(cfg["chat_id"]) == target {
		return true
	}
	return false
}

func (s *TelegramBotService) telegramChannelConfig(channel *model.NotifyChannel) map[string]string {
	return telegramConfigFromChannel(s.crypto, channel)
}

func telegramConfigFromChannel(crypto *CryptoService, channel *model.NotifyChannel) map[string]string {
	if channel == nil {
		return map[string]string{}
	}
	configStr := channel.Config
	if crypto != nil && configStr != "" {
		configStr = crypto.Decrypt(configStr)
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(configStr), &cfg); err != nil || cfg == nil {
		return map[string]string{}
	}
	normalizeTelegramConfig(cfg)
	return cfg
}

func normalizeTelegramConfig(cfg map[string]string) {
	if cfg == nil {
		return
	}
	chatID := strings.TrimSpace(cfg["chat_id"])
	if chatID == "" {
		return
	}
	if strings.HasPrefix(chatID, "-") {
		if strings.TrimSpace(cfg["group_chat_id"]) == "" && strings.TrimSpace(cfg["channel_chat_id"]) == "" && strings.TrimSpace(cfg["command_chat_id"]) == "" {
			cfg["group_chat_id"] = chatID
		}
		return
	}
	if strings.TrimSpace(cfg["admin_user_ids"]) == "" {
		cfg["admin_user_ids"] = chatID
	}
}

func (s *TelegramBotService) upsertTelegramBinding(ctx context.Context, msg *TelegramMessage, userID string) error {
	name := strings.TrimSpace(msg.From.FirstName)
	if msg.From.Username != "" {
		name = "@" + strings.TrimSpace(msg.From.Username)
	}
	telegramUserID := int64(msg.From.ID)
	return s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.TelegramBinding
		err := tx.Where("telegram_user_id = ?", telegramUserID).First(&existing).Error
		if err == nil {
			if err := s.replaceTelegramAccountBindingTx(ctx, tx, userID, telegramUserID); err != nil {
				return err
			}
			if err := tx.Model(&existing).Updates(map[string]any{
				"telegram_name": name,
				"chat_id":       telegramBindingChatIDForMessage(msg, &existing),
				"user_id":       userID,
			}).Error; telegramBindingUniqueErr(err) {
				return errTelegramAccountAlreadyBound
			} else if err != nil {
				return err
			}
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Unscoped().Where("telegram_user_id = ?", telegramUserID).Delete(&model.TelegramBinding{}).Error; err != nil {
			return err
		}
		if err := s.replaceTelegramAccountBindingTx(ctx, tx, userID, telegramUserID); err != nil {
			return err
		}
		err = tx.Create(&model.TelegramBinding{
			TelegramUserID: telegramUserID,
			TelegramName:   name,
			ChatID:         telegramBindingChatIDForMessage(msg, nil),
			UserID:         userID,
		}).Error
		if telegramBindingUniqueErr(err) {
			return errTelegramAccountAlreadyBound
		}
		return err
	})
}

func telegramBindingChatIDForMessage(msg *TelegramMessage, existing *model.TelegramBinding) int64 {
	if msg == nil {
		if existing != nil {
			return existing.ChatID
		}
		return 0
	}
	if msg.Chat.Type == "" || msg.Chat.Type == "private" {
		return int64(msg.Chat.ID)
	}
	if existing != nil && existing.ChatID > 0 {
		return existing.ChatID
	}
	return int64(msg.From.ID)
}

func telegramPrivateChatIDFromBinding(binding model.TelegramBinding) int64 {
	if binding.ChatID > 0 {
		return binding.ChatID
	}
	return binding.TelegramUserID
}

func (s *TelegramBotService) replaceTelegramAccountBindingTx(ctx context.Context, tx *gorm.DB, userID string, telegramUserID int64) error {
	return tx.WithContext(ctx).Unscoped().
		Where("user_id = ? AND telegram_user_id <> ?", userID, telegramUserID).
		Delete(&model.TelegramBinding{}).Error
}

func telegramBindingUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "idx_telegram_bindings_user_id_active") ||
		strings.Contains(msg, "telegram_bindings.user_id") ||
		(strings.Contains(msg, "unique") && strings.Contains(msg, "telegram_bindings"))
}

func parseStartCredentials(args []string) (string, string) {
	if len(args) >= 2 {
		return strings.TrimSpace(args[0]), strings.TrimSpace(strings.Join(args[1:], " "))
	}
	if len(args) == 1 {
		raw := strings.TrimSpace(args[0])
		for _, sep := range []string{"-", "：", ":"} {
			if parts := strings.SplitN(raw, sep, 2); len(parts) == 2 {
				return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			}
		}
	}
	return "", ""
}

func userNameOrFallback(user *model.User) string {
	if user == nil || strings.TrimSpace(user.Username) == "" {
		return "未知用户"
	}
	return user.Username
}
