# ops-guarding 项目分析

## 项目定位

`ops-guarding` 是一个面向批量机器脚本执行与状态回传的 Go 项目。命令入口提供 `server` 和 `agentd` 两种运行模式：`server` 负责任务创建、调度、状态查询和结果存储，`agentd` 部署在目标机器侧，通过 RPC 心跳领取任务、执行脚本并回传结果。

当前 README 仍是草稿形态，但代码已经呈现出清晰的“中心调度服务 + 机器代理”架构。项目还包含指标采集插件框架雏形，不过输出实现为空，说明 metrics 能力尚未闭环。

## 目录结构

- `cmd/`：CLI 入口，基于 `urfave/cli/v2` 注册 `server` 和 `agentd` 子命令。
- `pkg/server/`：服务端配置、HTTP API、RPC 服务、调度逻辑和定时任务。
- `pkg/agentd/`：代理端配置、RPC 客户端、任务执行、心跳上报和指标采集框架。
- `pkg/models/`：GORM 数据模型和任务状态操作。
- `pkg/appframe/`：通用框架代码，包含 HTTP、日志、配置、缓存、存储、系统命令和工具函数。
- `etc/service/`：示例配置文件。
- `sql/`：MySQL 初始化脚本，核心文件为 `sql/ibex.sql`。
- `meta/`：本地任务元数据和执行结果样例。

## 运行与配置

项目使用 Go 1.22.1。推荐先构建单一二进制，再通过子命令运行：

```bash
go build -o bin/guarding ./cmd
./bin/guarding server -c etc/service/server.conf
./bin/guarding agentd -c etc/service/agentd.conf
```

代码默认读取 `etc/server.conf` 和 `etc/agentd.conf`，但仓库中的示例配置位于 `etc/service/`。因此本地运行时应显式传入 `-c`，或补齐默认路径配置。

服务端配置包含 HTTP、BasicAuth、RPC、心跳、日志和数据库连接。`server.conf` 默认 HTTP 端口为 `10090`，RPC 监听为 `0.0.0.0:20090`。代理端默认 HTTP 端口为 `2090`，并通过 `Heartbeat.Servers` 连接服务端 RPC。

## 核心执行链路

1. 调用 `POST /ibex/v1/tasks` 创建任务，请求体包含标题、账号、脚本、参数、stdin、动作、创建人和 hosts。
2. 服务端将任务写入 `task_meta`、`task_action`、`task_scheduler`，并把目标机器写入按任务 ID 分片的 `task_host_*` 表。
3. 服务端定时器接管未分配任务，并按 `batch`、`tolerance`、`pause` 和超时策略推进调度。
4. agentd 周期性调用 `Server.Report`，先上报本地已完成任务，再领取新的 `AssignTask`。
5. agentd 拉取任务元信息，将脚本、参数和 stdin 缓存到 `meta/<task-id>/`，执行脚本后写入 stdout、stderr 和 `<clock>.done`。
6. 服务端根据 agent 上报更新 host 状态，任务全部完成后清理 `task_scheduler` 与 `task_action`。

## API 与通信

服务端 HTTP 路由包含健康检查类接口 `/ping`、`/pid`、`/addr`、`/version`，以及 BasicAuth 保护的 `/ibex/v1` 任务管理接口。任务接口支持创建、查询、动作更新、单 host 操作，以及 stdout/stderr 的文本和 JSON 读取。

服务端 RPC 使用 Go `net/rpc` 加 msgpack 编码，主要方法包括 `Ping`、`GetTaskMeta` 和 `Report`。agentd 会探测多个 RPC server，选择响应最快的节点并复用连接。

## 数据模型

`sql/ibex.sql` 定义了任务元信息、任务动作、调度器归属、调度器健康、执行中 host，以及 `task_host_0` 到 `task_host_99` 的执行状态分片表。模型层通过 `tht(id)` 选择任务 host 分片表，减少单表膨胀风险。

主要状态包括 `waiting`、`running`、`success`、`failed`、`timeout`、`killing`、`killed`、`killfailed` 和 `ignored`。服务端以 `task_action` 是否存在判断任务是否仍在进行中。

## 风险与改进建议

- 配置默认路径与仓库示例路径不一致，容易导致首次运行失败。
- `server` 和 `agentd` 自动探测外网 IP 时会访问 `8.8.8.8:80`，离线环境或受限网络中可能直接退出。
- `etc/service/server.conf` 包含明文数据库密码和 BasicAuth 示例口令，生产部署应改用环境变量或密钥管理。
- 仓库只发现 `pkg/appframe/logger/logger_test.go` 一个测试文件，调度、RPC、任务状态机和 agent 执行链路缺少回归测试。
- metrics 输入框架已经存在，但 `outputs.WriteSamples` 为空，当前不能确认指标数据会真正输出。
- `taskState` 注释说明当前只看 `task_action`，没有综合所有 host 执行状态，任务状态语义仍有改进空间。

## 建议优先级

短期应补齐可运行文档、统一默认配置路径，并增加 server/agentd 的最小集成测试。中期可以为任务状态机、超时处理、kill/redo/ignore 操作建立表驱动测试。长期应明确 metrics 输出目标、配置热加载策略、认证方案和多 server 高可用边界。

## 证据来源

本分析基于 `cmd/main.go`、`pkg/server/*`、`pkg/agentd/*`、`pkg/models/*`、`pkg/appframe/*`、`etc/service/*.conf`、`sql/ibex.sql` 和现有 README 草稿。
