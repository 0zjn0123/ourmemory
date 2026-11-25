# 上线部署：DNS、Cloudflare Tunnel 与反向代理

目标：`om.autozteam.cn` 指向本地服务器 `192.168.1.11:5259`，并让 `video.om`、`image.om`、`weibo.om`、`im.om` 进入同一应用。

> 现有 hp-lite 隧道在 2026-11-22 已持续报 `dial tcp 43.136.14.67:16666: connect: connection timed out`，不要把它当作唯一上线路径。推荐 **Cloudflare Tunnel**；它不需要云服务器开放 80/443，也不需要改 1Panel 配置。

## 1. DNS 记录（Cloudflare）

在 `autozteam.cn` 区域添加：

| 类型 | 名称 | 内容 | 代理状态 |
| --- | --- | --- | --- |
| A | `om` | `81.70.77.4` | 已代理（橙色云） |
| A | `*.om` | `81.70.77.4` | 已代理（橙色云） |

- `*.om` 覆盖 `video.om`、`image.om`、`weibo.om`、`im.om`。
- 如果 Cloudflare 不允许直接添加 `*.om`，逐条添加同名 A 记录即可。
- **不要**只依赖 Universal SSL：它只覆盖 `autozteam.cn` 与 `*.autozteam.cn`（一级），不覆盖 `video.om.autozteam.cn`（二级）。在 SSL/TLS → Edge Certificates 开启 **Total TLS**，或为 `*.om.autozteam.cn` 购买 Advanced Certificate。
- 若暂时无法开启 Total TLS，可先用一级域名：`video-om.autozteam.cn` 等（`*.autozteam.cn` 已覆盖），应用侧同样支持 `OM_BASE_HOST` 之外的 Host，只需把访问入口指向 5259。

## 2. Cloudflare Tunnel（推荐）

在 Cloudflare Zero Trust → Networks → Tunnels 创建隧道 `ourmemory`，添加两个 Public Hostname：

| Subdomain | Domain | Service |
| --- | --- | --- |
| `om` | `autozteam.cn` | `http://127.0.0.1:5259` |
| `*` | `om.autozteam.cn` | `http://127.0.0.1:5259` |

在 192.168.1.11 上安装 cloudflared：

```bash
# Debian/Ubuntu amd64
curl -L --output /tmp/cloudflared.deb \
  https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb
dpkg -i /tmp/cloudflared.deb

# 用控制台生成的 token 安装为开机自启服务
cloudflared service install <TUNNEL_TOKEN>
systemctl enable --now cloudflared
systemctl status cloudflared
```

或者用本地配置（把 `<UUID>` 和 `<TUNNEL_TOKEN>` 换成实际值）：

```yaml
# /etc/cloudflared/config.yml
tunnel: <UUID>
credentials-file: /etc/cloudflared/<UUID>.json
ingress:
  - hostname: om.autozteam.cn
    service: http://127.0.0.1:5259
  - hostname: "*.om.autozteam.cn"
    service: http://127.0.0.1:5259
  - service: http_status:404
```

```bash
cloudflared tunnel run <UUID>
```

验证：

```bash
curl -I https://om.autozteam.cn/
curl -I https://video.om.autozteam.cn/
curl -s https://om.autozteam.cn/api/om/me
```

预期 `/` 返回 200 `text/html`，`/api/om/me` 返回 `{"code":200,"data":null}`。

## 3. 备选：云服务器 Nginx 反代

仅当隧道已经把云服务器某个端口（示例 `8080`）转发到 `192.168.1.11:5259` 时使用。不要修改 1Panel 自带配置；新建独立文件，例如 `/etc/nginx/conf.d/ourmemory.conf`。

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 80;
    listen [::]:80;
    server_name om.autozteam.cn *.om.autozteam.cn;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name om.autozteam.cn *.om.autozteam.cn;

    ssl_certificate     /etc/ssl/ourmemory/fullchain.pem;
    ssl_certificate_key /etc/ssl/ourmemory/privkey.pem;

    client_max_body_size 8g;
    proxy_request_buffering off;
    proxy_buffering off;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
```

证书可以用 Cloudflare Origin Certificate（覆盖 `*.om.autozteam.cn`）或 Let's Encrypt DNS-01。Cloudflare 代理开启时，源站 443 只接受 Cloudflare IP 更安全。

若使用 Nginx 而不是 Tunnel，需要修好 hp-lite 或换成其他端口映射；当前 hp-lite 连接超时，不能直接上线。

## 4. 服务器环境

`/opt/ourmemory/ourmemory.env` 至少要包含：

```dotenv
OM_BASE_HOST=om.autozteam.cn
OM_BASE_URL=https://om.autozteam.cn
OM_SHARED_MAX_BYTES=5368709120
OM_CACHE_BYTES=10737418240
OM_NODE_SECRET=<NODE_SECRET.txt>
OM_DEBUG=0
OPENLIST_SITE_URL=https://om.autozteam.cn
```

- `OM_DEBUG=1` 时注册验证码写日志；生产在 SkyMail 配好前可临时保留，配好后改 `0`。
- SkyMail 使用已解析的 `cm.autozteam.cn`（Cloudflare 上的 Cloud Mail 实例）：
  `OM_SKYMAIL_BASE=https://cm.autozteam.cn`、`OM_SKYMAIL_EMAIL=admin@autozteam.cn`（仅用于登录取 token）、
  `OM_SKYMAIL_ACCOUNT_ID=5`（发件邮箱 `noreply@autozteam.cn`），密码写 `OM_SKYMAIL_PASSWORD`。
- 验证码邮件的 From 是 `noreply@autozteam.cn`；不要把 `OM_SKYMAIL_ACCOUNT_ID` 设成 admin 账号 1。

- 不要使用 `https://skymail.ink/api/login`：该自定义域对 POST 返回空 200；生产用 `cm.autozteam.cn`。
- 修改环境后 `systemctl restart ourmemory`。

## 4.1 自建隧道/反代（用户选定方案）

应用只监听 `0.0.0.0:5259`，本身不处理 TLS。把公网入口（hp-lite、frp、Cloudflare Tunnel 或云服务器 Nginx）反代到：

```text
http://192.168.1.11:5259
```

反代必须保留 Host 并设置 `X-Forwarded-Proto`，否则登录 Cookie 的 Secure 标记和子域判断会不正确：

```nginx
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
client_max_body_size 8g;
proxy_request_buffering off;
proxy_read_timeout 3600s;
```

入口必须同时覆盖 `om.autozteam.cn` 和 `*.om.autozteam.cn`；应用会按 Host 的第一个标签重写到 `/video`、`/image`、`/weibo`、`/im`。

## 5. 上线检查清单

```bash
systemctl is-active ourmemory
ss -lntp | grep 5259
curl -s http://127.0.0.1:5259/api/om/me
curl -I https://om.autozteam.cn/
curl -I https://video.om.autozteam.cn/
```

- [ ] `om` 与 `*.om` DNS 均解析到 81.70.77.4
- [ ] Cloudflare Total TLS 已签发二级子域证书
- [ ] Tunnel 两个 Public Hostname 均为 `http://127.0.0.1:5259`
- [ ] `OM_BASE_HOST` / `OM_BASE_URL` 正确
- [ ] 管理员密码已从 `/opt/ourmemory/ADMIN_PASSWORD.txt` 读取并另存
- [ ] 生产测试数据已清理
- [ ] 邮箱验证码可真实发送（或明确暂用日志）
- [ ] 备份任务已启用（`scripts/backup.sh`）
