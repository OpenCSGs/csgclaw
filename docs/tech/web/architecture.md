# Frontend Architecture Diagram

This document explains the `web/app` frontend architecture as one complete ASCII diagram. It is a visual companion to `development.md`.

Chinese companion: `architecture.zh.md`.

```text
+==============================================================================================+
|                                   CSGClaw Web Frontend                                        |
|                         Vite + React app under web/app/src                                    |
+==============================================================================================+

  Build / Hosting Boundary
  ----------------------------------------------------------------------------------------------

  +-----------------------+        +-----------------------+        +---------------------------+
  | Developer Source      |        | Vite Build            |        | Embedded Output           |
  | web/app/src           | -----> | pnpm / make build-web | -----> | web/static-dist           |
  | edit here             |        | generated assets      |        | served by Go embed        |
  +-----------------------+        +-----------------------+        +---------------------------+


  Runtime Boundary
  ----------------------------------------------------------------------------------------------

  +--------------------------------------------------------------------------------------------+
  | Browser Runtime                                                                            |
  | React renders Workspace UI, owns interaction state, calls HTTP APIs, receives realtime data |
  +---------------------------------------------+----------------------------------------------+
                                                |
                                                | HTTP JSON + SSE / shared worker events
                                                v
  +--------------------------------------------------------------------------------------------+
  | Go Server                                                                                  |
  | hosts embedded Web UI, exposes app / im / agents / hub / upgrade APIs                      |
  +---------------------------------------------+----------------------------------------------+
                                                |
                                                | backend coordination
                                                v
  +--------------------------------------------------------------------------------------------+
  | Local Runtime                                                                               |
  | agents, rooms, IM bridge, manager / worker runtime, hub templates, upgrade actions          |
  +--------------------------------------------------------------------------------------------+


  Source Architecture
  ----------------------------------------------------------------------------------------------

  web/app/src
  |
  +-- main.tsx
  |     React entrypoint only
  |
  +-- bootstrap/
  |     App startup, providers, query client, error boundary, root assembly
  |
  +-- routes/
  |     URL to page mapping only; no page implementation details
  |
  +-- pages/
  |     Route owners and page-private modules
  |     |
  |     +-- WorkspacePage
  |     |     Main shell: sidebar, layout, modals, overlays, nested outlet
  |     |
  |     +-- ConversationPage
  |     |     Conversation route content and conversation-private components
  |     |
  |     +-- ComputerPage
  |     |     Computer route content and computer-private components
  |     |
  |     +-- AgentPage
  |     |     Agent route content and agent-private components
  |     |
  |     +-- HubPage
  |           Hub route content and hub-private components
  |
  +-- hooks/
  |     Shared React controllers and reusable hooks
  |     Example: workspace controller composes navigation, data, mutations, UI state
  |
  +-- api/
  |     HTTP transport boundary
  |     Owns endpoint paths, payloads, response types, low-level error mapping
  |     Must not own rendering logic, React state, or page-specific defaults
  |
  +-- models/
  |     Pure domain logic
  |     Owns normalizers, formatters, serializers, routing helpers, state transition helpers
  |     Must not fetch, read browser storage, or depend on React
  |
  +-- components/
  |     Shared components only
  |     |
  |     +-- ui/
  |     |     Source-agnostic primitives: layout, buttons, form controls, icons
  |     |     Must not import api, pages, storage clients, realtime clients, or route models
  |     |
  |     +-- business/
  |           Shared app-aware components
  |           May combine UI primitives with business labels, state, callbacks, API-shaped data
  |
  +-- shared/
        Cross-cutting infrastructure
        |
        +-- constants/   Stable app contracts: API names, agent defaults, message types
        +-- i18n/        Message catalogs, locale selection, translator helpers
        +-- realtime/    Event bus, SSE/shared worker plumbing, subscriptions
        +-- storage/     Storage keys and local/session storage wrappers
        +-- styles/      Global CSS, reset, design tokens, CSS variables
        +-- theme/       Theme selection and theme helpers
        +-- lib/         Generic helpers with no React, API, storage, or page dependency


  Main UI Composition
  ----------------------------------------------------------------------------------------------

  +--------------------------------------------------------------------------------------------+
  | WorkspacePage                                                                               |
  |                                                                                            |
  |  +-----------------------+      +--------------------------------------------------------+  |
  |  | WorkspaceSidebar      |      | WorkspaceMainPanel                                     |  |
  |  | tabs, groups, footer  |      | renders nested route page through Outlet               |  |
  |  +-----------------------+      |                                                        |  |
  |                                 | ConversationPage / ComputerPage / AgentPage / HubPage  |  |
  |  +-----------------------+      +--------------------------------------------------------+  |
  |  | WorkspaceModals       |      +--------------------------------------------------------+  |
  |  | create, invite, setup |      | WorkspaceOverlays                                      |  |
  |  +-----------------------+      | popovers, previews, transient UI                       |  |
  |                                 +--------------------------------------------------------+  |
  +--------------------------------------------------------------------------------------------+


  Router-First Workspace Navigation
  ----------------------------------------------------------------------------------------------

  Workspace pane and sidebar tab selection are derived from the URL. React Router owns the route
  location; Zustand keeps only non-route workspace state.

  User click
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
  useLocation().pathname changes
      |
      v
  paneFromLocation(pathname)
      |
      +------------------------------+
      |                              |
      v                              v
  activePane derived value      workspaceTabForPane(activePane)
      |                              |
      v                              v
  Nested route page             Sidebar active tab / tab panel

  Supporting effects may run after pathname changes:

    pathname changed
      -> update activeConversationId for conversation routes
      -> close transient member/channel tool UI
      -> normalize route aliases with navigate(..., replace: true)

  Do not copy these derived values into another store:

    URL / Router location  ---->  activePane  ---->  workspaceTab
            |                         |                 |
            +-------------------------+-----------------+
                         single source of truth

    workspaceUiStore keeps:
      locale, theme, sidebar collapsed state, collapsed groups,
      activeConversationId, hub selection, composer/UI flags


  Data Flow
  ----------------------------------------------------------------------------------------------

  +--------------------+      +-----------------------+      +---------------------+
  | User Action        | ---> | Page / Controller     | ---> | api/                |
  | click, type, submit|      | load, mutate, decide  |      | request boundary    |
  +--------------------+      +-----------+-----------+      +----------+----------+
                                           ^                             |
                                           |                             v
  +--------------------+      +------------+----------+      +---------------------+
  | UI Components      | <--- | Page State / Props    | <--- | models/             |
  | render only        |      | loading/error/empty   |      | normalize and shape |
  +--------------------+      +-----------------------+      +----------+----------+
                                                                         ^
                                                                         |
  +--------------------+      +-----------------------+      +----------+----------+
  | Realtime Events    | ---> | shared/realtime       | ---> | event adapter       |
  | SSE / worker data  |      | subscribe and dispatch|      | reuse model helpers |
  +--------------------+      +-----------------------+      +---------------------+


  Dependency Rules
  ----------------------------------------------------------------------------------------------

  Allowed direction:

    pages  --->  hooks/controllers  --->  api + models
      |                 |
      v                 v
    components/business ---> components/ui ---> shared

  Keep close to owner first:

    one page only        -> pages/<PageName>/
    page component       -> pages/<PageName>/components/
    shared primitive     -> components/ui/
    shared app-aware UI  -> components/business/
    HTTP boundary        -> api/
    pure domain logic    -> models/
    cross-cutting infra  -> shared/

  Avoid:

    components/ui  ----X----> api/
    components/ui  ----X----> pages/
    pages/A        ----X----> pages/B private modules
    api/           ----X----> React state or rendering logic
    models/        ----X----> fetch, storage, browser effects


  Component Package Rules
  ----------------------------------------------------------------------------------------------

  Exported component package:

    PascalCaseDirectory/
      PascalCaseDirectory.tsx
      PascalCaseDirectory.css
      index.ts
      types.ts / utils.ts / tests when needed

  Promote to package when:

    has CSS, tests, types, utils, constants, child components,
    two or more importers, or grows beyond about 150-200 lines.


  Quality Gates
  ----------------------------------------------------------------------------------------------

  Documentation only                  -> read and review Markdown
  TypeScript or import path changes   -> pnpm --dir web/app typecheck
  Frontend source changes             -> pnpm --dir web/app lint
  Pure model/helper changes           -> pnpm --dir web/app test
  Standard frontend verification      -> pnpm --dir web/app check
  Bundle smoke without static-dist    -> pnpm --dir web/app exec vite build --outDir /private/tmp/csgclaw-web-build --emptyOutDir
  Intentional embedded output update  -> pnpm --dir web/app build
```
