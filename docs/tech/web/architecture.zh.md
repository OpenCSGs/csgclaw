# 前端架构图解

这张图用一个完整视角解释 `web/app` 前端架构，是 `development.zh.md` 的图形化补充。

英文版: `architecture.md`。

```text
+==============================================================================================+
|                                   CSGClaw Web 前端架构                                        |
|                           web/app/src 下的 Vite + React 应用                                  |
+==============================================================================================+

  构建与托管边界
  ----------------------------------------------------------------------------------------------

  +-----------------------+        +-----------------------+        +---------------------------+
  | 开发源码              |        | Vite 构建             |        | 嵌入产物                  |
  | web/app/src           | -----> | pnpm / make build-web | -----> | web/static-dist           |
  | 只在这里改源码        |        | 生成前端资源          |        | 由 Go embed 托管          |
  +-----------------------+        +-----------------------+        +---------------------------+


  运行时边界
  ----------------------------------------------------------------------------------------------

  +--------------------------------------------------------------------------------------------+
  | 浏览器运行时                                                                               |
  | React 渲染工作区 UI，管理交互状态，调用 HTTP API，接收实时数据                            |
  +---------------------------------------------+----------------------------------------------+
                                                |
                                                | HTTP JSON + SSE / shared worker 事件
                                                v
  +--------------------------------------------------------------------------------------------+
  | Go 服务端                                                                                  |
  | 托管嵌入的 Web UI，并暴露 app / im / agents / hub / upgrade APIs                          |
  +---------------------------------------------+----------------------------------------------+
                                                |
                                                | 后端编排
                                                v
  +--------------------------------------------------------------------------------------------+
  | 本地运行时                                                                                 |
  | agents, rooms, IM bridge, manager / worker runtime, hub templates, upgrade actions          |
  +--------------------------------------------------------------------------------------------+


  源码架构
  ----------------------------------------------------------------------------------------------

  web/app/src
  |
  +-- main.tsx
  |     只做 React 入口
  |
  +-- bootstrap/
  |     应用启动、providers、query client、错误边界、根装配
  |
  +-- routes/
  |     只做 URL 到页面的映射，不放页面实现细节
  |
  +-- pages/
  |     路由页面 owner 和页面私有模块
  |     |
  |     +-- WorkspacePage
  |     |     主工作区外壳: sidebar、layout、modals、overlays、nested outlet
  |     |
  |     +-- ConversationPage
  |     |     会话页面内容和会话页私有组件
  |     |
  |     +-- ComputerPage
  |     |     电脑页面内容和电脑页私有组件
  |     |
  |     +-- AgentPage
  |     |     Agent 页面内容和 Agent 页私有组件
  |     |
  |     +-- HubPage
  |           Hub 页面内容和 Hub 页私有组件
  |
  +-- hooks/
  |     共享 React controller 和复用 hook
  |     例如 workspace controller 组合导航、数据、mutation、UI 状态
  |
  +-- api/
  |     HTTP 传输边界
  |     负责 endpoint path、payload、response type、底层错误映射
  |     不承载渲染逻辑、React 状态或页面专属默认值
  |
  +-- models/
  |     纯领域逻辑
  |     负责 normalizer、formatter、serializer、routing helper、状态流转 helper
  |     不发请求、不读浏览器 storage、不依赖 React
  |
  +-- components/
  |     只放共享组件
  |     |
  |     +-- ui/
  |     |     与数据来源无关的基础组件: layout、buttons、form controls、icons
  |     |     不导入 api、pages、storage client、realtime client 或 route models
  |     |
  |     +-- business/
  |           跨页面复用的业务组件
  |           可以组合 UI 基础件和业务文案、状态、回调、API 整理后的数据
  |
  +-- shared/
        跨模块基础设施
        |
        +-- constants/   稳定应用契约: API 名称、agent 默认值、message types
        +-- i18n/        文案表、语言选择、翻译 helper
        +-- realtime/    event bus、SSE/shared worker 管道、订阅能力
        +-- storage/     storage key 和 local/session storage wrapper
        +-- styles/      全局 CSS、reset、设计 token、CSS 变量
        +-- theme/       主题选择和主题 helper
        +-- lib/         不依赖 React、API、storage、page 的通用 helper


  主界面组合
  ----------------------------------------------------------------------------------------------

  +--------------------------------------------------------------------------------------------+
  | WorkspacePage                                                                               |
  |                                                                                            |
  |  +-----------------------+      +--------------------------------------------------------+  |
  |  | WorkspaceSidebar      |      | WorkspaceMainPanel                                     |  |
  |  | tabs, groups, footer  |      | 通过 Outlet 渲染嵌套路由页面                           |  |
  |  +-----------------------+      |                                                        |  |
  |                                 | ConversationPage / ComputerPage / AgentPage / HubPage  |  |
  |  +-----------------------+      +--------------------------------------------------------+  |
  |  | WorkspaceModals       |      +--------------------------------------------------------+  |
  |  | 创建、邀请、配置      |      | WorkspaceOverlays                                      |  |
  |  +-----------------------+      | popover、preview、临时 UI                              |  |
  |                                 +--------------------------------------------------------+  |
  +--------------------------------------------------------------------------------------------+


  Router-first 工作区导航
  ----------------------------------------------------------------------------------------------

  工作区 pane 和 sidebar tab 都从 URL 推导。React Router 拥有 route location；
  Zustand 只保留非路由工作区状态。

  用户点击
      |
      v
  selectConversation / selectAgent / selectComputer / selectHub / selectWorkspaceTab
      |
      v
  pathForPane(pane, rooms)
      |
      v
  React Router navigate(canonicalPath)
      |
      v
  useLocation().pathname 变化
      |
      v
  paneFromLocation(pathname)
      |
      +------------------------------+
      |                              |
      v                              v
  activePane 派生值             workspaceTabForPane(activePane)
      |                              |
      v                              v
  嵌套路由页面                   Sidebar active tab / tab panel

  pathname 变化后可以运行辅助 effect:

    pathname changed
      -> conversation route 更新 activeConversationId
      -> 关闭 member/channel tools 等临时 UI
      -> 用 navigate(..., replace: true) 规范化 route alias

  不要把这些派生值复制到其它 store:

    URL / Router location  ---->  activePane  ---->  workspaceTab
            |                         |                 |
            +-------------------------+-----------------+
                              唯一来源

    workspaceUiStore 保留:
      locale、theme、sidebar collapsed、collapsed groups、
      activeConversationId、hub selection、composer/UI flags


  数据流
  ----------------------------------------------------------------------------------------------

  +--------------------+      +-----------------------+      +---------------------+
  | 用户操作           | ---> | 页面 / Controller     | ---> | api/                |
  | click, type, submit|      | load, mutate, decide  |      | 请求边界            |
  +--------------------+      +-----------+-----------+      +----------+----------+
                                           ^                             |
                                           |                             v
  +--------------------+      +------------+----------+      +---------------------+
  | UI 组件            | <--- | 页面状态 / Props      | <--- | models/             |
  | 只负责渲染         |      | loading/error/empty   |      | 归一化和整理数据    |
  +--------------------+      +-----------------------+      +----------+----------+
                                                                         ^
                                                                         |
  +--------------------+      +-----------------------+      +----------+----------+
  | 实时事件           | ---> | shared/realtime       | ---> | event adapter       |
  | SSE / worker data  |      | 订阅并分发            |      | 复用 model helper   |
  +--------------------+      +-----------------------+      +---------------------+


  依赖规则
  ----------------------------------------------------------------------------------------------

  允许的依赖方向:

    pages  --->  hooks/controllers  --->  api + models
      |                 |
      v                 v
    components/business ---> components/ui ---> shared

  先靠近 owner，再按真实复用提升:

    只属于一个页面       -> pages/<PageName>/
    页面私有组件         -> pages/<PageName>/components/
    共享基础 UI          -> components/ui/
    共享业务 UI          -> components/business/
    HTTP 边界            -> api/
    纯领域逻辑           -> models/
    跨模块基础设施       -> shared/

  避免:

    components/ui  ----X----> api/
    components/ui  ----X----> pages/
    pages/A        ----X----> pages/B private modules
    api/           ----X----> React state or rendering logic
    models/        ----X----> fetch, storage, browser effects


  组件包规则
  ----------------------------------------------------------------------------------------------

  导出的组件包:

    PascalCaseDirectory/
      PascalCaseDirectory.tsx
      PascalCaseDirectory.css
      index.ts
      types.ts / utils.ts / tests when needed

  升级成组件包的条件:

    有 CSS、tests、types、utils、constants、子组件，
    有两个及以上 importers，或增长到大约 150-200 行。


  质量门禁
  ----------------------------------------------------------------------------------------------

  只改文档                         -> 通读并检查 Markdown
  TypeScript 或 import 路径变化    -> pnpm --dir web/app typecheck
  前端源码变化                     -> pnpm --dir web/app lint
  纯 model/helper 变化             -> pnpm --dir web/app test
  标准前端验证                     -> pnpm --dir web/app check
  不写 static-dist 的打包冒烟      -> pnpm --dir web/app exec vite build --outDir /private/tmp/csgclaw-web-build --emptyOutDir
  明确要更新嵌入产物               -> pnpm --dir web/app build
```
