# ChatView Go 服务端配置

Go 服务端按以下顺序读取配置：

1. 内置默认值。
2. 通过 `-config` 传入的可选 YAML 文件。

服务端**不**读取 `CHATVIEW_*` 环境变量。

## 命令行参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-config` | 空 | 可选的 YAML 配置文件路径。如果为空，仅使用内置默认值。 |

示例：

```sh
go run ./cmd/server -config config.example.yaml
```

## YAML 配置项

| 键 | 默认值 | 必填 | 说明 |
|---|---:|---:|---|
| `listen_addr` | `:50051` | 否 | 传递给 `net.Listen` 的 TCP 地址。 |
| `db_dsn` | 空 | 是 | PostgreSQL DSN。如果为空，服务端启动失败。 |
| `tls_cert` | 空 | 否 | TLS 证书文件路径。当 `tls_cert` 或 `tls_key` 任一非空时启用 TLS；两者必须同时加载成功。 |
| `tls_key` | 空 | 否 | TLS 私钥文件路径。 |
| `session_ttl` | `24h` | 否 | 认证会话存活时间。 |
| `challenge_ttl` | `5m` | 否 | 登录挑战存活时间。 |
| `cleanup_interval` | `10m` | 否 | 过期认证/会话清理间隔。 |
| `presence_heal_interval` | `5m` | 否 | 在线状态 reconciliation 及好友/管理员更新推送间隔。 |
| `admin_pub_key` | 空 | 否 | 启动时作为管理员用户植入的公钥。空值表示不植入初始管理员。 |
| `migrations_dir` | `migrations` | 否 | SQL 迁移文件目录。与 `skip_migrations` 配合使用。 |
| `skip_migrations` | `false` | 否 | 为 `true` 时跳过数据库迁移。仅当数据库表结构已由其他方式（如手动执行迁移）准备就绪时使用。 |

时长字段接受 Go 时长字符串，如 `5m`、`24h`、`30s`。数值会被解析为原始 `time.Duration` 纳秒值，因此推荐使用时长字符串。

示例：

```yaml
listen_addr: ":50051"
db_dsn: "postgres://chatview:chatview@localhost:5432/chatview?sslmode=disable"
tls_cert: ""
tls_key: ""
session_ttl: 24h
challenge_ttl: 5m
cleanup_interval: 10m
presence_heal_interval: 5m
admin_pub_key: ""
migrations_dir: "migrations"
skip_migrations: false
```

## 启动流程

启动时服务端依次执行：

1. 加载配置。
2. 使用 `db_dsn` 打开 PostgreSQL。
3. 若 `skip_migrations` 为 `false`，从 `migrations_dir` 目录加载并应用 SQL 迁移。
4. 当 `admin_pub_key` 非空时植入初始管理员。
5. 清理一次过期在线会话。
6. 启动后台清理和在线状态修复循环。
7. 仅当 `tls_cert` 或 `tls_key` 已配置时启用 TLS。
8. 注册认证、聊天、事件流、管理、健康检查和反射服务。

关闭时先尝试 `GracefulStop()`，10 秒后回退为 `Stop()`。

## Proto 生成

```sh
cd ..
./scripts/gen-proto
```

Proto 定义统一放在仓库根目录 `proto/`，Go 生成代码统一输出到 `api/gen`，服务端通过 `chatview/api` module 引用。生成脚本优先使用 `PROTOC=/path/to/protoc`，否则自动发现唯一的 `tools/protoc-*/bin/protoc`，最后回退到 PATH 中的 `protoc`。如果本地保留多个 `tools/protoc-*` 版本，需要显式设置 `PROTOC`。
