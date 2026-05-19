# 前端开发规范

本规范适用于 `web/app` 下的 Vite 前端应用。

英文版为 `development.md`，Agent 默认以英文版为准。

## 工具链

- 前端开发使用 Node.js 22.13.x 或更新版本，但不包括 Node.js 25 及以上。
- 仓库根目录有 `.nvmrc`；使用 `nvm` 时，在 `csgclaw` 根目录运行 `nvm use`。
- `web/app/package.json` 通过 `engines.node` 声明 Node.js 版本范围。
- 包管理使用 `pnpm`。`web/app/package.json` 通过 `engines.pnpm` 声明支持的 pnpm 版本，并保留 `packageManager` 字段供 Corepack 使用。
- 项目的 Make 目标会通过 `scripts/web-pnpm.sh` 执行前端包管理命令。该脚本只把 `.nvmrc` 作为默认 nvm 版本；如果当前 Node.js 已在支持范围内，就不会强制切换，并且优先使用已有的兼容 pnpm，之后才回退到 Corepack。
- 日常开发优先使用这些项目级命令:

```bash
make web-install
make web-dev
make build-web
```

- 如果手动执行前端包管理命令，使用支持范围内的 Node.js 和 pnpm。如果切换 Node.js 版本后本机没有 `pnpm`，用 Node.js 自带的 Corepack 安装推荐版本:

```bash
corepack enable
corepack prepare pnpm@11.1.3 --activate
pnpm --dir web/app install
```

## 范围

- 前端源码放在 `web/app/src`。
- `web/static-dist` 是给 Go embed 使用的构建产物。
- `web/static` 是 legacy 前端对照资产，除非任务明确要求，否则不要修改。
- 新增模式或依赖前，优先沿用 `web/app` 里已有的约定。

## 顶层结构

- `src/api/`: HTTP 请求封装和 API 边界代码。
- `src/bootstrap/`: 应用启动、providers、根装配、错误边界和应用级 wiring。
- `src/components/`: 跨页面复用组件。
- `src/models/`: 纯数据归一化、格式化、路由解析和领域 helper。
- `src/pages/`: 路由页面和页面私有模块。
- `src/shared/`: 跨模块通用的常量、i18n、storage key、realtime、theme、样式和通用 helper。

不要随意新增顶层目录。只有模块确实是跨领域通用，并且不适合现有目录时再新增。

## 源码目录细则

创建、移动或重组 `web/app/src` 下的文件时，使用这张表判断归属。

| 路径 | 职责 | 避免 |
| --- | --- | --- |
| `src/main.tsx` | React 入口。 | 应用逻辑、路由规则或 provider 细节。 |
| `src/bootstrap/` | 应用启动、providers、根装配、错误边界和共享 client。 | 页面私有行为、feature 专属 helper 或万能 constants 文件。 |
| `src/routes/` | 路由声明和 route 到 page 的装配。 | 页面实现细节。 |
| `src/api/` | HTTP client、请求封装、endpoint 类型和传输边界。 | 渲染逻辑、页面状态或可复用数据归一化。 |
| `src/models/` | 可共享或可独立测试的纯数据整理、格式化、解析、路由和领域 helper。 | React hooks、浏览器存储、fetch 调用或 UI 状态。 |
| `src/hooks/` | 复用型 React hooks，用于组合共享应用数据或 controller 状态。 | 只属于单个页面的 hooks；这类 hooks 放页面附近。 |
| `src/components/ui/` | 展示型基础组件、布局基础件、表单控件、按钮和图标。 | CSGClaw 业务状态或 API 数据行为。 |
| `src/components/business/` | 跨页面复用的业务组件，组合 UI 基础件和业务文案、状态、动作或 API 数据。 | 只被单个页面使用的组件。 |
| `src/pages/<PageName>/` | 路由页面和该页面拥有的模块。 | 没有真实复用前提前抽跨页面抽象。 |
| `src/pages/<PageName>/components/` | 页面私有组件。 | 从其它页面导入私有模块。 |
| `src/shared/constants/` | 小而明确的跨模块稳定契约常量，例如 API endpoint、agent 默认值、结构化消息 type 字符串。 | 单个万能 constants 文件、页面私有值或领域逻辑。 |
| `src/shared/i18n/` | 文案表、语言选择和翻译 helper。 | 只属于单个未国际化路径的一次性文案。 |
| `src/shared/storage/` | storage key 和 local/session storage 封装。 | 页面专属持久化策略。 |
| `src/shared/realtime/` | event bus、SSE/shared worker、实时事件解析和订阅 helper。 | 页面渲染或组件私有 effect。 |
| `src/shared/theme/` | 主题选择和主题相关共享逻辑。 | 组件 CSS 或页面专属视觉规则。 |
| `src/shared/styles/` | 全局 CSS、reset、设计 token 和全局 CSS 变量。 | 组件或页面自有样式。 |
| `src/shared/lib/` | 不依赖 React、API、storage 或页面的小型通用 helper。 | 应放进 `src/models/` 的领域 helper。 |

默认把代码放在最接近 owner 的地方。只有出现真实跨页面复用，或边界确实共享时，才提升到 `src/components`、`src/models`、`src/hooks` 或 `src/shared`。

如果后续某个子目录需要自己的规则，在该子目录加一个短 README，并从本规范链接过去。

## 常量与共享契约

- 页面私有常量放在拥有它的页面、组件、hook 或 helper 附近。
- 被 model helper 使用的纯领域常量放在对应的 `src/models/<domain>.ts`。路由常量、pane type、route segment alias 和 path builder 应一起放在 `src/models/routing.ts`。
- 被多个远距离模块导入的稳定跨模块契约，放在 `src/shared/constants/` 下的聚焦文件里，例如 `api.ts`、`agents.ts`、`messages.ts` 或 `workspace.ts`。
- 成组字符串值优先使用 enum-like 的 `as const` 对象，并从对象推导 union type；只有孤立的一次性常量或外部协议字符串才用平铺 `export const`。
- 除非外部 API 明确需要运行时 enum 对象，否则避免 TypeScript `enum`；`as const` 对象生成的 JavaScript 更简单，也更容易表达 alias。

## 页面模块

- 路由页面放在 `src/pages/<PageName>/`。
- 页面私有组件放在 `src/pages/<PageName>/components/`。
- 页面私有 hooks、helpers、constants、types 放在拥有它们的页面附近。
- 只有出现真实跨页面复用后，才把页面私有代码提升到 `src/components`、`src/models` 或 `src/shared`。
- 页面私有组件通过页面本地入口导入，例如 `./components`。

## 共享组件

- `src/components/ui/` 放展示型基础组件和图标。
- `src/components/business/` 放跨页面复用的业务组件。
- 不要把页面私有组件放进 `src/components/`。
- 纯图标组件放在 `src/components/ui/Icons/`。
- 当组件把 UI 基础件与业务状态、文案、动作或 API 数据组合起来时，它就是业务组件。

## 组件命名

- 导出的组件包使用 PascalCase 目录名。
- 主实现文件使用与组件相同的 PascalCase 名称。
- 每个导出的组件包都有 `index.ts` 入口。
- `index.ts` 只导入必要 CSS 并 re-export 公开符号。
- 避免 `message-content` 这种 kebab-case 组件目录，也避免 `workspace-views` 这种语义模糊的分组名。

示例:

```text
src/pages/WorkspacePage/components/
  AgentDetailPane/
    AgentDetailPane.tsx
    AgentDetailPane.css
    index.ts
```

## 组件粒度

- `src/components/**` 里的公开组件必须是文件夹包。
- `src/pages/<PageName>/components/**` 里导出的页面组件建议也是文件夹包。
- 很小的私有子组件可以留在父组件 `.tsx` 文件里，前提是它不导出、没有独立 CSS、测试、types 或 helpers。
- 当组件有 CSS、测试、`types.ts`、`utils.ts`、常量、子组件、两个及以上导入方，或超过约 150-200 行时，升级成文件夹包。

## 导入

- 共享应用模块使用 `@/` alias。
- 同一个 feature 或 page 包内部使用相对导入。
- 共享组件从包入口导入，例如 `@/components/ui` 或 `@/components/business/ProfileControls`。
- 组件移动后不要新增旧路径兼容 re-export，直接更新调用方。
- 不要从一个页面导入另一个页面的私有模块。确实复用时，先提升为共享代码。

## 样式

- 组件自有 CSS 与组件放在一起，并使用组件名，例如 `AgentDetailPane.css`。
- feature 级共享 CSS 可以放在该 feature 的 components 目录，例如 `WorkspaceComponents.css`。
- 全局样式和设计 token 放在 `src/shared/styles/`。
- 新增颜色、间距、阴影前优先使用已有 CSS 变量和 token。
- CSS class 名应绑定组件或 feature 语义，避免容易全局冲突的泛用名称。
- 不要把页面专属样式放进 `src/shared/styles/`。

## 状态与数据

- API 请求代码放在 `src/api/`。
- 数据整理、归一化、格式化和路由解析，如果是共享逻辑或足够复杂，应放在 `src/models/`。
- 临时 UI 状态留在拥有它的页面或组件里。
- 只有多个远距离模块都需要读写时，才引入共享状态。
- 不要把 fetch、数据归一化和渲染逻辑都混在一个大组件里；可以清晰拆分时就拆出去。

## 数据流

- 把 `src/api/` 当作传输边界。API 模块负责 endpoint path、请求 payload、响应类型、底层错误映射，以及与 OpenAPI/server 返回形态的兼容；不要在这里承载 React 状态、渲染决策或页面专属默认值。
- 原始 API 响应进入大范围 UI 代码前，如果转换逻辑会复用、足够复杂或需要回归测试，先转换成前端应用使用的形态。纯转换 helper 放在 `src/models/<domain>.ts` 或聚焦的 model 模块里。
- 路由页面或页面拥有的 hook 负责组合数据加载、mutation、loading/error/empty 状态和页面专属默认值。只要还没有其它页面复用，就把这层编排留在路由附近。
- 共享组件应接收已经整理好的 props 或聚焦的 callback。组件可以展示业务状态，但不要直接 fetch；除非它是 `src/components/business/` 下有明确跨页面用途的 app-aware 组件。
- `src/components/ui/` 下的 UI 基础件必须与数据来源无关。不要导入 `src/api/`、route model、storage client、realtime client 或页面模块。
- 实时事件、轮询和共享订阅放在 `src/shared/realtime/`；如果只有一个页面消费，放在页面拥有的 hook 里。能复用 HTTP 数据的 model helper 时，实时 payload 也通过同一套 helper 归一化。
- mutation 流程保持显式：在页面/controller 层调用 API，在一个明确位置更新本地或共享状态，并通过 props 暴露 loading、disabled 和 error 状态。只有回滚行为清晰时才使用 optimistic update。
- 只有状态确实需要跨远距离路由或布局区域共享时，才引入全局 store 或 context。单一可见 owner 的数据优先使用页面本地 hook 和派生 props。
- 优先测试数据流中的纯逻辑：API shape adapter、model normalizer、serializer、routing helper 和状态流转 helper。组件测试只补用户可观察的 wiring。

## i18n 与文案

- 需要翻译的用户可见文案放在现有 i18n message 结构里。
- 页面私有组件沿用当前模式，通过传入的 translator 函数获取文案。
- 除非当前路径本来就未国际化，否则不要在组件里硬编码新的双语 UI 文案。
- 代码注释和开发文档默认用英文；如果明确维护中文 companion 文档，再同步中文版本。

## 可访问性

- 交互元素尽量使用原生 button 和表单控件。
- 只有图标的按钮需要 `aria-label`，通常也需要 `title`。
- 可点击的非 button 元素必须保留键盘访问能力，或者直接改成 button。
- 不要移除可见 focus 状态。
- 操作可能失败时，loading、disabled 和 error 状态要在 UI 中明确表达。

## 测试与验证

- TypeScript 或 import 路径变化后，运行 `pnpm --dir web/app typecheck`。
- 前端源码变化后，运行 `pnpm --dir web/app lint`。
- 提交或共享改动前，运行 `pnpm --dir web/app format:check`；需要应用 Prettier 格式化时运行 `pnpm --dir web/app format`。
- 运行 `pnpm --dir web/app test` 执行前端 Vitest 测试。
- 运行 `pnpm --dir web/app check` 执行标准前端验证组合。
- 需要验证打包但不想改 `web/static-dist` 时，运行 `pnpm --dir web/app exec vite build --outDir /private/tmp/csgclaw-web-build --emptyOutDir`。
- 只有确实需要更新嵌入产物时，才运行 `pnpm --dir web/app build`。
- 数据整理、路由解析、格式化、parser、serializer、状态流转 helper 应优先补纯单元测试。典型目标包括 `src/models/**`、`src/shared/lib/**`，以及组件包里不依赖 React 渲染的纯逻辑 helper。
- 纯单元测试放在 `tests/models`、`tests/shared` 或匹配语义的专门目录下，保持小而聚焦：覆盖表格化边界、非法输入、默认值和回归案例；函数调用足够时，不要为了测试而渲染 React。
- 组件行为用 React Testing Library + jsdom：渲染公开组件，用 role/label/text 查询，用 `userEvent.setup()` 驱动交互，并断言用户可见输出、disabled/loading/error 状态或回调。
- 如果逻辑已经抽出来，不要用组件测试替代单元测试。先直接测纯 helper，再给用户可观察的组件 wiring 补一两个行为测试。
- 只有 jsdom 难以表达的场景才使用 browser 或 e2e 验证，例如布局、响应式、canvas/media、真实浏览器 API 或完整应用流程。

## 构建产物

- 不要手动编辑 `web/static-dist`。
- 使用前端构建流程重新生成 embedded assets。
- 如果验证构建意外写入了 `web/static-dist`，需要说明，并且只在明确安全或获得确认后清理。
