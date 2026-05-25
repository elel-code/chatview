# ChatView 客户端配置

当前客户端实现为 Go/Fyne。旧 C++/Saucer 客户端和 Web frontend 已移除，运行、配置和本地数据路径均以 Go/Fyne 客户端为准。

## 运行方式

```sh
go run ./cmd/client
go run ./cmd/client --config client.example.yaml
```

Linux Wayland 会话下建议使用原生 Wayland tag：

```sh
go run -tags wayland ./cmd/client --config client.example.yaml
```

## Proto 生成

```sh
cd ..
./scripts/gen-proto
```

Proto 定义统一放在仓库根目录 `proto/`，Go 生成代码统一输出到 `api/gen`，客户端通过 `chatview/api` module 引用。生成脚本优先使用 `PROTOC=/path/to/protoc`，否则自动发现唯一的 `tools/protoc-*/bin/protoc`，最后回退到 PATH 中的 `protoc`。如果本地保留多个 `tools/protoc-*` 版本，需要显式设置 `PROTOC`。

## 命令行参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--config` | 空 | 可选 YAML 配置路径。为空时仅使用内置默认值。 |
| `--data-dir` | 配置或内置默认值 | 覆盖客户端数据目录。 |
| `--target` | 配置或 `127.0.0.1:50051` | 覆盖 gRPC server 地址。 |
| `--tls` | 根据 target 自动判断 | 覆盖是否启用 gRPC TLS，支持 `true`/`false`。 |

## YAML 配置项

| 键 | 默认值 | 说明 |
|---|---|---|
| `data_dir` | Linux: `~/.chatview`; Windows: `%APPDATA%/chatview`; macOS: `~/Library/Application Support/chatview` | 存放 Go/Fyne 客户端本地身份和缓存。无法获取平台家目录时回退到 `./.chatview`。 |
| `grpc_target` | `127.0.0.1:50051` | Go gRPC server 地址。 |
| `grpc_tls` | 环回地址默认 `false`; 其它地址默认 `true` | 是否启用 gRPC TLS。 |
| `grpc_ca_path` | 空 | 可选自定义 CA PEM 证书路径。空值使用 gRPC 平台默认根证书。 |
| `grpc_ssl_target_name_override` | 空 | 可选 TLS 主机名覆盖，用于开发或测试证书。 |

示例：

```yaml
data_dir: "/tmp/chatview-client"
grpc_target: "chatview.example.com:443"
grpc_tls: true
grpc_ca_path: ""
grpc_ssl_target_name_override: ""
```

## 本地数据

Go/Fyne 客户端当前存储以下文件：

| 文件 | 路径 |
|---|---|
| 身份文件 | `${data_dir}/identity-go.bin` |
| SQLite 缓存 | `${data_dir}/cache-go.db` |

Go/Fyne 客户端不会读取旧 `identity.bin` 和 `cache.db`。已有 Ed25519 私钥可以通过导入身份功能写入当前的 `identity-go.bin`。

## 迁移状态

- Go/Fyne 入口：`cmd/client`
- 配置加载：`internal/config`
- 身份/PIN/导入导出：`internal/identity`
- gRPC 客户端：`internal/rpcclient`
- 本地缓存和 outbox：`internal/storage`
- 应用编排：`internal/core`
- 桌面 UI：`internal/ui`

本地迁移验收目前覆盖身份创建/导入/导出、错误 PIN、损坏身份文件、配置解析、SQLite 缓存/outbox 生命周期、DTO JSON 字段兼容性，以及 fake gRPC server 下的登录签名、Bearer metadata、Chat/Admin/Event RPC 映射。真实 server 端到端联调仍需连接实际服务后验证。
