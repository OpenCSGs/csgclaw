# boxlite-cli 随 csgclaw bundle 分发的可行性分析与改造步骤

## 背景

当前 `boxlite-cli` sandbox provider 依赖用户自己安装 `boxlite` 命令，并通过：

```toml
[sandbox]
provider = "boxlite-cli"
boxlite_cli_path = "boxlite"
```

由 `internal/sandbox/boxlitecli` 直接执行该路径。

这意味着：

- release 产物目前默认是“单二进制”心智模型。
- `boxlite-cli` provider 只能从 `PATH` 或用户显式配置的绝对路径找到 `boxlite`。
- 用户若没有预装 `boxlite-cli`，即使 `csgclaw` 已安装，也无法直接使用 `boxlite-cli` provider。

目标是把 `boxlite-cli` 一起打进 `csgclaw` 的 release bundle，让最终用户：

- 不需要手动安装 `boxlite-cli`
- 不需要配置 `PATH`
- 不需要理解 `boxlite-cli` provider 的实现细节

也就是说，`boxlite-cli` 对用户应当是一个完全内部化的 runtime 依赖，而不是外部前置条件。

## 结论

这件事是可行的，而且比把 `boxlite-cli` 再做一层 embed 到 Go 二进制里更稳妥。推荐方案是：

- 保持 `csgclaw` 主程序仍为普通 Go 二进制。
- release 打包时，把对应平台的 `boxlite-cli` 预编译包下载并解压到 release bundle 中。
- `csgclaw` 启动 `boxlite-cli` provider 时，默认优先使用 bundle 内的 `boxlite`。
- 只有开发/调试场景下，才允许用户通过 `boxlite_cli_path` 显式覆盖 bundled binary。

这样做的优点：

- 不需要把第三方 CLI 再次重打包进 Go embed，避免增加运行时解压逻辑。
- 默认用户体验最简单，安装完 `csgclaw` 即可使用，不再依赖系统环境。
- 仍保留少量高级覆写能力，便于调试、热修复、切换版本。
- release 产物结构清晰，安装脚本也容易适配。

## 当前代码现状

从仓库现状看，主要约束如下：

- `internal/sandbox/boxlitecli/provider.go`
  - `NewProvider()` 默认路径是 `"boxlite"`。
  - provider 只认 `WithPath()` 注入的路径，不会自动探测 bundle 相对目录。
- `internal/sandboxproviders/boxlite_cli_provider.go`
  - 当前直接把 `cfg.BoxLiteCLIPath` 传给 `boxlitecli.WithPath(...)`。
- `internal/config/config.go`
  - `DefaultBoxLiteCLIPath = "boxlite"`。
  - 当前默认语义仍偏向“从 PATH 找到 boxlite”，这和“用户无感知”目标不一致。
- `scripts/package-release.sh`
  - 当前产物是单文件归档，只把 `csgclaw` 或 `csgclaw-cli` 放进 tar.gz/zip。
- `.github/workflows/release.yml`
  - 当前只发布 `csgclaw`、`csgclaw-cli` 两类产物。
  - release matrix 现在只有 `darwin/arm64`、`linux/amd64`。
- `scripts/install.sh`
  - 当前假设 release archive 解压后只包含一个可执行文件，并将其安装到 `~/.local/bin/<app>`。

所以，这不是只改 sandbox provider 就能完成的事情，必须同时改：

- 运行时路径解析
- 打包脚本
- release workflow
- 安装脚本
- 文档与测试

## 推荐的 bundle 形态

建议把 release archive 从“单文件”改为“目录归档”。

以 `csgclaw` 为例，建议归档内容如下：

```text
csgclaw/
  bin/
    csgclaw
    boxlite
```

这样运行时可以稳定地按 `../bin/boxlite` 或同目录规则定位。

更具体地说，推荐使用这一套布局：

```text
csgclaw_<version>_<goos>_<goarch>.tar.gz
└── csgclaw/
    └── bin/
        ├── csgclaw
        └── boxlite
```

理由：

- 安装脚本容易处理。
- macOS/Linux 都适合。
- 后续如果还要附带别的 helper 二进制、license、NOTICE、checksums，也有扩展空间。

不建议继续保留“archive 根目录直接只有一个 `csgclaw` 文件”的形式，否则无法优雅表达 bundled helper。

## boxlite-cli 产物命名映射

用户给出的 BoxLite release URL 是：

- `boxlite-cli-v0.8.2-aarch64-apple-darwin.tar.gz`
- `boxlite-cli-v0.8.2-aarch64-unknown-linux-gnu.tar.gz`
- `boxlite-cli-v0.8.2-x86_64-unknown-linux-gnu.tar.gz`

而 `csgclaw` 当前 release 维度使用的是 Go 风格：

- `darwin/arm64`
- `linux/amd64`
- 未来可能恢复 `linux/arm64`

因此需要建立一层映射：

| csgclaw GOOS/GOARCH | boxlite release suffix |
| --- | --- |
| `darwin/arm64` | `aarch64-apple-darwin` |
| `linux/amd64` | `x86_64-unknown-linux-gnu` |
| `linux/arm64` | `aarch64-unknown-linux-gnu` |

这里有一个现实约束：

- 当前 `csgclaw` 官方 release workflow 还没有启用 `linux/arm64`。
- 但 `boxlite-cli` 已经有 `linux/arm64` 预编译包。

所以建议分两步：

- 第一阶段：只支持当前已经发布的 `darwin/arm64`、`linux/amd64`。
- 第二阶段：恢复 `linux/arm64` release matrix，并同时纳入 bundle。

## 运行时改造建议

### 目标行为

`provider = "boxlite-cli"` 时，`csgclaw` 应按下面顺序解析最终执行路径：

1. 默认直接使用 bundle 内置的 `boxlite`
2. 仅当用户显式配置了 `boxlite_cli_path` 时，才覆盖默认 bundled path
3. `PATH` 回退只保留给开发环境或兼容性兜底，不应作为官方安装路径的依赖

如果目标真的是“用户不用关心，也不用安装 `boxlite-cli`”，那么官方安装路径上不能再把 `PATH` 查找当成正常路径，而只能把它当成 fallback。

建议不要把“bundle 解析”做成隐式修改 `boxlite_cli_path` 字符串，而是在 provider 初始化时解析成最终绝对路径。

### 推荐实现方式

新增一个“小而明确”的路径解析层，例如：

- `internal/sandbox/boxlitecli/resolve.go`
- 或 `internal/sandboxproviders/boxlite_cli_path.go`

核心逻辑建议是：

1. 通过 `os.Executable()` 找到当前 `csgclaw` 的真实路径。
2. 按官方约定路径定位 bundled `boxlite`。
3. 若 bundled `boxlite` 存在，则将其作为默认执行路径。
4. 若用户显式配置了 `boxlite_cli_path`：
   - 绝对路径则直接使用；
   - 相对路径则相对于当前工作目录或配置策略解释。
5. 只有在 bundled `boxlite` 缺失且用户未显式配置时，才回退到 `"boxlite"` / `PATH`。

换句话说，解析顺序应该是“bundle first, config override second”还是“config override first, bundle second”，要看你想把 `boxlite_cli_path` 定义为：

- 调试覆盖入口
- 还是正常运行时配置

如果目标是面向普通用户零感知，推荐定义成“调试覆盖入口”，也就是：

1. 若用户显式配置了 `boxlite_cli_path`，使用它
2. 否则使用 bundled `boxlite`
3. 最后才回退 `PATH`

这个顺序兼顾了工程可调试性和默认无感知体验。

### 建议只保留一个官方 bundle 约定路径

虽然可以同时探测多个路径，但长期看最好只公开一个稳定约定，否则后续安装脚本、文档、测试都容易分叉。

如果采用上文推荐布局，则建议唯一官方路径就是：

```text
<csgclaw bundle root>/bin/boxlite
```

实现上可通过：

- `os.Executable()` 得到 `<bundle root>/bin/csgclaw`
- 然后同目录找 `boxlite`

这样最简单，甚至不需要回退到 `../bin`。

也就是说，最稳的最终布局其实是：

```text
csgclaw/
  bin/
    csgclaw
    boxlite
```

并约定用户运行的是 `csgclaw/bin/csgclaw`，而不是把 `csgclaw` 单独拷走。

## 配置语义建议

这里最重要的是不要破坏已有配置兼容性。

建议保留现有字段：

```toml
[sandbox]
boxlite_cli_path = "boxlite"
```

但把语义从“默认从 PATH 中找命令”改成：

- 未显式配置时：使用 bundled `boxlite`
- 显式配置绝对路径时：强制使用该路径
- 显式配置相对路径时：按约定规则解释
- 只有 bundle 缺失且用户未显式配置时：才回退 `PATH`

这样可以做到：

- 老配置尽量不需要迁移
- 官方安装后的用户零配置可用
- 高级用户仍可手动覆盖

不建议为了这件事新增第二个配置项（如 `bundled_boxlite_cli_path`），因为会让心智模型更复杂。

## 打包流程改造建议

### 1. 增加下载并解压 boxlite-cli 的脚本

建议新增脚本，例如：

- `scripts/fetch-boxlite-cli.sh`

职责：

- 输入：`GOOS`、`GOARCH`、`BOXLITE_CLI_VERSION`、输出目录
- 根据平台映射拼接下载 URL
- 下载 `boxlite-cli-<version>-<target>.tar.gz`
- 解压出可执行文件
- 统一重命名为 `boxlite`
- 放到指定目录
- `chmod +x`

建议脚本参数形态：

```bash
./scripts/fetch-boxlite-cli.sh darwin arm64 /tmp/out/bin
```

环境变量：

```bash
BOXLITE_CLI_VERSION=v0.8.2
BOXLITE_CLI_BASE_URL=https://github.com/boxlite-ai/boxlite/releases/download
```

### 2. 改造 `scripts/package-release.sh`

当前脚本行为是：

- 编译一个二进制
- 直接打包该二进制

需要改成：

- 创建 staging 目录，例如 `${tmpdir}/csgclaw/bin`
- 将 `csgclaw` 编译到 `${tmpdir}/csgclaw/bin/csgclaw`
- 若 `APP=csgclaw`，额外拉取并放入 `${tmpdir}/csgclaw/bin/boxlite`
- 最终打包整个 `${tmpdir}/csgclaw` 目录

对于 `APP=csgclaw-cli`，建议保持原样，不要附带 `boxlite`。

原因：

- 用户问题只针对主程序 `csgclaw` 的 sandbox provider。
- `csgclaw-cli` 是否需要 `boxlite`，当前没有明确需求。
- 先缩小发布面，减少不必要的包体膨胀。

### 3. Makefile 增加版本参数和 bundle 目标

建议新增变量：

```make
BOXLITE_CLI_VERSION ?= v0.8.2
BOXLITE_CLI_BASE_URL ?= https://github.com/boxlite-ai/boxlite/releases/download
```

并让：

- `make package`
- `make release`

在 `APP=csgclaw` 时自动带上这些参数。

必要时可增加单独目标：

- `make package-with-bundled-boxlite-cli`

但如果团队已经决定正式采用 bundle 模式，通常直接把 `package` / `release` 切过去更干净。

## Release Workflow 改造建议

### 当前限制

`.github/workflows/release.yml` 现在：

- 只构建 `darwin/arm64`、`linux/amd64`
- 没有下载 `boxlite-cli`
- 上传的 `dist/*` 默认都视为最终 release asset

### 建议修改

1. 在 workflow 顶部增加：

```yaml
env:
  BOXLITE_CLI_VERSION: v0.8.2
```

2. `Package release archive` 这一步调用 `scripts/package-release.sh` 时，把 `BOXLITE_CLI_VERSION` 传进去。

3. 若要恢复 `linux/arm64`，把 matrix 中注释掉的条目恢复，并确认：
   - GitHub runner 可用
   - Go 交叉编译没问题
   - boxlite 对应平台 release 存在

4. 若将来要做 supply-chain 收敛，可以增加：
   - sha256 校验
   - 固定下载 URL
   - 失败时中止 release

## 新旧 release/install 流程并存策略

这里需要先明确一个原则：

- 可以保留旧的 `boxlite-sdk` 发布能力
- 但不建议长期维护两套并行的 workflow / package script / install script 副本

如果简单做成：

- `release.yml` 保留旧版，再复制一份新版
- `install.sh` 保留旧版，再复制一份新版
- `package-release.sh` 保留旧版，再复制一份新版

短期看起来切换成本低，但很快会出现这些问题：

- 版本号、平台矩阵、artifact 命名规则会分叉
- 每次 release 逻辑调整都要改两套
- README 和文档很难清晰表达“哪个才是官方默认路径”
- 默认安装体验容易继续落回旧路径，达不到“用户不用关心，也不用安装 `boxlite-cli`”的目标

所以更推荐的决策是：

- 以新的 `boxlite-cli bundle` 方案作为唯一主流程
- 旧的 `boxlite-sdk` 方案只保留为兼容模式（legacy mode）
- 尽量保留一份 workflow、一份 package script、一份 install script
- 通过参数、环境变量或 target 区分新旧模式

### 推荐的并存模型

#### 1. Release workflow 只保留一份

建议继续只保留：

- `.github/workflows/release.yml`

不要复制成：

- `release.yml`
- `release-boxlite-cli.yml`
- `release-boxlite-sdk.yml`

更合理的是在同一个 workflow 里区分产物模式，例如通过 env 或 matrix 控制：

- `PACKAGE_MODE=bundled-boxlite-cli`
- `PACKAGE_MODE=legacy-sdk`

但正式对外 release 时，应把：

- `bundled-boxlite-cli`

作为默认产物。

如果确实还要继续发布旧版兼容产物，建议：

- 命名上明确体现 `legacy` / `sdk`
- 不要让它和默认产物同名

例如：

```text
csgclaw_v0.1.0_darwin_arm64.tar.gz
csgclaw-sdk-legacy_v0.1.0_darwin_arm64.tar.gz
```

这样可以避免安装脚本或用户误拿旧产物当默认产物。

#### 2. package 脚本只保留一份

建议继续只保留：

- `scripts/package-release.sh`

不要再复制出一份 `package-release-legacy.sh` 作为长期方案。

更合理的方式是给脚本增加模式变量，例如：

```bash
PACKAGE_MODE=bundled-boxlite-cli
PACKAGE_MODE=legacy-single-binary
```

默认值应是：

```bash
PACKAGE_MODE=bundled-boxlite-cli
```

各模式语义建议如下：

- `bundled-boxlite-cli`
  - 产出目录型 bundle
  - 主程序附带 bundled `boxlite`
  - 用于正式对外 release
- `legacy-single-binary`
  - 保留当前单二进制归档逻辑
  - 仅用于兼容、回滚、内部验证

这样可以把公共逻辑继续共用：

- 版本号处理
- Go build 参数
- archive 命名
- staging 目录管理

而不是把大量重复逻辑拆成两份脚本。

#### 3. install 脚本只保留一份

建议继续只保留：

- `scripts/install.sh`

并让它默认安装新的 bundle 方案。

如果确实需要兼容安装旧版，可以增加一个显式参数或环境变量，例如：

```bash
INSTALL_VARIANT=default
INSTALL_VARIANT=legacy-sdk
```

但官方 README、官网安装命令、文档示例都应只展示默认值，也就是新方案。

否则虽然技术上“支持新方案”，但用户仍会被默认引导到旧流程，产品目标就落空了。

### 什么情况下才值得保留旧文件副本

只有下面两种情况，才建议额外保留一份单独文件：

1. 旧流程只是一个很短期的过渡分支，很快就会删除
2. 新旧流程结构差异过大，硬塞进同一份脚本会让脚本不可维护

即便如此，也建议：

- 文件名显式带 `legacy`
- 文件头写清楚用途、废弃状态、计划删除时间
- 不要继续作为默认入口

也就是说，可以临时存在：

- `scripts/install-legacy.sh`
- `release-legacy.yml`

但不建议把它们作为长期常态。

### 推荐的最终决策

结合当前目标，推荐采用下面这套策略：

- 保留一份 `.github/workflows/release.yml`
- 保留一份 `scripts/package-release.sh`
- 保留一份 `scripts/install.sh`
- 默认模式统一切到 `bundled-boxlite-cli`
- 旧 `boxlite-sdk` 路径只保留为 `legacy mode`
- 如果需要继续产出旧兼容包，命名上必须显式区分 `legacy` / `sdk`
- README 和文档只把新方案当作官方默认路径

一句话总结就是：

- 保留旧能力，但不要长期保留两套并行文件；应当保留一套主流程，把旧方案收敛成兼容模式。

## 安装流程改造建议

当前 `scripts/install.sh` 假设 archive 解压后只有一个 `${APP}` 文件，这会和 bundle 目录结构冲突。

建议把安装模型从“install 单文件”改成“install 整个目录”，例如：

- 安装到 `~/.local/lib/csgclaw/<version>/`
- 然后在 `~/.local/bin/csgclaw` 放一个 symlink 指向：
  - `~/.local/lib/csgclaw/<version>/csgclaw/bin/csgclaw`

这样做的好处：

- `boxlite` 与 `csgclaw` 的相对目录关系不会被破坏。
- 支持未来 bundle 更多辅助文件。
- 回滚或并存多个版本也更容易。

如果仍然坚持把 `csgclaw` 二进制复制到 `~/.local/bin`，那它就会脱离 bundle，运行时找不到旁边的 `boxlite`，设计会失效。

因此，安装脚本必须和 bundle 设计一起改，不能只改 release 归档。

并且从产品目标看，这里不是“最好改”，而是“必须改”：

- 只要官方安装路径还会破坏 bundle 相对关系，就达不到“用户不用关心，也不用安装 `boxlite-cli`”。

### 推荐安装目录形态

```text
~/.local/lib/csgclaw/v0.1.0/csgclaw/bin/csgclaw
~/.local/lib/csgclaw/v0.1.0/csgclaw/bin/boxlite
~/.local/bin/csgclaw -> ~/.local/lib/csgclaw/v0.1.0/csgclaw/bin/csgclaw
```

若后续要支持卸载/升级，可以再补：

- `current` 软链接
- 旧版本清理策略

## 测试改造建议

至少需要补三类测试。

### 1. 运行时路径解析测试

位置建议：

- `internal/sandbox/boxlitecli/resolve_test.go`

覆盖场景：

- 配置绝对路径时直接命中
- bundle 同目录存在 `boxlite` 时优先命中
- bundle 不存在时回退原始 `"boxlite"`
- 用户显式配置不存在的绝对路径时，保留原错误暴露

### 2. provider 装配测试

位置建议：

- `cli/serve/serve_test.go`
- 或 `internal/sandboxproviders/*_test.go`

覆盖场景：

- `provider = "boxlite-cli"` 时传入的是解析后的路径
- 不影响 `boxlite-sdk` provider 的现有行为

### 3. 打包脚本测试或至少 smoke test

如果不方便对 shell 脚本做单测，至少补一份人工验证步骤：

- 运行 `make package APP=csgclaw`
- 解压产物
- 确认存在：
  - `csgclaw/bin/csgclaw`
  - `csgclaw/bin/boxlite`
- 运行 `./csgclaw/bin/csgclaw serve`
- 观察 `boxlite-cli` provider 是否能找到 bundled `boxlite`

## 推荐的实施与交付顺序

在加入“新方案为主、旧方案收敛为 legacy mode”的决策后，阶段顺序需要稍作调整。

关键变化是：

- 安装脚本不能再放得太晚
- release packaging、release workflow、install script 需要按同一套 bundle 约定联动修改
- 不建议出现“本地包结构已经变了，但官方安装路径还停留在旧模型”的中间态

推荐按下面顺序推进。

### 阶段 1：先改运行时路径解析

目标：

- 先把 `boxlite-cli` provider 从“默认依赖 PATH”改成“支持 bundled binary”。
- 在不动正式 release 的前提下，先把核心运行时行为改对。

改动点：

- `internal/sandbox/boxlitecli` 增加 bundled path 解析函数
- `internal/sandboxproviders/boxlite_cli_provider.go` 使用解析后的路径
- 补路径解析与 provider 装配测试

验收：

- 显式配置路径时仍可覆盖
- bundle 同目录存在 `boxlite` 时可正常命中
- 现有 `PATH` fallback 不回归

### 阶段 2：改本地打包与安装模型

目标：

- 先把“产物结构”和“安装方式”配套改完。
- 确保新 bundle 一旦生成，就已经有对应的可用安装路径。

改动点：

- 新增 `scripts/fetch-boxlite-cli.sh`
- 修改 `scripts/package-release.sh`
- 修改 `Makefile`
- 修改 `scripts/install.sh`
- 若需要，也同步改 `scripts/install.ps1`

验收：

- `make package APP=csgclaw` 产出目录型 bundle
- bundle 内存在 `bin/csgclaw` 和 `bin/boxlite`
- 安装后 `csgclaw` 入口仍能通过相对路径找到 bundled `boxlite`

### 阶段 3：切换 GitHub release 主流程

目标：

- 把新的 bundled-boxlite-cli 方案设为正式 release 默认路径。
- 同时保留旧 `boxlite-sdk` 的 legacy mode，但不再作为默认发布方式。

改动点：

- 修改 `.github/workflows/release.yml`
- 在 workflow 中引入 `PACKAGE_MODE` 或等价模式参数
- 若继续产出旧包，统一调整 artifact 命名，显式区分 `legacy` / `sdk`

验收：

- workflow 默认产出 bundled 版 release asset
- 如保留旧兼容包，其名称与默认包清晰区分
- 手工下载 release asset 解压后结构正确

### 阶段 4：同步文档与对外入口

目标：

- 让 README、配置文档、安装说明全部以新方案为默认路径。
- 对外只暴露“安装 csgclaw 即可使用”的产品心智。

改动点：

- `README.md`、`README.zh.md`
- `docs/tech/config.md`、`docs/tech/config.zh.md`

验收：

- 文档默认不再要求用户单独安装 `boxlite-cli`
- `boxlite_cli_path` 被明确描述为高级覆盖项，而非普通用户前置配置

### 阶段 5：可选恢复 linux/arm64 release

目标：

- 把已有的 BoxLite Linux arm64 预编译包纳入正式 csgclaw release。

改动点：

- `.github/workflows/release.yml` matrix
- 安装脚本平台提示

验收：

- 产出 `linux/arm64` bundled release asset

## 风险与注意事项

### 1. 安装脚本是关键风险点

如果 release bundle 已经带了 `boxlite`，但安装脚本仍把 `csgclaw` 单独复制到 `~/.local/bin`，最终用户体验仍然会失败。

所以安装脚本不能晚太久改，否则“release 已支持，官方安装方式却不可用”。

### 2. 第三方二进制版本管理

需要明确 `csgclaw` 与 `boxlite-cli` 的兼容矩阵。建议至少先固定：

- `BOXLITE_CLI_VERSION=v0.8.2`

并在文档中注明：

- 当前 `csgclaw` release bundle 内置的 BoxLite CLI 版本
- 升级流程应如何同步修改

### 3. 体积增加

`csgclaw` release archive 会变大。这通常是可接受的，但要提前接受这个 tradeoff。

### 4. License / NOTICE

如果 BoxLite CLI 的分发要求附带 license 或 notice，release bundle 里可能要一起放。这个要在正式落地前确认。

### 5. macOS 可执行权限和签名

如果未来做 notarization / codesign，需要确认 bundle 中附带的 `boxlite` 是否也要纳入处理。

当前仓库似乎还没有这套流程，但后续可能会碰到。

## 首轮落地范围与最终决策

如果目标是“官方 release 安装后，用户无需单独安装 `boxlite-cli`，甚至不需要知道它的存在，就能直接使用 `boxlite-cli` provider”，那么首轮落地建议直接按下面这套方案执行。

### 最终决策

- `csgclaw` release 改为目录型 bundle，而不是单二进制归档
- bundle 内置 `bin/csgclaw` 与 `bin/boxlite`
- `boxlite-cli` provider 默认使用 bundled `boxlite`，而不是默认依赖 `PATH`
- `boxlite_cli_path` 退化为高级覆盖项，而不是普通用户必需配置项
- 官方安装脚本改为安装整个 bundle，并在 `PATH` 中暴露 `csgclaw` 入口
- release、package、install 只保留一套主流程；旧 `boxlite-sdk` 仅保留为 legacy mode
- 第一阶段只覆盖 `darwin/arm64` 与 `linux/amd64`，后续再补 `linux/arm64`

### 首轮最小改造范围

按上面的最终决策，第一轮至少要改这些内容：

- 在 `internal/sandbox/boxlitecli/` 下新增 bundled path 解析逻辑与测试
- 修改 [internal/sandboxproviders/boxlite_cli_provider.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/sandboxproviders/boxlite_cli_provider.go)
- 新增 [scripts/fetch-boxlite-cli.sh](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/scripts/fetch-boxlite-cli.sh)
- 修改 [scripts/package-release.sh](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/scripts/package-release.sh)
- 修改 [Makefile](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/Makefile)
- 修改 [scripts/install.sh](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/scripts/install.sh)
- 修改 [.github/workflows/release.yml](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/.github/workflows/release.yml)，让 bundled 方案成为默认主流程
- 同步更新 [README.md](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/README.md)、[docs/tech/config.md](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/docs/tech/config.md)、[docs/tech/config.zh.md](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/docs/tech/config.zh.md)

这套收敛后的范围，基本对应上文阶段 1 到阶段 4；阶段 5 的 `linux/arm64` 可以作为后续增强，不必阻塞首轮落地。
