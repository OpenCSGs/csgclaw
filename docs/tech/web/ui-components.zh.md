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

如果一个页面需要新的、强业务定制的 UI，可以先放在页面私有组件里实现。出现跨页面复用、交互契约稳定，或多个页面开始复制同类结构时，再抽取到 `src/components/ui` 或 `src/components/business`。抽取时同步更新本规范或 `docs/tech/web/development.md` 中的样式/组件规则。

## 组件包结构

- 每个导出组件包使用一个 PascalCase 目录。
- 实现、CSS 和 `index.ts` 放在同一个组件目录。
- 组件实现直接导入自身 `.module.css`；`index.ts` 只导出公开接口。
- 公共 UI 组件从 `src/components/ui/index.ts` 统一 re-export。
- 移动或替换组件后避免保留旧路径兼容 re-export；直接更新调用方。

## 样式

- 颜色、圆角、阴影、focus ring 和共享层级值优先使用 `src/shared/styles/tokens.css` 里的 token。
- 自有样式和动画使用同目录 CSS Modules；UI 样式不使用 `:global`、组件内主题选择器或 `!important`。
- 少量布局细节可以使用 Tailwind CSS utility class；稳定组件样式应使用组件自己的 class name。
- token 已存在时，不要在 UI 组件里使用裸的全局颜色、阴影或 z-index 值。
- UI 基础件应保持足够中性，便于跨页面复用。业务专属布局和文案留在 `src/components/ui` 之外。
- 明暗主题和共享状态角色统一在 `shared/styles/tokens.css` 管理；Tailwind 用于少量布局。
- 已有根节点和内容布局 class 仅在业务仍使用时作为兼容钩子保留，不覆盖基础控件的颜色、字体或状态；新增组合使用 props 和调用方布局 wrapper。
- Combobox 在 UI 库内部复用选择控件的视觉模块；业务代码导入组件入口，不导入私有 stylesheet。

## 按钮与导航链接

执行操作使用 `Button` 或 `IconButton`，页面导航使用 `LinkButton`。`LinkButton` 渲染原生链接，保留 `href`、`target`、`rel` 和键盘激活行为，与 Button 共用局部的变体、尺寸、内容及焦点样式。不要通过给原生元素添加全局 `btn` 类来复刻按钮。

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
- 从统一的 `radix-ui` 包导入 Radix namespace，例如 `import { Select as RadixSelect } from "radix-ui"`。不要为已经由 `radix-ui` 暴露的 primitive 新增 `@radix-ui/react-*` 分包依赖。
- 只有统一包没有暴露所需 primitive 时，才单独新增 Radix 分包，并在改动里说明原因。
- 如果 jsdom 缺少某个 Radix primitive 依赖的浏览器 API，在 `web/app/tests/setup.ts` 里补最小、稳定的 polyfill，并让组件测试聚焦用户可见行为。

## Select

`Select` 基于 `radix-ui` 包里的 `Select` namespace 实现。常规表单优先使用 data-driven 的 `options` prop。只有需要自定义布局时才使用 compound exports（`SelectRoot`、`SelectTrigger`、`SelectContent`、`SelectItem`）。

`Select` 会把业务侧空值（`""`）映射到内部 Radix item value，因为 Radix 把空字符串保留给清空选择。调用方仍然按 `""` 正常读写即可。

## Combobox、模型字段与基础表单

普通 Select 用于有限枚举；搜索单选使用 Combobox。Combobox 组合共享 Popover、TextInput，焦点保留在搜索输入内：方向键跳过禁用项，Enter 确认，Escape 关闭搜索层，Tab/Shift+Tab 按表单顺序离开。IME 确认不选择选项。支持受控 value/open 和非受控 defaultValue/defaultOpen；复杂 label 提供 textValue，并传入翻译后的搜索、空状态及 loading 文案。

空业务值仍为 `""`；隐藏的原生 select 保留 name/required 的提交与校验。过滤不改变已选值；异步加载或缺少选项时仍显示保存值。禁用项不能选择，loading 时不能确认。调用方通过设置 value 为 `""` 或提供明确空项清除选择。

弹层默认 Portal 到 document body，逃逸滚动容器裁切。在 Dialog 内使用嵌套 modal 焦点范围；Escape 保留父弹窗，Tab 回到弹窗内的下一项。contentProps.portalContainer 可指定不会裁切内容的宿主。保持视口碰撞处理，用 UI Lab 验收长列表、窄屏和弹窗焦点。

`components/business/AgentModelFields` 负责提供商信息、认证保护和切换模型规则；ModelPicker 负责失效旧模型的保留与提示。创建弹窗和详情共用这些业务字段，基础 Combobox 不依赖业务 catalog。

Field 向 TextInput、TextArea、SelectTrigger 和 Combobox 传递 label/control ID、hint/error 描述、invalid 和 required 可访问状态；显式 ID 与名称优先。Field 的 required 用于字段标记，需要原生校验时同时设置控件 required。一个 Field 对应一个逻辑控件。

TextInput、TextArea 拥有局部、带 data 标识的样式；历史输入和工作区选择器排除所有 `data-csg-control` 元素。收窄迁移期间，原生复合表单保留在 `shared/styles/inputs.css`，受管理 Field 的标签、提示和错误样式保持局部。紧凑输入使用 size="sm"；组合搜索容器负责边框和背景时使用 variant="plain"，避免 `!important`。

PanelHeader 提供内容与 actions 布局，EmptyState 提供标题、描述和操作，SegmentedControl 提供命名单选组及键盘导航。业务文案和持久化留给调用方。菜单表面、hover/selected、Tooltip 和 Dialog 背景颜色通过 shared/styles/tokens.css 的语义 token 管理。

在 web/app 运行 `corepack pnpm ui:lab`，打开独立 ui-lab.html 展示生产组件和真实工作区 CSS 的组合场景；无需应用数据，也不进入生产构建。

## Dropdown Menu

`DropdownMenu` 基于 `radix-ui` 包里的 `DropdownMenu` namespace 实现。菜单应组合共享的 `DropdownMenuRoot`、`DropdownMenuTrigger`、`DropdownMenuContent`、`DropdownMenuItem` 和 `DropdownMenuSeparator`，让页面继承一致的 hover、键盘、danger 状态、portal 和焦点行为。

## Dialog

`Dialog` 基于 `radix-ui` 包里的 `Dialog` namespace 实现。modal 流程应使用共享 dialog exports，让页面继承一致的 backdrop、portal、focus trap、关闭按钮、标题、描述和布局行为。dialog 或嵌套 floating child 需要留在特定层级内时，暴露并使用 `portalContainer`。
