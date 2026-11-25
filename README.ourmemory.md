# ourmemory

ourmemory 是一个以 **OpenList** 为底座二次开发的共享存储平台：一个「所有用户共享的归档大包」、每人一个「加密私人归档包」，并在此之上提供视频、图片、微博、IM 四个内容站，以及开放存储 API 和贡献节点。

> 本项目基于 [OpenList](https://github.com/OpenListTeam/OpenList)（AGPL-3.0）修改。网络服务对应源码与修改说明见 [docs/SOURCE_OFFER.md](docs/SOURCE_OFFER.md)。

## 当前已实现

| 模块 | 状态 |
| --- | --- |
| 邮箱验证码注册 / 登录 / 会话 | 可用（SkyMail 未配置时用 `OM_DEBUG=1` 输出验证码到日志） |
| 共享盘：目录、上传、改名、移动、删除、直链、下载、签名链接 | 可用 |
| 私人盘：手动开通、AES-256-GCM 加密、按用户配额 | 可用 |
| 视频站：上传登记、列表、详情、播放、点赞/投币/收藏、评论、弹幕、进度、转发 | 可用；浏览器不支持的编码需下载观看 |
| 图片站：列表、详情、点赞/收藏、评论 | 可用 |
| 微博：纯文字正文 + 共享盘媒体、点赞、评论、转发、关注 | 可用 |
| IM：公共大厅、私人建群、限制加入/阅读、聊天记录归档为单个文件 | 可用 |
| 开放 API：API Key、`/api/v1` 文件接口 | 可用 |
| 贡献节点：注册、心跳、分配、复制、上报、开机自启安装脚本 | 可用 |
| 管理后台：统计、兑换码、用户、节点、存储包整理 | 可用；`/@manage` 为 OpenList 原生完整管理台 |
| 捐赠档位 / 预留包 / 兑换码 | 可用 |
| 子域路由：`video.` / `image.` / `weibo.` / `im.` | 可用 |
| TailchatPlus 接入 | 提供接入提示词与 API 契约，尚未改 tcplus 代码 |
| 生产反向代理 / TLS / DNS | 需按 [docs/DEPLOY.md](docs/DEPLOY.md) 配置 |

## 存储模型

- 共享盘：`data/packs/shared.ompk`，追加型归档；每个文件独立 zstd 压缩，记录在 `om_blobs`，按 SHA-256 去重；读取时解压到 `data/cache`。
- 私人盘：`data/packs/private-<uid>.ompk`，zstd + AES-256-GCM（1 MiB 分块）；用户密钥由主密钥包裹后存 `om_user_keys`；解密临时文件写入 `data/tmp` 后删除。
- 删除只减引用计数并记 `waste`；管理后台「存储包 → 整理」调用 `Compact` 重写归档回收磁盘。
- 共享盘默认上限 5 GiB（`OM_SHARED_MAX_BYTES`），因为首版服务器只有约 29 GiB 可用；贡献节点接入后按实际提供容量扩容。

## 快速启动（开发机）

```powershell
$env:GOTOOLCHAIN="local"
$env:GOPROXY="https://goproxy.cn,direct"
$env:GOSUMDB="sum.golang.google.cn"
go build -trimpath -o bin\ourmemory.exe .

$env:OPENLIST_ADMIN_PASSWORD="admin12345"
$env:OM_DEBUG="1"
$env:OM_BASE_HOST="om.autozteam.cn"
.\bin\ourmemory.exe server --data .\data --log-std
```

访问 `http://127.0.0.1:5259`。端口在 `data/config.json` 的 `scheme.http_port`，环境变量 `OPENLIST_SCHEME_HTTP_PORT` 对已存在的配置不生效。

## 生产部署

服务器已按 `scripts/install-server.sh` 安装为 systemd 服务：

```bash
systemctl status ourmemory
systemctl restart ourmemory
journalctl -u ourmemory -f
```

- 程序：`/opt/ourmemory/ourmemory`
- 数据：`/opt/ourmemory/data`
- 环境：`/opt/ourmemory/ourmemory.env`
- 管理员初始密码：`/opt/ourmemory/ADMIN_PASSWORD.txt`
- 节点密钥：`/opt/ourmemory/NODE_SECRET.txt`

### 管理入口

- `/admin`：ourmemory 统计、兑换码、用户、贡献节点、存储包整理。
- `/@manage`：OpenList 原生完整管理台（用户、存储驱动、站点设置、任务、索引等）。两个入口使用同一 `admin` 账号。
- `public/dist` 是 OpenList 前端，24 MB 资源不入库；部署机执行 `scripts/fetch-openlist-frontend.sh` 或从 release 解压。仓库只保留 `public/dist/index.html` 作为启动占位。

### 文件预览

共享盘与私人盘的文本、Markdown、JSON、CSV、PDF、图片、音视频、HTML 支持在线预览；私人文件只有所有者可读，管理员也不能绕过。私人文件可生成 24 小时签名链接。

完整 DNS / Cloudflare Tunnel / Nginx / 证书步骤见 [docs/DEPLOY.md](docs/DEPLOY.md)。

## 目录

```
internal/om/          业务层（模型、存储、认证、文件、子站、IM、API、节点）
cmd/omnode/           贡献节点 agent
public/om/            前端（原生 HTML/CSS/JS，无构建）
public/dist/          OpenList 前端；index.html 入库，assets 由 scripts/fetch-openlist-frontend.sh 拉取
deploy/               systemd 与环境变量模板
scripts/              服务端安装、节点安装
docs/                 计划、部署、API、TailchatPlus 接入、源码提供
```

## 安全边界

- 私人盘对磁盘和贡献节点不可读；主站进程持有主密钥，因此**不是零知识 E2EE**。需要零知识时应在客户端加密后再上传。
- 共享盘内容对所有登录/匿名访问者可见；上传、改名、删除按上传者校验。
- 仅允许图片、视频、音频、HTML 在线打开；HTML 以 `CSP sandbox` 返回，不能读取站点会话。
- API Key 只在创建时返回一次，数据库存 SHA-256。

## License

AGPL-3.0。上游版权归 OpenListTeam / AList 贡献者；本项目修改部分同样以 AGPL-3.0 提供。
