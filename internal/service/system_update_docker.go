package service

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func (s *SystemUpdateService) check(ctx context.Context) SystemUpdateStatus {
	status := s.baseStatus(ctx)
	now := time.Now().Format(time.RFC3339)
	status.CheckedAt = &now

	customCommand := s.rawUpdateCommand(ctx)
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return systemUpdateCustomFallback(status, systemUpdateFallback{
			command:        customCommand,
			customMessage:  "当前环境无法检查 Docker；将执行自定义更新命令",
			customDetails:  err.Error(),
			defaultMessage: "当前镜像内未安装 docker CLI，无法自动拉取并重启 Docker 镜像",
		})
	}
	if runtime.GOOS != "windows" {
		if _, err := os.Stat("/var/run/docker.sock"); err != nil {
			return systemUpdateCustomFallback(status, systemUpdateFallback{
				command:        customCommand,
				customMessage:  "未检测到 Docker socket；将执行自定义更新命令",
				customDetails:  err.Error(),
				defaultMessage: "未检测到 /var/run/docker.sock，请挂载 Docker socket 后再使用热更新",
				defaultDetails: err.Error(),
			})
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, systemUpdateCheckTimeout)
	defer cancel()
	if out, err := runSystemUpdateCommand(checkCtx, dockerPath, "version", "--format", "{{.Server.Version}}"); err != nil {
		details := strings.TrimSpace(out + "\n" + err.Error())
		return systemUpdateCustomFallback(status, systemUpdateFallback{
			command:        customCommand,
			customMessage:  "无法连接 Docker 引擎；将执行自定义更新命令",
			customDetails:  details,
			defaultMessage: "无法连接 Docker 引擎，请检查 Docker socket 权限",
			defaultDetails: details,
		})
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

	populateSystemUpdateDockerMetadata(checkCtx, dockerPath, &status)
	status.UpdateAvailable = compareDockerDigests(status.LocalDigest, status.RemoteDigest)
	status.CanApply = true
	status.Message = systemUpdateCheckMessage(status)
	return status
}

type systemUpdateFallback struct {
	command        string
	customMessage  string
	customDetails  string
	defaultMessage string
	defaultDetails string
}

func systemUpdateCustomFallback(status SystemUpdateStatus, fallback systemUpdateFallback) SystemUpdateStatus {
	if fallback.command != "" {
		status.CanApply = true
		status.Message = fallback.customMessage
		status.Details = fallback.customDetails
		return status
	}
	status.Message = fallback.defaultMessage
	status.Details = fallback.defaultDetails
	return status
}

func populateSystemUpdateDockerMetadata(ctx context.Context, dockerPath string, status *SystemUpdateStatus) {
	if status == nil {
		return
	}
	if out, err := runSystemUpdateCommand(ctx, dockerPath, "inspect", status.ContainerID, "--format", "{{.Name}}|{{.Image}}"); err == nil {
		name, imageID := parseContainerInspectLine(out)
		status.ContainerName = name
		status.CurrentImageID = imageID
	}
	if out, err := runSystemUpdateCommand(ctx, dockerPath, "image", "inspect", status.Image, "--format", "{{json .RepoDigests}}"); err == nil {
		status.LocalDigest = firstDockerDigest(out)
	}
	if out, err := runSystemUpdateCommand(ctx, dockerPath, "manifest", "inspect", "--verbose", status.Image); err == nil {
		status.RemoteDigest = firstDockerDigest(out)
	} else if status.Details == "" {
		status.Details = strings.TrimSpace(out + "\n" + err.Error())
	}
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
