# Feishu 配置收敛重构方案

## 背景

当前 Feishu 配置虽然已经从主 `config.toml` 拆分到了独立的 `channels/feishu.toml`，但配置模型仍然泄漏到了多个层次：

- `internal/config` 负责 Feishu 配置文件的路径、加载、保存、回填。
- `cli/serve` 负责把 `cfg.Channels.Feishu*` 转成 `feishu.AppConfig`，并负责 reload 后把整份 `ChannelsConfig` 再推给 runtime。
- `runtimewiring` / `sandboxgateway` 持有的是整份 `config.ChannelsConfig`，但真正只需要按 `botID` 查询 Feishu 凭证。
- `onboard` / `detect` 也把 `cfg.Channels` 传给 runtime wiring，但其配置加载实际并不读取独立的 `channels/feishu.toml`，存在误导性依赖。

这导致 Feishu 配置的职责边界不清晰：

- 配置存储职责在 `internal/config`
- 业务配置视图在 `internal/channel/feishu`
- runtime 注入又依赖全局 config 数据结构

目标是把 Feishu 配置能力收敛到 `internal/channel/feishu`，让其他模块只依赖一个稳定、最小的 provider 接口。

## 目标

- Feishu 配置的读取、保存、reload、运行时快照统一由 `internal/channel/feishu` 管理。
- `internal/config.Config` 不再承载 Feishu 专属字段。
- `cli/serve` 不再解释 Feishu 配置结构，只负责装配 provider 和 service。
- runtime 不再依赖整份 `config.ChannelsConfig`，只依赖按 `botID` 查询 Feishu 凭证的能力。
- reload 通知传递 Feishu 运行时快照，而不是整份全局 channels config。
- 保持现有 `channels/feishu.toml` 文件格式不变，避免用户迁移成本。

## 非目标

- 不修改 Feishu Channel 现有业务 API 和消息处理逻辑。
- 不修改 BoxLite / PicoClaw / OpenClaw 的运行时集成方式本身，只调整 Feishu 配置注入方式。
- 不改动 `channels/feishu.toml` 的存储格式，除非后续有单独需求。

## 设计原则

1. 单一职责：Feishu 配置存储和 Feishu 配置运行时视图都归 `internal/channel/feishu`。
2. 最小依赖：上层只依赖 provider，不依赖 `config.ChannelsConfig`。
3. 单一事实源：运行中的 Feishu app 配置、reload 结果、runtime 注入都来自同一个 provider。
4. 渐进重构：先引入 provider 并切换调用方，再移除 `internal/config` 中的 Feishu 专属逻辑。

## 目标结构

建议在 `internal/channel/feishu` 内拆成两层：

### 1. Store 层

负责持久化读写 `channels/feishu.toml`。

建议新增：

```go
type Snapshot struct {
    AdminOpenID string
    Bots        map[string]AppConfig
}

type Store interface {
    Load() (Snapshot, error)
    LoadIfExists() (Snapshot, bool, error)
    Save(Snapshot) error
}
```

实现建议：

- `fileStore`：基于 `configPath -> channels/feishu.toml` 做路径解析
- 文件格式与现有 `channels/feishu.toml` 保持一致

注意：

- `Store` 是 Feishu 包内部抽象，主要服务于 provider 和测试替身。
- `Store` 不向外暴露 `config.ChannelsConfig`，只暴露 Feishu 自己的 `Snapshot`。

### 2. Provider 层

负责向 service、runtime、API 暴露统一的运行时配置能力。

建议新增：

```go
type Provider interface {
    Snapshot() Snapshot
    BotConfig(botID string) (AppConfig, bool)
    Reload() (Snapshot, error)
    Update(Update) (Entry, Snapshot, error)
    SetReloadHook(func(Snapshot))
}
```

职责：

- 缓存当前生效的 Feishu 配置快照
- 从 Store reload
- 更新配置并持久化
- 向运行时和 service 提供按 `botID` 查询能力
- 在 reload 后通知订阅方

建议实现：

- `ConfigProvider struct { store Store; snapshot Snapshot; ... }`

## service 和 runtime 的依赖方式

### Feishu Service

当前问题：

- `feishu.Service` 既持有 `apps map[string]AppConfig`，又持有一个 `configStore`
- 配置与运行时 app 数据有两套来源

调整后：

- `feishu.Service` 直接依赖 `Provider`
- `ReloadConfig()` 调用 provider 的 `Reload()`
- `GetConfig()` / `UpdateConfig()` 通过 provider 完成
- service 内部如仍需保留 `apps map[string]AppConfig` 作为热路径缓存，则该缓存也应只从 provider 的 snapshot 同步

建议接口：

```go
func NewService(provider Provider) *Service
```

### Runtime Wiring

当前问题：

- `WithPicoClawSandboxRuntime(channels config.ChannelsConfig)` 传入的是整份 channels config
- `sandboxgateway.Runtime` 也保存和 clone 整份 `ChannelsConfig`

调整后：

- `WithPicoClawSandboxRuntime(feishuProvider feishu.Provider)` 或更窄一点的接口
- runtime 构建环境变量时只调用 `BotConfig(botID)`

更推荐的最小接口：

```go
type BotCredentialProvider interface {
    BotConfig(botID string) (feishu.AppConfig, bool)
}
```

如果后续 runtime 只需要这个能力，可以让 `feishu.Provider` 满足该接口，而不是为 runtime 再定义一套独立 provider。

### Reload 通知

当前问题：

- reload hook 传的是 `config.ChannelsConfig`
- runtime 更新逻辑耦合全局 config 包结构

调整后：

- reload hook 改为传 `feishu.Snapshot`
- runtime update 只关心 snapshot 中的 bot 配置

## 各模块改动方案

### `internal/channel/feishu`

新增或调整：

- 新增 `store.go`
- 新增 `provider.go`
- 把 `config.go` 中与路径解析、文件读写、独立加载相关的逻辑迁到 `store.go`
- 保留 `Entry` / `Update` / `MaskConfig` 这类与 API 视图直接相关的类型和方法
- `Service` 改为依赖 provider，不再自行持有“文件存储 + apps map”两套事实源

### `internal/config`

目标：

- 去除 Feishu 专属配置模型和专属读写逻辑

调整建议：

- 删除 `ChannelsConfig` 中的 `FeishuAdminOpenID`、`Feishu map[string]FeishuConfig`
- 删除 `FeishuConfig` 类型
- 删除 `internal/config/feishu_config.go`
- `Config` 回归通用配置，不再负责 channel 私有配置拼装

注意：

- 这一步不要最先做。
- 应在 provider 已接管所有调用方后，再删除 config 层 Feishu 字段和逻辑。

### `cli/serve`

目标：

- 只负责装配，不负责解释 Feishu 配置结构

调整建议：

- 当前 `NewFeishuService(cfg)` 改为 `NewFeishuService(configPath)` 或 `NewFeishuService(provider)`
- 删除 `feishuAppsFromConfig`
- `configureFeishuService` 改为直接给 provider 或 service 注册 reload hook
- reload hook 不再传 `config.ChannelsConfig`，改传 `feishu.Snapshot`

推荐装配方式：

1. `store := feishu.NewFileStore(configPath)`
2. `provider := feishu.NewProvider(store)`
3. `feishuSvc := feishu.NewService(provider)`
4. runtime wiring 注入同一个 `provider`
5. reload hook 使用同一个 `provider`

这样 `serve` 是唯一 composition root，但不再承担配置转换逻辑。

### `internal/app/runtimewiring`

目标：

- 从“依赖全局 channels config”改为“依赖 Feishu bot credential provider”

调整建议：

- `WithPicoClawSandboxRuntime(channels config.ChannelsConfig)` 改为 `WithPicoClawSandboxRuntime(provider BotCredentialProvider)`
- `UpdatePicoClawChannels(...)` 改为 `BindPicoClawFeishuProvider(...)` 或更自然的更新方式

更进一步的简化方案：

- runtime 不必再维护一份 channels 快照
- runtime 在构建 env 时直接调用 provider `BotConfig(botID)`

如果担心并发读配置性能：

- provider 内部维护 snapshot cache
- runtime 只读 provider cache，不直接读文件

### `internal/runtime/sandboxgateway`

目标：

- 删除 `Dependencies.Channels config.ChannelsConfig`

调整建议：

- 用 `FeishuProvider` 或 `BotCredentialProvider` 替代
- 删除 `cloneChannelsConfig`
- `SetChannels(...)` 改成 `SetFeishuProvider(...)`，或者直接不需要这个方法

推荐方向：

- 不让 runtime 持有可变的完整 config 数据
- 只持有一个 provider 引用
- reload 后 provider 的内部 snapshot 更新，runtime 无需显式 copy 全量配置

这样可以进一步减少 reload 时的胶水代码。

### `internal/onboard`

当前问题：

- `onboard` / `detect` 给 `WithPicoClawSandboxRuntime` 传了 `cfg.Channels`
- 但自身 `loadConfig` 实际只调用 `config.Load(path)`，不会加载 `channels/feishu.toml`

调整建议：

- 若 manager bootstrap 根本不依赖 Feishu，则直接传空 provider / nil provider
- 若 wiring 必须有 provider，则传一个明确的 no-op provider

目标：

- 消除“看起来依赖 Feishu，实际并未生效”的误导

### `cli/bot config` 与 `internal/api/feishu_config.go`

这两处依赖是合理的，应保留，但依赖目标应改为 provider / service，而不是全局 config 结构。

调整建议：

- CLI 不变更协议
- API handler 不变更路由
- service 内部改由 provider 实现 `GetConfig` / `UpdateConfig` / `ReloadConfig`

## 推荐实施步骤

建议分 5 步推进，避免一次性大改。

### 第一步：引入 Feishu Store 和 Provider

改动：

- 在 `internal/channel/feishu` 新增 `store.go`、`provider.go`
- 复制并迁移 `internal/config/feishu_config.go` 的文件读写逻辑
- 引入 `Snapshot`、`Store`、`Provider`

完成标准：

- 不改现有调用方的前提下，新 provider 可以独立完成 load/save/reload/update

### 第二步：让 Feishu Service 切到 Provider

改动：

- `feishu.Service` 改为依赖 provider
- `GetConfig` / `UpdateConfig` / `ReloadConfig` 改走 provider
- `SetConfigPath`、`SetConfigReloadHook` 这类以文件路径和 config store 为中心的方法，改成 provider 风格接口，必要时保留兼容层一段时间

完成标准：

- `internal/api/feishu_config.go` 和 Feishu 业务逻辑只通过 service/provider 获取配置

### 第三步：runtime 切到最小 provider 接口

改动：

- 修改 `internal/app/runtimewiring`
- 修改 `internal/runtime/sandboxgateway`
- 删除对 `config.ChannelsConfig` 的运行时依赖

完成标准：

- runtime 注入 Feishu 环境变量时只通过 `BotConfig(botID)` 获取凭证
- reload 不再传整份 channels config

### 第四步：`cli/serve` 改成纯装配

改动：

- `serve` 中创建 `store -> provider -> service`
- runtime 注入同一个 provider
- 删除 `feishuAppsFromConfig`
- 删除 `NewFeishuService(cfg config.Config)` 这种从全局 config 拆 Feishu 配置的代码

完成标准：

- `cli/serve` 不再理解 Feishu 配置的字段结构

### 第五步：删除 `internal/config` 中的 Feishu 专属逻辑

改动：

- 删除 `internal/config/feishu_config.go`
- 删除 `Config.Channels` 中的 Feishu 字段
- 清理受影响测试
- 清理 `onboard` 中的误导性依赖

完成标准：

- `internal/config` 只包含通用配置
- Feishu 配置不再经过全局 config 模型

## 兼容性与风险

### 兼容性

- `channels/feishu.toml` 文件格式保持不变
- Feishu config API 路由保持不变
- `csgclaw bot config --channel feishu` 用法保持不变

### 风险点

1. reload 行为变化
- 需要确保 provider reload 后，service 使用的新配置与 runtime 注入的新配置一致。

2. 并发访问
- provider 内部需要明确读写锁边界。
- `Update` 和 `Reload` 不能产生脏读或覆盖。

3. 迁移过程中的双事实源
- 在第二、三步期间最容易出现“service 一套 apps、provider 一套 snapshot”。
- 实施时要尽快把 service 内部配置读取统一到 provider，避免长期共存。

4. 测试回归
- 配置读写测试
- reload 测试
- runtime env 注入测试
- serve wiring 测试
- API config 测试

## 测试建议

建议至少覆盖以下测试：

- `internal/channel/feishu/store_test.go`
  - `Load`
  - `LoadIfExists`
  - `Save`
  - 非法 bot id
  - 空文件 / 缺失文件

- `internal/channel/feishu/provider_test.go`
  - 初始加载
  - `BotConfig`
  - `Reload`
  - `Update`
  - reload hook 触发

- `internal/app/runtimewiring`
  - provider 存在 bot config 时正确注入 `PICOCLAW_CHANNELS_FEISHU_APP_ID`
  - provider 无 bot config 时不注入

- `cli/serve/serve_test.go`
  - 使用同一个 provider 装配 service 和 runtime
  - reload 后 runtime 可见最新配置

- `internal/api/feishu_config_test.go`
  - GET/PUT/POST 行为不变

## 最终效果

重构完成后，Feishu 配置依赖关系应变成：

- `internal/channel/feishu`
  - 负责配置文件存储
  - 负责配置快照与 reload
  - 负责对外提供 provider

- `cli/serve`
  - 只负责装配 provider、service、runtime

- `internal/api`
  - 只依赖 `feishu.Service`

- `runtime`
  - 只依赖 Feishu credential provider

- `internal/config`
  - 不再承载 Feishu 专属配置

这样可以把目前分散在 `config`、`serve`、`runtime`、`channel/feishu` 的 Feishu 配置职责，收敛为一个清晰的中心模型，后续无论新增其他 channel，还是继续演进 Feishu 配置能力，都更容易复用同一套边界。
