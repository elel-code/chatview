# ChatView 客户端配置

C++ 桌面客户端使用内置默认值，外加一个可选的 YAML 文件（通过 `--config` 传入）。
客户端**不会**读取 `CHATVIEW_*` 环境变量。

## 命令行参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--config` | 空 | 可选的 YAML 配置文件路径。如果为空，则仅使用内置默认值。支持 `--config path/to/client.yaml` 和 `--config=path/to/client.yaml` 两种写法。 |

示例：

```sh
./build/release/src/chatview/chatview_client --config client.yaml
```

## YAML 配置项

| 键 | 默认值 | 说明 |
|---|---|---|
| `data_dir` | Linux: `~/.chatview`；Windows: `%APPDATA%/chatview`；macOS: `~/Library/Application Support/chatview` | 存放 `identity.bin` 和 `cache.db` 的目录。如果无法获取平台家目录，则回退到 `./.chatview`。 |
| `grpc_target` | `127.0.0.1:50051` | Go gRPC 服务器目标地址，传给 `grpc::CreateChannel`。 |
| `grpc_tls` | 环回地址默认 `false`；其它地址默认 `true` | 是否启用 gRPC TLS。 |
| `grpc_ca_path` | 空 | 可选的自定义 CA PEM 证书路径。空值则使用 gRPC 平台的默认根证书。 |
| `grpc_ssl_target_name_override` | 空 | 可选的 TLS 主机名覆盖，用于开发或测试证书。 |

示例：

```yaml
data_dir: "/tmp/chatview-client"
grpc_target: "chatview.example.com:443"
grpc_tls: true
grpc_ca_path: ""
grpc_ssl_target_name_override: ""
```

## CMake 选项

| 选项 | 默认值 | 说明 |
|---|---|---|
| `CHATVIEW_BUILD_TESTS` | `ON` | 构建 `chatview_client_local_tests` 测试目标。 |
| `CHATVIEW_FRONTEND_DIST_DIR` | `${CMAKE_SOURCE_DIR}/../frontend/dist` | 生产前端 dist 目录，通过 `saucer_embed()` 嵌入到二进制文件中。该目录必须包含 `index.html`。 |

示例：

```sh
cmake --preset release -DCHATVIEW_BUILD_TESTS=OFF
cmake --preset release -DCHATVIEW_FRONTEND_DIST_DIR=/absolute/path/to/frontend/dist
```

## 固定运行时路径

客户端当前存储以下文件：

| 文件 | 路径 |
|---|---|
| 身份文件 | `${data_dir}/identity.bin` |
| SQLite 缓存 | `${data_dir}/cache.db` |

生产前端在构建时嵌入，通过 `saucer::embedded::all()` 加 `webview->serve("/index.html")` 加载；生产客户端中没有开发服务器 URL 回退机制。
