# ourmemory API

Base URL：`https://om.autozteam.cn`

## 认证

### 浏览器会话

登录后服务端设置 HttpOnly Cookie `om_token`。所有 `/api/om/*` 请求自动携带。

### API Key（第三方应用）

在「设置 → API Key」创建，明文只显示一次。请求头任选：

```http
X-API-Key: om_xxxxxxxxxxxxxxxx
```

或：

```http
Authorization: Bearer om_xxxxxxxxxxxxxxxx
```

`/api/v1/*` 与 `/api/om/*` 都接受 API Key；API Key 对应创建它的用户，权限与本人相同。

### 登录

```http
POST /api/om/auth/login
Content-Type: application/json

{"username":"admin","password":"..."}
```

返回：

```json
{"code":200,"data":{"token":"...","user":{"id":1,"username":"admin","role":2}}}
```

### 注册（邮箱验证码）

```http
POST /api/om/auth/register/request
{"username":"alice","email":"a@example.com","password":"12345678"}
```

```http
POST /api/om/auth/register/verify
{"username":"alice","email":"a@example.com","password":"12345678","code":"123456"}
```

验证码 10 分钟有效，6 位数字。`OM_DEBUG=1` 时验证码写入服务日志。

## 文件

路径参数 `scope`：`shared`（共享盘，默认）或 `private`（私人盘，需登录并开通）。

### 列表

```http
GET /api/om/files?scope=shared&parent=
```

返回 `items`，每项：

```json
{"file":{"id":"...","name":"a.txt","path":"/a.txt","scope":"shared","size":52,"kind":"file","mime":"text/plain","uploader_id":1},"uploader":{"id":1,"username":"admin"}}
```

### 上传

```http
POST /api/om/files/upload
Content-Type: multipart/form-data

scope=shared
parent=/somedir
file=@a.mp4
```

返回 `{file, blob}`；后续引用文件用 `file.id`。

禁止上传 `.exe/.sh/.py/.js` 等可执行扩展名（服务端有黑名单）；允许图片、视频、音频、HTML、压缩包、文档等。

### 新建目录 / 改名 / 移动 / 删除

```http
POST /api/om/folders      {"scope":"shared","parent":"/","name":"docs"}
POST /api/om/files/rename {"id":"...","name":"new.txt"}
POST /api/om/files/move   {"ids":["..."],"parent":"/docs"}
POST /api/om/files/delete {"ids":["..."]}
```

只能改名/移动/删除自己上传的文件；管理员可操作全部。

### 直链

| 链接 | 说明 |
| --- | --- |
| `GET /f/<file-id>` / `GET /f/<file-id>/<filename>` | 仿直链，共享文件可匿名；私人文件需登录 Cookie。带扩展名的形式可被 Cloudflare 按文件类型缓存 |
| `GET /dl/<file-id>` / `GET /dl/<file-id>/<filename>` | `Content-Disposition: attachment` |
| `POST /api/om/files/<id>/sign` | 生成带过期时间的签名链接 `{"url":"/s/..."}` |

共享文件响应 `Cache-Control: public, max-age=86400`，适合 Cloudflare CDN；私人文件 `private, no-store`。

## 私人盘

```http
GET  /api/om/private
POST /api/om/private/enable
```

`private` 返回 `enabled/quota_bytes/used_bytes/compression_level`。首次开通按档位设置压缩等级（普通 7、Ⅰ/Ⅱ 5、Ⅲ 3）。

## 用户与头衔

```http
GET  /api/om/u/<username>
POST /api/om/profile       {"display_name":"...","bio":"...","avatar_file_id":"..."}
GET  /api/om/titles        ?username=alice
POST /api/om/redeem        {"code":"..."}
```

## 视频

```http
GET  /api/om/videos?q=&category=&sort=new|hot|like&page=1
GET  /api/om/videos/<id>
POST /api/om/videos        {"file_id":"...","cover_file_id":"...","title":"...","description":"...","category":"...","tags":"..."}
POST /api/om/videos/<id>/react    {"kind":"like|coin|favorite"}
GET  /api/om/videos/<id>/comments
POST /api/om/videos/<id>/comments {"content":"..."}
GET  /api/om/videos/<id>/danmaku
POST /api/om/videos/<id>/danmaku  {"time":12.5,"text":"...","color":"#fff","position":"right"}
POST /api/om/videos/<id>/progress {"position":123.4}
GET  /api/om/videos/<id>/progress
```

视频/封面必须来自共享盘；`kind` 为 `video` 或 `audio`。

## 图片

```http
GET  /api/om/images?q=&page=1
GET  /api/om/images/<id>
POST /api/om/images        {"file_id":"...","title":"...","description":"...","tags":"..."}
POST /api/om/images/<id>/react
```

图片必须来自共享盘。

## 微博

```http
GET  /api/om/posts?user=&q=&page=1
GET  /api/om/posts/<id>
POST /api/om/posts         {"content":"文字","media_file_ids":["..."]}
POST /api/om/posts/<id>/react    {"kind":"like"}
POST /api/om/posts/<id>/repost   {"content":"转发语"}
GET  /api/om/posts/<id>/comments
POST /api/om/posts/<id>/comments {"content":"..."}
POST /api/om/follow/<user-id>
```

正文只存主库；媒体只接受共享盘文件。

## IM

### 公共大厅

```http
GET  /api/om/halls
POST /api/om/halls            {"name":"...","description":"...","join_policy":"open|approval|closed","read_policy":"public|members"}
POST /api/om/halls/<id>/join
GET  /api/om/halls/<id>/messages
POST /api/om/halls/<id>/messages {"content":"...","kind":"text","file_id":""}
POST /api/om/halls/<id>/settings {"join_policy":"...","read_policy":"..."}
```

第一次访问会自动创建「主大厅」。公共大厅的聊天记录镜像为共享盘中的 `/halls/hall-<id>.jsonl`，所有人可见。

### 私人群

```http
GET  /api/om/groups
POST /api/om/groups                 {"name":"..."}
POST /api/om/groups/<id>/members    {"username":"alice"}
GET  /api/om/groups/<id>/messages
POST /api/om/groups/<id>/messages   {"content":"...","file_id":""}
```

创建者必须先开通私人存储。私人群记录写入创建者私人盘的 `/groups/<id>.jsonl`，非成员不可读。

## 开放存储 API

`/api/v1` 是给第三方应用的稳定前缀，参数与 `/api/om` 相同：

```http
GET  /api/v1/me
GET  /api/v1/files?scope=shared
POST /api/v1/files/upload
POST /api/v1/folders
POST /api/v1/files/rename
POST /api/v1/files/move
POST /api/v1/files/delete
GET  /api/v1/files/<id>
POST /api/v1/files/<id>/sign
```

## 贡献节点

节点注册需要 `X-Node-Secret: $OM_NODE_SECRET`；其他节点接口使用注册返回的 `token` 作为 `X-Node-Token`。

```http
POST /api/om/nodes/register  {"name":"node1","capacity_bytes":10737418240,"private_capable":true,"username":"alice"}
POST /api/om/nodes/heartbeat {"used_bytes":0}
POST /api/om/nodes/assign?limit=10
GET  /api/om/nodes/blob/<blob-id>
POST /api/om/nodes/report   {"blob_id":"...","ok":true,"error":""}
```

贡献额度 = `capacity_bytes × 50% × 90%`，计入贡献者私人配额并授予「存储贡献者」头衔。节点只保存不透明字节；私人块为 AES-GCM 密文。

## 错误格式

```json
{"code":400,"message":"参数错误"}
```

HTTP 状态码同时反映错误：400 参数、401 未登录、403 无权限、404 不存在、500 服务端错误。

## 限流与上限

- 单文件默认上限 `OM_MAX_UPLOAD`（默认 2 GiB）。
- 共享盘默认上限 5 GiB（首版）；私人盘按档位。
- IM 单条消息 2000 字；视频/图片列表每页 24；动态每页 30。
- 上传是流式落临时文件后入库，不会把大文件读进内存。

## 第三方应用接入审核

1. 用 API Key 调 `/api/v1` 完成存储读写。
2. 提交应用名称、域名、用途、隐私说明到管理员。
3. 审核通过后加入「官方支持」列表；不得绕过用户权限、不得把私人盘内容公开展示。
