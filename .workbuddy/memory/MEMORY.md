# MediaStationGo 项目约定

## 项目信息
- **仓库**：https://github.com/ShukeBta/MediaStationGo
- **本地路径**：D:\项目\MediaStationGo
- **技术栈**：Go 1.25 + Gin + GORM + SQLite / React 18 + Vite + Tailwind CSS + Zustand

## 构建方式（裸机 Windows）
```bash
# 后端
cd D:/项目/MediaStationGo
go build ./cmd/server    # 生成 server.exe

# 前端
cd D:/项目/MediaStationGo/web
npm install && npm run build    # 生成 web/dist/

# 配置
cp config.example.yaml config.yaml
mkdir -p data cache
```

## 启动命令
```bash
cd D:/项目/MediaStationGo
./server.exe
# 或带环境变量：
MEDIASTATION_APP_PORT=8080 MEDIASTATION_APP_DATA_DIR=./data MEDIASTATION_APP_WEB_DIR=./web/dist ./server.exe
```

## 默认配置
- 端口：8080
- 管理员：admin / admin123（首次登录提示改密）
- 数据目录：./data
- 缓存目录：./cache

## 关键 API 端点
- `GET /api/health` - 健康检查（无需认证）
- `POST /api/auth/login` - 登录获取 JWT
- `GET /api/stats` - 系统统计（需认证）
- `GET /api/libraries` - 媒体库列表（需认证）
- `GET /api/admin/users` - 用户管理（需 admin）

## 注意事项
- Docker Desktop 未启动时使用裸机构建
- Go 1.22 会自动下载 1.25 工具链（GOTOOLCHAIN=auto）
- FFmpeg/ffprobe 需要单独安装才能使用媒体扫描和转码功能
- **DB 迁移**：model.Base 嵌入 gorm.DeletedAt，如从旧版升级需手动 `ALTER TABLE api_configs ADD COLUMN deleted_at datetime, created_at datetime`
- **前端重建**：修改后端代码后记得 `npm run build` 重建 dist

## 服务容器 (internal/service/service.go)
- `SiteService` 已注册为 `Container.Site`
- `SiteHandler` 位于 `internal/handler/site_handler.go`，支持 List/Get/Create/Update/Delete/Test/SiteTypes/AuthTypes
- 站点路由注册于 authed 组：`/api/sites`（7条）

## 前端 Admin 调度器面板
- 后端只支持 `schedulerAPI.status()` 和 `schedulerAPI.run(name)`
- 不支持 enable/disable 切换（后端无对应 API）

## 前端站点管理页面 (2026-05-16 新增)
- **API 客户端**：`web/src/api/sites.ts`，对接 8 个端点（list/get/create/update/remove/test/types/authTypes）
- **页面组件**：`web/src/pages/SitesPage.tsx`，完整 CRUD + 测试 + 弹窗
- **路由**：`/sites`（RequireAdmin），已集成到 App.tsx
- **侧边栏**：管理区「站点管理」链接（Globe 图标）
- **AdminPage Tab**：新增「站点管理」默认激活 Tab，内嵌 SitesPage
- **站点类型**：nexusphp / gazelle / unit3d / mteam / discuz / custom_rss
- **认证方式**：cookie / api_key / auth_header
- **模型字段**：Site{Name, Type(not "site_type"), URL(not "base_url"), AuthType, Cookie, APIKey, AuthHeader, Enabled, IsDefault, Extra} — 注意 Go 后端与 Python 后端的字段名差异

## UI 修复与优化 (2026-05-16 第二阶段)

### 搜索页 & 收藏页空白修复
- **收藏页根因**：后端 `GET /api/favourites` 在无数据时返回 `{"items": null}`，前端 `playbackAPI.listFavourites()` 提取 `r.data.items` 得到 null，`items.length` 抛出 TypeError → React 白屏
- **修复**：`FavouritesPage` 增加 `data ?? []` 空值保护，添加 .catch 错误处理、重试按钮、空状态提示
- **搜索页修复**：增加错误处理，空查询不再发请求（避免无效 API 调用），添加 idle/empty/error 三态 UI

### AdminPage 去重 & APIConfigs 重构
- **问题**：原「API 配置」独立页面 (`/api-configs`) 与 AdminPage 内「设置」Tab 功能重叠
- **方案**：
  - AdminPage 新增「外部API」Tab（`api`），嵌入新建的 `APIConfigsPanel` 组件
  - 原「设置」Tab 重命名为「系统设置」以区分用途
  - 侧边栏移除「API 配置」独立链接（`KeyRound`图标）
  - `/api-configs` 路由改为 `<Navigate to="/admin" replace />`
- **APIConfigsPanel**：紧凑表格布局（Provider | 密钥掩码 | 状态徽章 | 操作按钮），点击编辑后行内展开表单，替代原先卡片式布局
- **文件变更**：新增 `web/src/components/APIConfigsPanel.tsx`；修改 `AdminPage.tsx`、`Layout.tsx`、`App.tsx`；保留 `APIConfigsPage.tsx` 但不再路由引用
