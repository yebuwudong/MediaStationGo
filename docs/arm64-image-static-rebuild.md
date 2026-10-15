# arm64 官方镜像无法启动的原因与本地静态重编

## 现象

拉取官方镜像 `ghcr.io/shukebta/mediastation-go` 的 arm64 变体（如
`MediaStationGo-v0.1.25`）后，容器立即崩溃并反复重启，日志只有一行：

```
/entrypoint.sh: exec: line 22: mediastation-go: not found
```

二进制文件存在且可执行（`ls -la /usr/local/bin/mediastation-go` 正常，
37 MB，有 `+x`），但 shell 报 "not found"——这是**动态链接器缺失**的典型症状，
不是文件缺失。

## 根因

镜像里各架构的二进制解释器不一致：

| 架构 | 二进制解释器 | 运行时 libc | 结果 |
|---|---|---|---|
| amd64 | `/lib/ld-musl-x86_64.so.1` | musl（Alpine） | **可用** |
| arm64 | `/lib/ld-linux-aarch64.so.1` | musl（Alpine） | **崩溃** |

arm64 二进制被链接成 **glibc** 动态可执行文件，而基础镜像是
`alpine:3.23`（musl）。musl 环境没有 `ld-linux-aarch64.so.1`，内核找不到
解释器，于是 `exec` 失败并统一报 "not found"。

检查方法：

```bash
docker run --rm --entrypoint sh <image> -c 'readelf -l /usr/local/bin/mediastation-go | grep INTERP'
docker run --rm --entrypoint sh <image> -c 'cat /etc/os-release | head -2'
```

## 触发条件

`Dockerfile` 用 `CGO_ENABLED=0` 构建，但依赖
[`gen2brain/webp`](https://github.com/gen2brain/webp) 在默认构建标签下走
**purego** 路径（`dlopen` 加载 libc 与 libwebp），会强制二进制动态链接。
该库的构建标签相互排斥：

| 标签 | 文件 | 行为 |
|---|---|---|
| 默认（unix && !nodynamic） | `purego_unix.go`、`webp_dynamic.go` | purego + dlopen，**动态链接** |
| `nodynamic` | `purego_other.go`、`webp_wazero.go` | wazero（纯 Go WASM），**静态链接** |

amd64 构建恰好产出 musl 解释器所以能跑；arm64 产出 glibc 解释器所以崩溃。
两者行为差异属于构建工具链的架构相关特性，不是代码问题。

## 解决办法：用 `nodynamic` 标签本地重编

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -tags nodynamic -trimpath \
  -ldflags="-s -w -X main.version=<version>" \
  -o mediastation-go ./cmd/server
```

产出为 `statically linked`，可在 musl 环境运行。

**功能影响：无。** `nodynamic` 切到 wazero 实现，`webp.Decode` / `webp.Encode`
等公开 API 完全一致，WebP 图片变体功能（A10）正常。已用往返编解码测试验证
（编码 32x32 RGBA → 156 字节，`RIFF`/`WEBP` 头正确，解码回 32x32）。

## 复用官方镜像前端

为避免在本地安装前端依赖（上游新增了 `danmaku@^2.0.10` 等），推荐基于官方
镜像只替换后端二进制：

```dockerfile
FROM ghcr.io/shukebta/mediastation-go:MediaStationGo-v0.1.25

ARG VERSION=dev
ARG REVISION=dev

COPY mediastation-go /usr/local/bin/mediastation-go
RUN chmod +x /usr/local/bin/mediastation-go

LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
```

前端 `dist`、ffmpeg、entrypoint、pg client 全部沿用官方镜像。

构建时若 Docker 的默认 bridge 网络不可用，加 `--network host`：

```bash
docker build --network host -t <tag> .
```

## 附带的 PostgreSQL 客户端版本问题

官方镜像内置 `postgresql16-client`（`pg_dump 16.15`）。若数据库是
PostgreSQL 18，`/api/admin/backups` 与 Telegram `/backup_db` 会直接失败：

```
pg_dump: error: aborting because of server version mismatch
pg_dump: error: detail: server version: 18.6; pg_dump version: 16.15
```

上游 `Dockerfile` 有 `POSTGRES_CLIENT_VERSION` 构建参数，自建镜像时应设为与
服务器主版本一致的值。若无法自建，可从 `postgres:18-alpine` 提取客户端二进制
挂载进容器，并让 `PATH` 优先命中：

```bash
docker run --rm -v ./pgtools:/out --entrypoint sh postgres:18-alpine \
  -c 'cp $(command -v pg_dump) $(command -v pg_restore) /out/ && chmod +x /out/*'
```

compose 侧：

```yaml
volumes:
  - ./pgtools:/pg18:ro
environment:
  PATH: /pg18:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
```

注意 `/api/admin/backups` 通过 `exec.LookPath("pg_dump")` 查找，因此只需
调整 `PATH` 即可，无需改代码。

## 上游状态

截至 `dc6e5a7`，上游 CI（`.github/workflows/docker-publish.yml`）同时构建
amd64 与 arm64，未针对该动态链接问题加固，因此官方 arm64 镜像持续不可用。
若上游修复，本节所述本地重编可弃用。

## 部署注意

应用内「热更新」默认镜像写死 `ghcr.io/shukebta/mediastation-go:latest`
（`internal/service/system_update.go` 的 `DefaultSystemUpdateImage`）。
在 arm64 上点击该功能会把容器换成无法启动的镜像。部署时应显式设置
`MEDIASTATION_UPDATE_IMAGE` 指向可用镜像。
