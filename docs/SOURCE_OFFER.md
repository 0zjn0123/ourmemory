# 源码提供与 AGPL 合规

ourmemory 是基于 [OpenList](https://github.com/OpenListTeam/OpenList) 的修改版，并以网络服务形式对外提供。OpenList 使用 **AGPL-3.0**；按该许可证，通过网络使用本服务的用户有权获得对应的完整源码。

## 源码位置

- 上游：https://github.com/OpenListTeam/OpenList
- 本项目修改版：上线前发布到公开仓库（待定，建议 `autozteam/ourmemory`），并在页脚与登录页保留 Source 链接。
- 若仓库暂时不可公开，必须在站点提供可下载的源码包（含 `go.mod`、`go.sum`、全部修改、构建脚本），并保留至少 3 年。

## 修改说明

主要修改：

- 新增 `internal/om`：共享/私人归档存储、认证、文件 API、视频/图片/微博/IM、开放 API、贡献节点、管理后台。
- 新增 `cmd/omnode`：贡献节点 agent。
- 修改 `internal/bootstrap/run.go`：初始化 OM 业务层。
- 修改 `server/router.go`：注册 OM 路由与子域重写。
- 新增 `public/om`：前端。
- 新增 `deploy/`、`scripts/`、`docs/`。

我们没有删除上游版权与许可证声明。`LICENSE` 保持 AGPL-3.0。

## 构建

```bash
# 主程序
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o ourmemory .

# 节点 agent
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o omnode ./cmd/omnode
```

依赖版本见 `go.mod` / `go.sum`。构建不需要私有依赖。

## 用户如何获取

- 站点页脚「Source」链接；
- 登录页与 README 中的源码地址；
- 如仓库不可用，管理员在 `/opt/ourmemory/SOURCE_OFFER.txt` 放置源码包下载地址与离线副本路径。

## 注意

- 不要删除或替换上游 `LICENSE`、版权头。
- 分发二进制时一并提供源码或书面 offer。
- 若将本项目再分发，修改部分同样按 AGPL-3.0 授权。
