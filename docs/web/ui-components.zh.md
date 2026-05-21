# UI 组件库规范

本规范适用于 Vite 前端应用里的共享展示组件库 `web/app/src/components/ui`。

英文版为 `ui-components.md`，Agent 默认以英文版为准。

## 范围

`src/components/ui` 负责展示型基础件：布局基础件、按钮、图标、表单控件，以及基于 Radix 的交互基础件。UI 组件必须与数据来源无关，不应导入 API client、route 模块、workspace controller、页面模块、storage client 或 realtime client。

带业务语义的共享组件放在 `src/components/business`。页面私有组件放在所属页面目录下。

## 组件分层原则

项目内业务功能应优先使用已有组件库，而不是在页面里重复实现基础交互或样式：

1. 页面和 feature 代码优先组合 `src/components/business` 和 `src/components/ui`。
2. `src/components/business` 可以组合 UI 基础件、业务文案、业务状态和动作，但不应重新发明基础控件。
3. `src/components/ui` 封装可复用的展示型基础件，并负责把 Radix primitive 的交互能力包装成 CSGClaw 的本地 API 和视觉规范。
4. Radix primitive 是组件库的交互底座，不是业务页面的默认直接依赖。

如果一个页面需要新的、强业务定制的 UI，可以先放在页面私有组件里实现。出现跨页面复用、交互契约稳定，或多个页面开始复制同类结构时，再抽取到 `src/components/ui` 或 `src/components/business`。抽取时同步更新本规范或 `docs/web/development.md` 中的样式/组件规则。

## 组件包结构

- 每个导出组件包使用一个 PascalCase 目录。
- 实现、CSS 和 `index.ts` 放在同一个组件目录。
- 组件包 CSS 从该包的 `index.ts` 导入。
- 公共 UI 组件从 `src/components/ui/index.ts` 统一 re-export。
- 移动或替换组件后避免保留旧路径兼容 re-export；直接更新调用方。

## 样式

- 颜色、圆角、阴影、focus ring 和共享层级值优先使用 `src/shared/styles/tokens.css` 里的 token。
- 组件自有样式放在组件旁边。
- 少量布局细节可以使用 Tailwind CSS utility class；稳定组件样式应使用组件自己的 class name。
- token 已存在时，不要在 UI 组件里使用裸的全局颜色、阴影或 z-index 值。
- UI 基础件应保持足够中性，便于跨页面复用。业务专属布局和文案留在 `src/components/ui` 之外。
- 后续通用样式会逐步沉淀到 Tailwind CSS 和共享 token；页面级个性化样式可以先就近写在页面或 feature CSS 中，确认复用价值后再抽取。

## 浮层层级

层级值是共享设计 token。组件内部局部 stacking context 里的 `z-index: 1` 或 `2` 可以接受，但跨组件浮层必须使用这些 token：

| Token                | 用途                                                                 |
| -------------------- | -------------------------------------------------------------------- |
| `--z-page-popover`   | 保持在 app shell 内的页面级菜单和 popover。                          |
| `--z-page-overlay`   | 高于 app shell 的固定非 modal 预览面板。                             |
| `--z-modal`          | modal backdrop 和阻塞型 modal surface。                              |
| `--z-portal-popover` | 需要逃逸滚动容器或 modal card 的 portalled dropdown/select/popover。 |
| `--z-tooltip`        | 应高于 popover 的 tooltip 和帮助浮层。                               |

新增带 portal 的 Radix 组件时，条件允许就暴露 container escape hatch。如果一个 floating child 属于某个具体 modal 或 panel，优先把它 render 到该层的 container，而不是继续抬高 z-index。高位 portal token 只用于必须在任意页面或 modal 上下文都可工作的 UI 基础件。

## Radix Primitives

- Radix Primitives 文档见 <https://www.radix-ui.com/primitives/docs/overview/introduction>。
- Radix primitive 提供可访问性、键盘导航、焦点管理、受控/非受控状态、portal 等交互能力；它默认不带样式，样式由 CSGClaw 本地组件负责。
- 广泛使用 Radix primitive 前，先用本地 UI 组件封装。
- 保留 Radix 的交互契约，在其上组合 CSGClaw 的样式。
- 常规表单控件优先提供 data-driven API；只有需要自定义布局时才使用 compound exports。
- 页面或业务组件不要直接依赖 Radix primitive，除非这是一次明确的页面私有探索；一旦需要复用或形成稳定交互，应抽到 `src/components/ui`。
- 新增 Radix 依赖时，优先遵循项目现有依赖方式，并尽量让 Radix 相关包保持一致版本，避免重复 shared dependency。
- 如果 jsdom 缺少某个 Radix primitive 依赖的浏览器 API，在 `web/app/tests/setup.ts` 里补最小、稳定的 polyfill，并让组件测试聚焦用户可见行为。

## Select

`Select` 基于 `@radix-ui/react-select` 实现。常规表单优先使用 data-driven 的 `options` prop。只有需要自定义布局时才使用 compound exports（`SelectRoot`、`SelectTrigger`、`SelectContent`、`SelectItem`）。

`Select` 会把业务侧空值（`""`）映射到内部 Radix item value，因为 Radix 把空字符串保留给清空选择。调用方仍然按 `""` 正常读写即可。
