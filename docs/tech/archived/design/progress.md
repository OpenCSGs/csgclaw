# 进展概要

## IM

- [x] 完成 IM 页面整体方案设计，技术栈明确为 Go 后端 + React 前端。
- [x] 收敛核心能力范围，包括基于 room 的群聊、DM 单聊、@ 用户、创建 Room、邀请成员，以及中英文切换。
- [x] 明确界面方向，整体以简洁清爽为目标，并参考 Element / Telegram 的交互风格。
- [x] 收敛页面视觉和交互细节，包括品牌区、语言切换、New Room / New Chat 入口、统计区布局等关键区域。
- [x] 统一数据模型为“会话模型”，默认在 UI 上弱化 Room / DM 区分，仅保留轻量属性处理权限和文案，降低界面和实现复杂度。

## CLI / Server

- [x] 明确首期目标是建设一个标准 Go 项目，同时覆盖 Server 与 CLI 两部分能力。
- [x] 明确 Server 侧核心职责：通过 REST API 创建 Agent，并支持 `name`、`image` 等必要参数。
- [x] 定义 CLI 主流程，包括 `onboard`、`serve`、`create` 三个子命令，分别对应本地配置初始化、服务启动和 Agent 创建。
- [x] 初步确定配置方案，`~/.csgclaw/config.toml` 用于承载模型接入所需的 `base_url`、`api_key` 和 `model_id` 等基础参数。
- [x] 完成 BoxLite Go SDK 与 Python SDK 的技术调研，并确认 vendored SDK、`go generate`、运行时创建 box、端口映射、数据卷挂载等关键集成点。
- [x] 沉淀容器化运行 OpenClaw / Agent Gateway 的配置与启动样例，可作为后续 Server daemon 化和本地开发联调的实现参考。
- [x] 为 IM 增加本地 bootstrap 状态文件，并让 `csgclaw onboard` 负责初始化/修正该状态，保证重复执行时具备幂等性。
- [x] 调整 `csgclaw onboard`：除确保 manager box 外，同时确保 IM 中存在 `Admin` 与 `Manager` 两个成员；若已存在则跳过创建，不清空既有用户和会话数据。
- [x] 取消 IM 硬编码种子数据；首次 onboard 从空白 IM 状态起步，仅补齐必要成员，并在每次 onboard 时清空当前邀请成员草稿数据。
- [x] 在 onboard 阶段确保 `Admin` 和 `Manager` 至少同处于一个 Room 中；若已有满足条件的 Room 则复用，否则补建一个 bootstrap Room。
