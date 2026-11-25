# 给 TailchatPlus 侧 Agent 的接入提示词

把下面整段内容发给负责 `D:\project\tailchatplus\repo`（服务器 `/opt/tcplus/tailchat`、插件 `/opt/tcplug`）的 Agent。它假设对方没有 ourmemory 的上下文。

---

## 任务

把 TailchatPlus 的群组资源存储与聊天记录归档接入 **ourmemory**（`https://om.autozteam.cn`），不再把新资源写入 TailchatPlus 现有的 MinIO。

ourmemory 是一个基于 OpenList 的共享存储平台：

- **共享盘（shared）**：所有登录/匿名访问者可见，适合公共大厅、公共群、公开附件。
- **私人盘（private）**：每个用户一个 AES-256-GCM 加密归档，只有本人（以及被授权读取签名链接的人）能取到内容；适合私人群附件与私人群聊天归档。
- 聊天记录在 ourmemory 中按「一个房间一个 JSONL 文件」归档：
  - 公共大厅 → 共享盘 `/halls/hall-<id>.jsonl`
  - 私人群 → 群主私人盘 `/groups/<id>.jsonl`

目标不是替换 TailchatPlus 的 Mongo 热数据，而是：

1. 新上传的群文件/附件进入 ourmemory，不再进入 MinIO；
2. 每条聊天消息同步到 ourmemory 的 IM 归档（或至少保证归档文件持续更新）；
3. 公开群/大厅走共享盘，私人群走**群主**私人盘；
4. 现有 MinIO 数据可迁移，迁移完成前不删除；
5. 不泄露 API Key，不把私人盘内容公开。

## 现有代码约束

TailchatPlus 是 Tailchat fork：

- 服务端 Moleculer `TcService`，Mongo + Redis；
- 插件目录：`server/plugins`、`client/web/plugins`；
- 插件 ID 使用反域名，例如 `com.autoz.omstorage`；
- 使用现有 OpenAPI/类型生成流程；不要手写重复类型；
- 遵循仓库 `AGENTS.md`：不泄露密钥、分布式一致性、测试与构建要求；
- 不要修改 ourmemory 代码；只改 TailchatPlus 侧，通过 HTTP API 对接。

## ourmemory API 契约

Base URL 通过环境变量 `OM_BASE_URL` 配置（生产 `https://om.autozteam.cn`）。所有请求 JSON 或 multipart；成功响应 `{"code":200,"data":...}`，失败 `{"code":4xx,"message":"..."}`。

### 认证

两种方式：

1. 用户 API Key：`X-API-Key: om_...`（也接受 `Authorization: Bearer om_...`）。API Key 权限等于创建它的用户。
2. 公共归档服务账号：在 ourmemory 建一个专用账号 `tc-archive`，创建 API Key，仅用于公共大厅/公开群的共享盘归档。私人群**必须**使用群主自己的 API Key，不能用服务账号代替。

每个 Tailchat 用户需要在插件里绑定 ourmemory：

- 用户在 ourmemory「设置 → API Key」创建 key；
- 在 Tailchat 插件设置里粘贴 username + API Key；
- 服务端调用 `GET /api/v1/me` 验证，成功后用 `TC_OM_ENCRYPTION_KEY` 派生的密钥加密存 Mongo；
- 前端永远不拿明文 API Key；下载/上传都由服务端插件代调。

### 用户与私人盘

```http
GET  /api/v1/me
GET  /api/om/private
POST /api/om/private/enable
```

创建私人群前，群主必须已 `private.enabled=true`。未开通时插件提示「请先在 ourmemory 开通私人存储」，不要静默把私人群降级到共享盘。

### 文件

```http
POST /api/v1/files/upload        multipart: scope=shared|private, parent=/tc/<groupId>, file=@...
GET  /api/v1/files?scope=shared&parent=/tc/<groupId>
GET  /api/v1/files/<file-id>
POST /api/v1/files/<file-id>/sign   -> {"url":"/s/<token>","expires_at":...}
```

- 公共群/大厅：`scope=shared`，路径 `/tc/public/<groupId>/...`。
- 私人群：`scope=private`，使用**群主**的 API Key，路径 `/tc/private/<groupId>/...`。
- 附件消息在 Tailchat Mongo 中保存 `om_file_id`、`om_scope`、`om_uploader_id`、原始文件名/MIME/大小；展示 URL：
  - shared：`https://om.autozteam.cn/f/<file-id>/<filename>`（带扩展名，可直接 CDN 缓存）；
  - private：服务端用群主 key 调 `sign`，把短期 `/s/<token>` 拼成绝对 URL 给客户端；TTL 建议 10 分钟，不落库。
- 不要在前端直接持有群主 key。私人群的签名下载由插件服务端完成，并在签名前校验请求者是该群成员。

### IM 归档

```http
GET  /api/om/halls
POST /api/om/halls                  {"name":"...","description":"...","join_policy":"open","read_policy":"public"}
POST /api/om/halls/<id>/join
POST /api/om/halls/<id>/messages    {"content":"...","kind":"text","file_id":""}
GET  /api/om/halls/<id>/messages

POST /api/om/groups                 {"name":"..."}
POST /api/om/groups/<id>/members    {"username":"..."}
POST /api/om/groups/<id>/messages   {"content":"...","kind":"text|image|file","file_id":"..."}
GET  /api/om/groups/<id>/messages
```

- 公共大厅/公开群：`POST /api/om/halls/<id>/messages`，使用 `tc-archive` key 或任意成员 key；om 会把整房间重写为共享盘 `/halls/hall-<id>.jsonl`。
- 私人群：`POST /api/om/groups/<id>/messages`，必须用**群主** key；om 会把整房间重写为群主私人盘 `/groups/<id>.jsonl`。
- 消息发送失败不应阻塞 Tailchat 主流程：写一个 outbox（Mongo 集合 `om_archive_outbox`），后台 worker 重试；记录 `tc_message_id` 去重，避免重复归档。
- Tailchat 自己的 Mongo 仍是消息热存储与已读/搜索来源；ourmemory 是归档与跨站可读副本。不要把 ourmemory 当消息队列。

### 映射表

在 Tailchat Mongo 新增（或用插件 collection）：

```text
om_user_bindings:   { tcUserId, omUserId, omUsername, apiKeyEnc, createdAt, updatedAt }
om_room_bindings:   { tcRoomId, omRoomType: "hall"|"group", omRoomId, scope: "shared"|"private", ownerTcUserId, createdAt }
om_archive_outbox:  { tcMessageId, tcRoomId, payload, attempts, nextRetryAt, status }
```

唯一索引：`tcUserId`、`tcRoomId`、`tcMessageId`。

### 创建/删除群

- 创建公开群：建 Tailchat group 后，用 `tc-archive` key 调 `POST /api/om/halls`（`join_policy=open`、`read_policy=public`），保存映射。
- 创建私人群：校验群主绑定 + `private.enabled`，用群主 key 调 `POST /api/om/groups`，保存映射；拉人时同步 `POST /api/om/groups/<id>/members`。
- 删除群：只解除映射，不删除 ourmemory 文件（归档保留）；如需删除，走管理员审核，不要由普通群主直接物理删除。
- 修改群名：可不同步；ourmemory 侧名称仅作归档标识。

## MinIO 迁移

1. 扫描 MinIO bucket，按对象前缀/消息记录判断归属房间。
2. 公共房间对象上传到 `shared`；私人群对象上传到群主 `private`（需要群主已绑定 key；未绑定的先挂起并报告）。
3. 校验 SHA-256/大小；把消息里的 URL 改写为 `om_file_id`，保留旧 URL 作为 fallback。
4. 双读一段时间：新消息写 OM，旧消息可读 MinIO。
5. 确认无遗漏后停止 MinIO 写入；**不要立即删 bucket**，至少保留 30 天只读。
6. 输出迁移报告：成功/跳过/失败数量、未绑定用户列表、失败对象清单。

## 失败与边界

- ourmemory 401：API Key 失效，标记绑定为 `invalid`，提示用户重新绑定；不要重试到死。
- 403：私人盘未开通/非群成员/非群主；按语义提示，不降级到共享盘。
- 413/容量满：提示「共享盘或私人盘容量不足」，不要吞掉附件。
- 5xx/网络：outbox 指数退避（1m/5m/30m/2h，最多 8 次），记录指标。
- 私人群附件签名 URL 过期：客户端重新向插件请求，不缓存永久链接。
- 公共群内容一旦进共享盘就是公开的；创建群时必须明确告知用户。

## 交付物

1. `server/plugins/omstorage` 服务端插件：绑定、上传、签名、归档 outbox、迁移命令。
2. `client/web/plugins/omstorage` 设置页：绑定/解绑、私人盘状态、归档状态。
3. 环境变量：`OM_BASE_URL`、`TC_OM_ARCHIVE_KEY`（仅公共归档）、`TC_OM_ENCRYPTION_KEY`（32 字节）。
4. 迁移脚本与 dry-run 模式。
5. 单元测试 + 一个集成测试（mock OM API）：公共群/私人群各一条上传+消息+签名。
6. `docs/omstorage.md`：部署、配置、迁移、回滚。
7. PR 描述写清：未改 ourmemory；未删除 MinIO 数据；测试实际跑了什么。

## 验收

- 新公开群附件只出现在 `https://om.autozteam.cn/f/...`，MinIO 无新增对象。
- 新私人群附件只出现在群主私人盘；非成员拿不到可读 URL；群主能在 ourmemory `/files?scope=private` 看到 `/tc/private/<groupId>/...`。
- 公共大厅/公开群消息在共享盘 `/halls/hall-<id>.jsonl` 可见。
- 私人群消息在群主私人盘 `/groups/<id>.jsonl` 可见，其他用户不可见。
- 断网/OM 5xx 时 Tailchat 消息仍可发送，outbox 恢复后补归档，无重复。
- 迁移 dry-run 报告数量与实际对象数一致。
