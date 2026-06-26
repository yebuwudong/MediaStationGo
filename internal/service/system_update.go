package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

const (
	SystemUpdateImageSettingKey           = "system.update.image"
	SystemUpdateWatchtowerImageSettingKey = "system.update.watchtower_image"
	SystemUpdateCommandSettingKey         = "system.update.command"

	DefaultSystemUpdateImage           = "ghcr.io/shukebta/mediastation-go:latest"
	DefaultSystemUpdateWatchtowerImage = "containrrr/watchtower:latest"
)

const (
	systemUpdateCheckTimeout = 20 * time.Second
	systemUpdateRunTimeout   = 15 * time.Minute
)

var ErrSystemUpdateRunning = errors.New("system update already running")

type SystemUpdateStatus struct {
	Image           string  `json:"image"`
	WatchtowerImage string  `json:"watchtower_image,omitempty"`
	ContainerID     string  `json:"container_id,omitempty"`
	ContainerName   string  `json:"container_name,omitempty"`
	CurrentImageID  string  `json:"current_image_id,omitempty"`
	LocalDigest     string  `json:"local_digest,omitempty"`
	RemoteDigest    string  `json:"remote_digest,omitempty"`
	DockerAvailable bool    `json:"docker_available"`
	CanApply        bool    `json:"can_apply"`
	UpdateAvailable *bool   `json:"update_available,omitempty"`
	Running         bool    `json:"running"`
	TaskID          string  `json:"task_id,omitempty"`
	Message         string  `json:"message,omitempty"`
	Details         string  `json:"details,omitempty"`
	CheckedAt       *string `json:"checked_at,omitempty"`
	StartedAt       *string `json:"started_at,omitempty"`
}

type SystemUpdateService struct {
	cfg   *config.Config
	log   *zap.Logger
	repo  *repository.Container
	tasks *TaskTrackerService

	mu      sync.Mutex
	running bool
	last    *SystemUpdateStatus
}

func NewSystemUpdateService(cfg *config.Config, log *zap.Logger, repo *repository.Container, tasks *TaskTrackerService) *SystemUpdateService {
	return &SystemUpdateService{cfg: cfg, log: log, repo: repo, tasks: tasks}
}

func (s *SystemUpdateService) Status(ctx context.Context) SystemUpdateStatus {
	s.mu.Lock()
	if s.last != nil {
		status := cloneSystemUpdateStatus(*s.last)
		status.Running = s.running
		s.mu.Unlock()
		return status
	}
	running := s.running
	s.mu.Unlock()

	status := s.baseStatus(ctx)
	status.Running = running
	status.Message = "尚未检查更新"
	return status
}

func (s *SystemUpdateService) Check(ctx context.Context) SystemUpdateStatus {
	status := s.check(ctx)
	s.mu.Lock()
	status.Running = s.running
	s.last = &status
	s.mu.Unlock()
	return status
}

func (s *SystemUpdateService) Apply(ctx context.Context) (SystemUpdateStatus, error) {
	s.mu.Lock()
	if s.running {
		status := SystemUpdateStatus{}
		if s.last != nil {
			status = cloneSystemUpdateStatus(*s.last)
		}
		status.Running = true
		s.mu.Unlock()
		return status, ErrSystemUpdateRunning
	}
	s.running = true
	s.mu.Unlock()

	status := s.check(ctx)
	if !status.CanApply {
		s.finishWithoutTask(status)
		return status, errors.New(firstNonEmpty(status.Message, "当前环境不支持自动更新"))
	}

	task := s.startUpdateTask(status)
	if task != nil {
		status.TaskID = task.id
	}
	now := time.Now().Format(time.RFC3339)
	status.StartedAt = &now
	status.Running = true
	status.Message = "已开始后台更新，容器会在更新完成后重启"
	s.mu.Lock()
	s.last = &status
	s.mu.Unlock()

	go s.runUpdate(context.Background(), status, task)
	return status, nil
}

func (s *SystemUpdateService) finishWithoutTask(status SystemUpdateStatus) {
	status.Running = false
	s.mu.Lock()
	s.running = false
	s.last = &status
	s.mu.Unlock()
}

func (s *SystemUpdateService) check(ctx context.Context) SystemUpdateStatus {
	status := s.baseStatus(ctx)
	now := time.Now().Format(time.RFC3339)
	status.CheckedAt = &now

	customCommand := s.rawUpdateCommand(ctx)
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		if customCommand != "" {
			status.CanApply = true
			status.Message = "当前环境无法检查 Docker；将执行自定义更新命令"
			status.Details = err.Error()
			return status
		}
		status.Message = "当前镜像内未安装 docker CLI，无法自动拉取并重启 Docker 镜像"
		return status
	}
	if runtime.GOOS != "windows" {
		if _, err := os.Stat("/var/run/docker.sock"); err != nil {
			if customCommand != "" {
				status.CanApply = true
				status.Message = "未检测到 Docker socket；将执行自定义更新命令"
				status.Details = err.Error()
				return status
			}
			status.Message = "未检测到 /var/run/docker.sock，请挂载 Docker socket 后再使用热更新"
			status.Details = err.Error()
			return status
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, systemUpdateCheckTimeout)
	defer cancel()
	if out, err := runSystemUpdateCommand(checkCtx, dockerPath, "version", "--format", "{{.Server.Version}}"); err != nil {
		if customCommand != "" {
			status.CanApply = true
			status.Message = "无法连接 Docker 引擎；将执行自定义更新命令"
			status.Details = strings.TrimSpace(out + "\n" + err.Error())
			return status
		}
		status.Message = "无法连接 Docker 引擎，请检查 Docker socket 权限"
		status.Details = strings.TrimSpace(out + "\n" + err.Error())
		return status
	}
	status.DockerAvailable = true

	containerID := currentContainerID()
	status.ContainerID = containerID
	if containerID == "" {
		status.Message = "无法识别当前容器 ID；请配置自定义更新命令"
		if customCommand != "" {
			status.CanApply = true
			status.Message = "无法识别当前容器 ID；将执行自定义更新命令"
		}
		return status
	}

	if out, err := runSystemUpdateCommand(checkCtx, dockerPath, "inspect", containerID, "--format", "{{.Name}}|{{.Image}}"); err == nil {
		name, imageID := parseContainerInspectLine(out)
		status.ContainerName = name
		status.CurrentImageID = imageID
	}
	if out, err := runSystemUpdateCommand(checkCtx, dockerPath, "image", "inspect", status.Image, "--format", "{{json .RepoDigests}}"); err == nil {
		status.LocalDigest = firstDockerDigest(out)
	}
	if out, err := runSystemUpdateCommand(checkCtx, dockerPath, "manifest", "inspect", "--verbose", status.Image); err == nil {
		status.RemoteDigest = firstDockerDigest(out)
	} else if status.Details == "" {
		status.Details = strings.TrimSpace(out + "\n" + err.Error())
	}
	status.UpdateAvailable = compareDockerDigests(status.LocalDigest, status.RemoteDigest)
	status.CanApply = true
	status.Message = systemUpdateCheckMessage(status)
	return status
}

func (s *SystemUpdateService) runUpdate(parent context.Context, status SystemUpdateStatus, task *TaskHandle) {
	runCtx, cancel := context.WithTimeout(parent, systemUpdateRunTimeout)
	defer cancel()

	command := s.updateCommand(runCtx, status)
	if task != nil {
		task.Update(TaskUpdate{
			Stage:   "pull_restart",
			Message: "正在拉取最新镜像并重建当前容器",
			Details: []string{
				"image=" + status.Image,
				"container=" + firstNonEmpty(status.ContainerName, status.ContainerID),
				"command=" + redactSystemUpdateCommand(command),
			},
		})
	}

	output, err := runSystemUpdateShell(runCtx, command)
	details := systemUpdateOutputDetails(output)
	if task != nil {
		task.Finish(err, TaskUpdate{
			Stage:   "completed",
			Message: "更新命令已执行；如果容器未自动重启，请查看 Docker 日志",
			Details: details,
		})
	}
	status.Running = false
	status.Message = "更新命令已执行；如果容器未自动重启，请查看 Docker 日志"
	status.Details = strings.Join(details, "\n")
	if err != nil {
		status.Message = "更新失败：" + err.Error()
	}
	s.mu.Lock()
	s.running = false
	s.last = &status
	s.mu.Unlock()
	if err != nil && s.log != nil {
		s.log.Warn("system update failed", zap.Error(err), zap.String("output", output))
	}
}

func (s *SystemUpdateService) startUpdateTask(status SystemUpdateStatus) *TaskHandle {
	if s == nil || s.tasks == nil {
		return nil
	}
	return s.tasks.Start(TaskKindUpdate, "Docker 镜像热更新", TaskUpdate{
		Stage:      "prepare",
		SourcePath: status.Image,
		DestPath:   firstNonEmpty(status.ContainerName, status.ContainerID),
		Message:    "准备拉取最新版 Docker 镜像并重启容器",
	})
}

func (s *SystemUpdateService) baseStatus(ctx context.Context) SystemUpdateStatus {
	image := s.setting(ctx, SystemUpdateImageSettingKey, os.Getenv("MEDIASTATION_UPDATE_IMAGE"))
	if image == "" {
		image = DefaultSystemUpdateImage
	}
	watchtowerImage := s.setting(ctx, SystemUpdateWatchtowerImageSettingKey, os.Getenv("MEDIASTATION_UPDATE_WATCHTOWER_IMAGE"))
	if watchtowerImage == "" {
		watchtowerImage = DefaultSystemUpdateWatchtowerImage
	}
	return SystemUpdateStatus{
		Image:           image,
		WatchtowerImage: watchtowerImage,
		ContainerID:     currentContainerID(),
	}
}

func (s *SystemUpdateService) setting(ctx context.Context, key, fallback string) string {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return strings.TrimSpace(fallback)
	}
	if value, err := s.repo.Setting.Get(ctx, key); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func (s *SystemUpdateService) updateCommand(ctx context.Context, status SystemUpdateStatus) string {
	custom := s.rawUpdateCommand(ctx)
	if strings.TrimSpace(custom) != "" {
		return renderSystemUpdateCommand(custom, status)
	}
	return renderSystemUpdateCommand(defaultSystemUpdateCommand(), status)
}

func (s *SystemUpdateService) rawUpdateCommand(ctx context.Context) string {
	return s.setting(ctx, SystemUpdateCommandSettingKey, os.Getenv("MEDIASTATION_UPDATE_COMMAND"))
}

func defaultSystemUpdateCommand() string {
	return "docker run --rm -v /var/run/docker.sock:/var/run/docker.sock {{watchtower_image}} --run-once --cleanup {{container}}"
}

func renderSystemUpdateCommand(template string, status SystemUpdateStatus) string {
	replacements := map[string]string{
		"{{image}}":            shellQuote(status.Image),
		"{{watchtower_image}}": shellQuote(firstNonEmpty(status.WatchtowerImage, DefaultSystemUpdateWatchtowerImage)),
		"{{container}}":        shellQuote(firstNonEmpty(status.ContainerName, status.ContainerID)),
		"{{container_id}}":     shellQuote(status.ContainerID),
		"{{container_name}}":   shellQuote(status.ContainerName),
	}
	out := template
	for marker, value := range replacements {
		out = strings.ReplaceAll(out, marker, value)
	}
	return strings.TrimSpace(out)
}

func runSystemUpdateCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func runSystemUpdateShell(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("empty update command")
	}
	if runtime.GOOS == "windows" {
		return runSystemUpdateCommand(ctx, "cmd", "/C", command)
	}
	return runSystemUpdateCommand(ctx, "/bin/sh", "-c", command)
}

func currentContainerID() string {
	if value := strings.TrimSpace(os.Getenv("HOSTNAME")); looksLikeContainerID(value) {
		return value
	}
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	host = strings.TrimSpace(host)
	if looksLikeContainerID(host) {
		return host
	}
	return ""
}

func looksLikeContainerID(value string) bool {
	if len(value) < 12 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'f') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func parseContainerInspectLine(raw string) (string, string) {
	line := strings.TrimSpace(raw)
	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		return strings.TrimPrefix(line, "/"), ""
	}
	return strings.TrimPrefix(strings.TrimSpace(parts[0]), "/"), strings.TrimSpace(parts[1])
}

var dockerDigestPattern = regexp.MustCompile(`sha256:[a-fA-F0-9]{64}`)

func firstDockerDigest(raw string) string {
	match := dockerDigestPattern.FindString(raw)
	return strings.ToLower(strings.TrimSpace(match))
}

func compareDockerDigests(localDigest, remoteDigest string) *bool {
	localDigest = strings.ToLower(strings.TrimSpace(localDigest))
	remoteDigest = strings.ToLower(strings.TrimSpace(remoteDigest))
	if localDigest == "" || remoteDigest == "" {
		return nil
	}
	available := localDigest != remoteDigest
	return &available
}

func systemUpdateCheckMessage(status SystemUpdateStatus) string {
	if status.UpdateAvailable == nil {
		return "已连接 Docker，可执行更新；当前环境无法精确比较本地与远端镜像摘要"
	}
	if *status.UpdateAvailable {
		return "检测到远端镜像摘要与本地不同，可以执行更新"
	}
	return "当前本地镜像摘要与远端一致"
}

func systemUpdateOutputDetails(output string) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func redactSystemUpdateCommand(command string) string {
	command = strings.TrimSpace(command)
	if len(command) > 500 {
		return command[:500] + "..."
	}
	return command
}

func shellQuote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func cloneSystemUpdateStatus(status SystemUpdateStatus) SystemUpdateStatus {
	if status.UpdateAvailable != nil {
		v := *status.UpdateAvailable
		status.UpdateAvailable = &v
	}
	if status.CheckedAt != nil {
		v := *status.CheckedAt
		status.CheckedAt = &v
	}
	if status.StartedAt != nil {
		v := *status.StartedAt
		status.StartedAt = &v
	}
	return status
}
