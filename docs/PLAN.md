# ourmemory 实施计划

> 版本：v1（可上线 MVP）  
> 底座：OpenList 源码 fork（保留 AGPL 与上游历史）  
> 部署：本地 `192.168.1.11:5259`，公网 `om.autozteam.cn`，子站 `video/image/weibo/im.om.autozteam.cn`

## 1. 目标与验收

### 1.1 总目标

做一个「共享存储网盘 + 每人私人加密盘 + 视频/图片/微博/IM 子站」的平台。所有子站资源都进共享盘；共享资源所有人可见；私人资源只有本人可见；所有资源尽量直链，方便 Cloudflare CDN。

### 1.2 首版必须可用（P0）

- [ ] 邮箱验证注册、登录、退出、个人主页、多枚头衔。
- [ ] 共享盘：全局一个逻辑归档包，按需解压；上传/浏览/重命名/移动/删除自己的文件；所有人可读。
- [ ] 私人盘：手动开通，一人一个加密归档包；普通/Ⅰ/Ⅱ/Ⅲ 四档配额与压缩等级。
- [ ] 在线预览：图片、视频、音频、静态 HTML；其他文件仅下载；禁止 exe/py/sh 等可执行文件。
- [ ] 直链与仿直链：`/f/:id/:name` 内联、`/dl/:id/:name` 下载、`/s/:token` 带签名 CDN 链接；显示上传者与上传者主页。
- [ ] 视频站：列表、播放、进度、点赞、投币、收藏、评论、弹幕、推荐排序、转发到动态。
- [ ] 图片站：瀑布流、详情、标签、点赞、收藏、评论、排行；无弹幕。
- [ ] 微博站：纯文字正文 + 共享盘媒体；发帖、转发、评论、点赞、关注、话题、个人时间线。
- [ ] IM：公共大厅（共享盘聊天记录、可加入/可看记录/可限制）、私人群（创建者私人盘、成员可见）；简易 WebSocket 实时消息。
- [ ] 开放 API：API Key（`X-API-Key`），文件读写、列表、元数据；第三方应用申请审核。
- [ ] 存储贡献：节点注册、心跳、容量统计、副本复制、贡献者头衔与私人额度折算。
- [ ] 管理后台：用户、文件、节点、兑换码、捐赠档位、审核、审计日志。
- [ ] 兑换码：捐赠档位、预留包、容量包；永久权益。
- [ ] 存储贡献者一键安装工具：询问 GB、写 systemd、开机自启、自动注册上线。
- [ ] 部署：`/opt/ourmemory`，端口 5259，systemd；Nginx/Cloudflare 配置文档；TailchatPlus 接入提示词。

### 1.3 首版明确不做（P1，接口预留）

- 真实在线支付（首版兑换码由管理员发放）。
- 视频转码集群/多码率（先原片 + Range；转码接口预留）。
- 完整 B 站级推荐算法（先用加权热度公式）。
- 贡献节点的跨公网 NAT 穿透（节点需有公网/可被主站访问的地址；或后续加反向隧道）。
- 零知识端到端加密（首版为服务端信封加密：节点/磁盘不可读，主站持主密钥）。
- TailchatPlus 深度改造（首版只交付接入提示词与兼容 API）。

## 2. 现状与硬约束

- 工作区 `D:\project\ourmemory` 已是 OpenList fork，HEAD `7c26f10`，底座已成功编译为 `bin/openlist-base.exe`。
- 本地 Windows：Go 1.27.3（`D:\tools\go1.26.4\go\bin\go.exe`）、Node 22、pnpm 11；无 Docker。
- 服务器 `192.168.1.11`：Ubuntu 22.04，12 核 / 14GiB / 根盘剩 ~29GB；Docker、1Panel、OpenResty（80/443）、SafeLine、Tailchat、tcplus、PostgreSQL/MySQL/Mongo/Redis/MinIO 已运行。
- 不允许改 1Panel 配置文件；本应用只监听 `127.0.0.1:5259`，由独立反代/隧道接入。
- 现有 hp-lite 隧道实测断线（`43.136.14.67:16666` timeout）。上线前必须二选一：
  1. 修好 hp-lite；或
  2. 用 Cloudflare Tunnel（推荐，自动 TLS、支持 `*.om` 二级子域）。
- DNS `om.autozteam.cn` 尚不存在；Cloudflare Universal SSL 不覆盖 `*.om` 二级子域，需要 Total TLS 或单独证书。
- 首版共享盘容量 5GB（本地盘）；外部贡献节点上线后扩容。
- 用户给的邮箱后台账号只用于部署环境变量，不写进仓库、不写日志。

## 3. 技术路线

### 3.1 为什么不直接用 OpenList 的 `/api/fs`

OpenList 的权限位只能表达「能否上传/删除」，无法表达「只能改/删自己上传的」。因此：

- 保留 OpenList 作为：Gin 服务、启动/配置、用户与 JWT、管理后台、驱动框架、WebDAV/S3 等底座能力。
- 新增 `internal/om` 业务层：自己的表、自己的权限、自己的存储引擎、自己的路由与页面。
- 新路由注册在 `server.Init` 的 `static.Static` 之前，避免被 SPA 的 NoRoute 吞掉。
- 不改现有 5244 的个人 OpenList 实例；新实例用独立数据目录和 5259 端口。

### 3.2 单二进制 + 服务端模板

- 一个 Go 二进制同时提供：网盘、视频、图片、微博、IM、管理后台、开放 API。
- 页面用 `html/template` + 原生 JS/CSS，嵌入二进制；不引入 Node 构建链，部署简单、启动快。
- 子域路由：同一个 5259 服务根据 `Host` 识别站点；Nginx/Cloudflare 把所有 `*.om.autozteam.cn` 反代到同一个上游。路径式 `/video`、`/image` 等也保留，方便本地和内网访问。

### 3.3 数据库

- 复用 OpenList 的用户表（`model.User`、`op.CreateUser`、JWT）。
- OM 业务表放独立 SQLite：`<data>/om.db`，GORM + `glebarez/sqlite`（已是依赖）。
- 好处：不动 OpenList 的迁移与表前缀，升级 OpenList 不冲突；用户 ID 作为外键关联。
- 生产可无缝切 PostgreSQL/MySQL：GORM 方言由 `OM_DB_DSN` 决定（P1）。

## 4. 存储引擎（核心）

### 4.1 逻辑归档包

共享盘和每个私人盘各是一条「追加型归档包」：

```
data/packs/shared.ompk
data/packs/private-<userID>.ompk
```

包内每条记录 = 一个文件内容（不是整个 tar）：

```
magic "OMB1" | flags | nonce[12] | plainSize | storedSize | sha256[32] | payload
```

- 共享：`payload = zstd(plain)`，压缩等级按上传者档位（3/5/7/9）。
- 私人：`payload = AES-256-GCM(zstd(plain))`；AAD = blob ID；每文件随机 nonce。
- 包尾不写全局索引；索引在 SQLite 的 `om_blobs`（offset/storedSize/plainSize/hash/refcount）。
- 读取时只 seek 到目标记录，解压/解密这一条；不解析整包。满足「整盘一个压缩包、按需解压、占盘最小」。

### 4.2 去重与引用计数

- `om_blobs` 以 `dedup_key` 唯一：
  - 共享：`s:<sha256>`，跨用户去重。
  - 私人：`p:<userID>:<hmac-sha256(sha, userKey)>`，只在该用户内去重，避免跨用户泄露「相同文件」。
- `om_files` 指向 `om_blob`；同内容多文件只占一份物理空间。
- 删除文件：refcount--；为 0 时标记可回收。
- 包内垃圾超过 30% 且 >256MB 时，后台重写存活记录并删旧包（单飞，避免并发读坏）。

### 4.3 按需解压缓存

- 共享文件首次播放/预览：解压到 `data/cache/<blobID>`，用 `http.ServeContent` 支持 Range；LRU 按 `OM_CACHE_BYTES`（默认 10GB）清理。
- 私人文件：不落明文缓存；解密到 `data/tmp`（0600）后响应，响应结束立即删除；大文件走流式下载。
- HTML 内联：`Content-Security-Policy: sandbox allow-scripts allow-forms`，禁止访问本站 cookie/同源接口。

### 4.4 副本与贡献节点

- `om_nodes`：节点 URL、token hash、容量、已用、状态、最后心跳、是否可承担私人分片。
- 主站写文件后异步复制：
  - 目标份数 = 上传者档位（普通 2 / Ⅰ 3 / Ⅱ 5 / Ⅲ 8，含主站 1 份）。
  - 节点不足时降级，UI 显示「当前 N/M 份」。
- 节点协议（节点只存不透明字节，无法解密私人数据）：
  - `POST /api/om/node/register`
  - `POST /api/om/node/heartbeat`
  - `PUT /api/om/node/blob/:id`
  - `GET /api/om/node/blob/:id`
  - `DELETE /api/om/node/blob/:id`
- 新节点加入后后台按容量把已有共享对象复制过去（最多到节点 80%），剩余做新空间。
- 贡献者私人额度 = 提供容量 × 50% × 90%（扣 10% 运维），离线 30 天回收；头衔自动授予。

## 5. 账号、邮箱、权益

### 5.1 注册/登录

1. `POST /api/om/auth/register/request`：用户名/邮箱/密码 → 生成 6 位验证码，存哈希与 10 分钟过期，调用 SkyMail 发信。
2. `POST /api/om/auth/register/verify`：校验验证码 → 创建 OpenList 用户 → 创建 `om_profiles` → 返回 JWT。
3. 登录复用 `POST /api/auth/login`；OM 页面用 HttpOnly Cookie 保存 token，API 也接受 `Authorization`。
4. 邮箱唯一；用户名唯一；密码至少 8 位；登录限流复用 OpenList。

SkyMail：
- 环境变量：`OM_SKYMAIL_BASE`、`OM_SKYMAIL_EMAIL`、`OM_SKYMAIL_PASSWORD`、`OM_SKYMAIL_ACCOUNT_ID`（可空，启动时 `/api/account/list` 自动取第一个）。
- 未配置时注册接口返回明确错误，不静默放行；管理员可先用初始 admin 登录后台建用户。

### 5.2 档位与配额

| 档位 | 价格 | 私人额度 | 副本份数 | 压缩等级 | 下载 |
|---|---|---|---|---|---|
| 普通 | 0 | 1GB | 2 | 7 | CDN |
| Ⅰ | 15 元 | 11GB | 3 | 5 | CDN |
| Ⅱ | 50 元 | 61GB | 5 | 5 | 主站 |
| Ⅲ | 100 元 | 121GB | 8 | 3 | 主站 |

- 权益永久；兑换码写入 `om_redeem_codes`，兑换后写 `om_profiles.tier` 与头衔。
- 预留包：10/50/100GB/年，写 `om_reservations`，只保证不被共享挤占，不预分配物理空间。
- 动态分配：未使用空间全部给共享；私人额度是上限，不做预留；上传时实时检查「共享已用 + 私人已用 + 预留 ≤ 磁盘容量」。
- 贡献额度加到 `om_profiles.contribution_quota_bytes`，与捐赠额度叠加。

## 6. 子站功能

### 6.1 视频站

- 上传：先上传文件（`scope=shared, kind=video`），再 `POST /api/om/videos` 建元数据；封面可选。
- 播放：`<video>` 指向 `/f/:fileID/:name`；服务端 Range + 缓存。
- 互动：点赞、投币（新用户 5 币，投币扣币并给作者加币）、收藏、评论（支持回复）、弹幕（按时间轴）、转发到动态。
- 播放信息：`om_watch_progress` 记录位置、时长、最后播放时间；首页「继续观看」。
- 推荐：`score = views + likes*5 + coins*10 + favorites*3`，按时间衰减；支持最新/最热/分类/标签。
- 页面：首页、分区、搜索、播放页、投稿页、个人主页。

### 6.2 图片站

- 瀑布流列表、详情大图、标签、作者、点赞、收藏、评论、日/周/月排行。
- 复用通用互动表与评论表，`target_type=image`；无弹幕。
- 上传支持多图；第一张为封面；原图直链。

### 6.3 微博站

- 正文只存主库；图片/视频文件进共享盘，`om_post_media` 关联。
- 发帖、转发（带评论）、评论、点赞、关注、话题（`#话题#` 解析）、个人时间线、关注流。
- 单条正文上限 2000 字；媒体最多 9 个。

### 6.4 IM

- 公共大厅：
  - 聊天记录写入共享盘 `im/halls/<hallID>.jsonl`，同时写 `om_messages` 便于分页。
  - 默认公开可读、可申请加入；房主可设 `join_policy=open/approval/closed`、`read_policy=public/members`。
  - 所有人可见历史（除非房主设为 members）。
- 私人群：
  - 聊天记录写入创建者私人盘 `im/groups/<groupID>.jsonl`，AES-GCM 加密。
  - 只有成员能读；服务端校验成员关系后解密返回。
- 实时：`/api/om/im/ws?room=hall:<id>`，gorilla/websocket；消息先落库/落盘再广播。
- 界面：房间列表、消息流、输入框、成员/设置；首版简易但可用。
- TailchatPlus：交付 `docs/tailchatplus-integration-prompt.md`，要求把 MinIO 替换为 OM 存储 API，聊天记录镜像到大厅/私人群文件。

## 7. 权限模型

| 操作 | 共享盘 | 私人盘 |
|---|---|---|
| 查看 | 所有登录用户；直链可公开 | 仅本人（API Key 同权） |
| 上传 | 所有登录用户 | 仅本人，且未超配额 |
| 修改/重命名/移动 | 仅上传者 | 仅本人 |
| 删除 | 仅上传者（管理员可删） | 仅本人（管理员可删） |
| 在线执行/预览 | 图片/视频/音频/静态 HTML | 同上 |
| 下载 | 所有人 | 仅本人/签名链接 |

- 管理员可管理用户、节点、兑换码、审核、删除违规内容；管理员操作写审计日志。
- 文件页显示上传者、上传时间、大小、MIME、当前副本数、直链。
- 用户主页显示头像、简介、头衔、公开文件/视频/图片/动态。

## 8. 数据模型（`internal/om/model`）

- `om_profiles`：user_id PK、email、email_verified、display_name、bio、avatar_file_id、tier、private_enabled、private_quota_bytes、private_used_bytes、contribution_quota_bytes、compression_level、created_at。
- `om_pending_registrations`：id、username、email、password_hash、code_hash、expires_at、attempts。
- `om_user_keys`：user_id PK、wrapped_key、nonce。
- `om_packs`：id、scope、owner_id、path、size、waste、created_at。
- `om_blobs`：id、pack_id、offset、stored_size、plain_size、sha256、dedup_key、encrypted、ref_count、created_at。
- `om_files`：id、owner_id、scope、parent_id、name、path、size、mime、kind、blob_id、is_dir、visibility、created_at、updated_at。
- `om_titles` / `om_user_titles`。
- `om_videos`、`om_images`、`om_posts`、`om_post_media`。
- `om_reactions`：user_id、target_type、target_id、kind、value；唯一索引。
- `om_comments`：target_type、target_id、user_id、parent_id、content、like_count。
- `om_danmaku`：video_id、user_id、time_ms、mode、color、content。
- `om_watch_progress`：user_id、video_id、position、duration、updated_at。
- `om_wallets`：user_id、coins。
- `om_follows`：follower_id、followee_id。
- `om_halls`、`om_hall_members`、`om_groups`、`om_group_members`、`om_messages`。
- `om_api_keys`：user_id、name、prefix、hash、scopes、last_used_at。
- `om_nodes`、`om_replicas`、`om_contribution_events`。
- `om_redeem_codes`、`om_redemptions`、`om_reservations`。
- `om_audit_logs`、`om_settings`。

## 9. 路由

页面（HTML）：
- `/` 首页（按子域切换站点）
- `/login`、`/register`、`/u/:username`、`/settings`
- `/drive`、`/drive/private`、`/f/:id/:name`、`/dl/:id/:name`
- `/video`、`/video/:id`、`/video/upload`
- `/image`、`/image/:id`、`/image/upload`
- `/weibo`、`/weibo/:id`
- `/im`、`/im/hall/:id`、`/im/group/:id`
- `/admin`、`/admin/users`、`/admin/nodes`、`/admin/codes`

API：
- `POST /api/om/auth/register/request`、`/verify`
- `GET /api/om/me`、`POST /api/om/me`
- `GET /api/om/files?scope=&parent=`、`POST /api/om/files/upload`、`POST /api/om/files/folder`、`POST /api/om/files/rename`、`POST /api/om/files/move`、`POST /api/om/files/delete`
- `GET /api/om/videos`、`GET /api/om/videos/:id`、`POST /api/om/videos`、`POST /api/om/videos/:id/react`、`POST /api/om/videos/:id/comment`、`POST /api/om/videos/:id/danmaku`、`POST /api/om/videos/:id/progress`
- `GET /api/om/images`、`GET /api/om/images/:id`、`POST /api/om/images`、`POST /api/om/images/:id/react`、`POST /api/om/images/:id/comment`
- `GET /api/om/posts`、`POST /api/om/posts`、`POST /api/om/posts/:id/repost`、`POST /api/om/posts/:id/react`、`POST /api/om/posts/:id/comment`、`POST /api/om/follow`
- `GET /api/om/im/rooms`、`POST /api/om/im/halls`、`POST /api/om/im/halls/:id/join`、`GET /api/om/im/messages`、`WS /api/om/im/ws`
- `GET /api/om/keys`、`POST /api/om/keys`、`POST /api/om/keys/:id/delete`
- `GET /api/v1/files` 等开放 API（`X-API-Key`）
- `POST /api/om/node/register|heartbeat|blob/:id`
- 管理：`/api/om/admin/stats|users|nodes|codes|audit`

响应沿用 OpenList 的 `{code,message,data}` 包装；分页 `{page,page_size,total,items}`。

## 10. 部署与上线

### 10.1 目录

```
/opt/ourmemory/
  ourmemory            # 主程序
  om-node              # 贡献节点程序（可选）
  data/
    config.json        # OpenList 配置（5259）
    data.db            # OpenList 用户/设置
    om.db              # OM 业务表
    packs/             # 共享/私人归档包
    cache/             # 按需解压缓存
    tmp/
  web/                 # 可选模板覆盖（默认嵌入）
  .env                 # 600，主密钥/邮箱/域名
```

### 10.2 systemd

`ourmemory.service`：
- `WorkingDirectory=/opt/ourmemory`
- `ExecStart=/opt/ourmemory/ourmemory server --data /opt/ourmemory/data`
- `EnvironmentFile=/opt/ourmemory/.env`
- `Restart=always`
- `LimitNOFILE=1048576`

### 10.3 反代

- 应用只监听 `127.0.0.1:5259`（`config.json` 的 `scheme.address=127.0.0.1`、`http_port=5259`）。
- 云服务器 81.70.77.4：Nginx 监听 80/443，把 `om.autozteam.cn`、`video.om...`、`image.om...`、`weibo.om...`、`im.om...` 全部反代到隧道出口/本机 5259；`client_max_body_size 0`，`proxy_request_buffering off`，WebSocket upgrade，`X-Forwarded-Proto`。
- 推荐 Cloudflare Tunnel：`cloudflared tunnel --url http://127.0.0.1:5259` 或命名隧道；DNS 加 CNAME 到 tunnel；TLS 由 Cloudflare 终止。二级子域需要 Total TLS 或 `*.om` 高级证书。
- 现有 hp-lite 若修复，则云侧 Nginx 反代到 hp-lite 映射端口；上线门禁是 `/ping` 公网可达 + 上传/播放通过。

### 10.4 DNS

| 记录 | 类型 | 值 |
|---|---|---|
| `om` | A/CNAME | 81.70.77.4 或 Tunnel CNAME |
| `video.om` | CNAME | `om.autozteam.cn`（或 Tunnel） |
| `image.om` | CNAME | 同上 |
| `weibo.om` | CNAME | 同上 |
| `im.om` | CNAME | 同上 |
| `*`（可选） | CNAME | 同上 |

### 10.5 主密钥

- `OM_MASTER_KEY`：32 字节随机数的 base64；首次部署生成，写入 `.env` 600。
- 丢失 = 私人盘不可恢复；备份到用户密码管理器/离线介质。
- 私人盘用户密钥：随机 32 字节，AES-GCM 包裹后存 `om_user_keys`；解密只在内存。

## 11. 安全

- 私人数据信封加密；节点与备份拿到的是密文。
- 上传：扩展名 + MIME 双校验；拒绝 `.exe/.dll/.so/.bat/.cmd/.ps1/.sh/.py/.pyc/.jar/.msi/.com/.scr/.vbs`；`.js/.css` 仅作为 HTML 静态资源，不服务端执行。
- 文件名清洗；路径穿越拒绝；SQL 全参数化；模板自动转义。
- HTML 预览 sandbox；禁止 `allow-same-origin`；CSP 禁止外链脚本可选。
- 登录/注册/上传/评论限流；API Key 只存 hash，明文只显示一次。
- 管理操作审计；敏感配置只从 env 读，不写日志。
- AGPL：仓库公开 + 页脚 Source 链接；部署说明附源码地址。

## 12. 里程碑

### M0（已完成）
- OpenList fork、Go 工具链、底座编译通过（`bin/openlist-base.exe`）。

### M1 核心网盘（P0）
- OM 配置/DB/迁移、存储引擎、注册登录、文件上传/列表/重命名/删除、共享/私人盘、预览/下载、个人主页、头衔、API Key。
- 验收：两个账号；A 上传共享文件 B 可见但不能删；A 上传私人文件 B 不可见；私人大文件加密；服务重启后数据仍在。

### M2 子站（P0）
- 视频、图片、微博、IM 大厅/私群、WebSocket。
- 验收：上传视频可播放/拖进度/发弹幕；图片点赞收藏评论；微博发文字带图；IM 双浏览器实时收发，私群非成员 403。

### M3 权益与节点（P0）
- 兑换码、档位、预留包、贡献节点注册/心跳/复制、贡献者额度/头衔、om-node 安装器。
- 验收：兑换 Ⅰ 级后额度 11GB、副本目标 3；节点上线后收到副本；节点离线 30 天任务标记回收。

### M4 上线（P0）
- systemd、Nginx/Tunnel、DNS、备份脚本、健康检查、首启向导、管理员账号。
- 验收：公网 `https://om.autozteam.cn/ping` 返回 pong；注册收信；上传 1GB 文件；视频 Range 播放；子域可访问。

### M5 迭代（P1）
- TailchatPlus 接入、转码、支付、推荐、移动端优化、对象存储后端。

## 13. 风险与待用户动作

1. **隧道断线**：必须修 hp-lite 或提供 Cloudflare Tunnel token；否则只能内网访问。上线门禁。
2. **二级子域证书**：Cloudflare Universal SSL 不覆盖 `*.om`；需开 Total TLS 或上传 `*.om` 证书。
3. **磁盘只有 5GB 可用**：首版共享盘容易满；需要尽快接入贡献节点或外挂盘。
4. **SkyMail 发件账号/accountId**：部署时用 env 配置；若 API 限制发件域名，需要用户在后端加白名单。
5. **GitHub 推送凭据**：服务器 SSH 经代理 400；需要可用的 HTTPS token 或修代理；否则源码先留在本地/服务器，暂不公开。
6. **主密钥备份**：用户必须保存；丢失私人盘无法恢复。

## 14. 验收清单（上线前逐项打勾）

- [ ] `curl http://127.0.0.1:5259/ping` → `pong`
- [ ] 管理员登录，创建普通用户
- [ ] 注册验证码真实收到邮件
- [ ] 共享上传/下载/预览/改删权限正确
- [ ] 私人开通、加密包落盘、非本人 403
- [ ] 视频 Range/进度/弹幕/点赞/投币/收藏/评论
- [ ] 图片上传/点赞/收藏/评论/排行
- [ ] 微博文字+媒体、转发/评论/关注
- [ ] IM 公共大厅/私群、WebSocket、历史文件落盘
- [ ] API Key 读写、越权 403
- [ ] 兑换码/档位/配额/预留包
- [ ] 节点注册/心跳/副本/离线回收
- [ ] `systemctl restart ourmemory` 后数据完好
- [ ] 备份/恢复演练
- [ ] 公网域名/子域/HTTPS/上传大小/WebSocket 通过
- [ ] 页脚源码链接与 AGPL 说明

## 15. 交付物

- 可执行文件：`ourmemory`、`om-node`
- 源码：本仓库（公开后附 AGPL）
- 文档：`docs/PLAN.md`、`docs/DEPLOY.md`、`docs/API.md`、`docs/tailchatplus-integration-prompt.md`、`docs/OPERATIONS.md`
- 脚本：`scripts/install-node.sh`、`scripts/backup.sh`、`scripts/smoke-test.sh`
- 配置样例：`deploy/ourmemory.service`、`deploy/nginx-om.conf`、`deploy/env.example`
- 初始化：管理员账号、示例头衔、示例公共大厅
