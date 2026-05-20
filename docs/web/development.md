# Frontend Development Guide

These rules apply to the Vite web app in `web/app`.

Chinese companion: `development.zh.md`. This English document is the agent-facing source of truth.

## Tooling

- Use Node.js 22.13.x or newer, up to but not including Node.js 25, for frontend development.
- The repository root has an `.nvmrc`; with `nvm`, run `nvm use` from the `csgclaw` root.
- `web/app/package.json` declares the required Node.js range in `engines.node`.
- Use `pnpm` for package management. `web/app/package.json` declares supported pnpm versions through `engines.pnpm` and keeps `packageManager` pinned for Corepack users.
- Project Make targets run package commands through `scripts/web-pnpm.sh`, which reads `.nvmrc` only as the default nvm version, accepts any supported current Node.js version, and uses an existing compatible pnpm before falling back to Corepack.
- Prefer these project-level commands for normal development:

```bash
make web-install
make web-dev
make build-web
```

- If you run package commands manually, use a supported Node.js and pnpm version. If `pnpm` is not available after switching Node.js versions, install the preferred version with Corepack:

```bash
corepack enable
corepack prepare pnpm@11.1.3 --activate
pnpm --dir web/app install
```

## Scope

- Keep frontend source in `web/app/src`.
- Treat `web/static-dist` as generated output for Go embedding.
- Prefer local conventions already present in `web/app` before adding new patterns or dependencies.

## Top-Level Structure

- `src/api/`: HTTP request wrappers and API boundary code.
- `src/bootstrap/`: app bootstrapping, providers, root assembly, error boundaries, and app-level wiring.
- `src/components/`: shared components used outside a single page.
- `src/models/`: pure data normalization, formatting, routing, and domain helpers.
- `src/pages/`: route-level pages and page-private modules.
- `src/shared/`: cross-cutting constants, i18n, storage keys, realtime utilities, theme, styles, and generic helpers.

Do not create new top-level directories unless a module is clearly cross-cutting and does not fit the existing structure.

## Source Directory Details

Use this map when creating, moving, or reorganizing files under `web/app/src`.

| Path | Owns | Avoid |
| --- | --- | --- |
| `src/main.tsx` | React entrypoint only. | App logic, routing rules, or provider setup details. |
| `src/bootstrap/` | App startup, providers, root app assembly, error boundaries, and shared clients. | Page-private behavior, feature-specific helpers, or catch-all constants. |
| `src/routes/` | Route declarations and route-to-page wiring. | Page implementation details. |
| `src/api/` | HTTP clients, request wrappers, endpoint types, and transport boundary code. | Rendering logic, page state, or reusable data normalization. |
| `src/models/` | Pure data shaping, formatting, parsing, routing helpers, and domain helpers that are shared or independently testable. | React hooks, browser storage, fetch calls, or UI state. |
| `src/hooks/` | Reusable React hooks that compose shared app data or controller state. | Hooks owned by one page only; keep those beside that page. |
| `src/components/ui/` | Presentation primitives, layout primitives, form controls, buttons, and icons. | CSGClaw-specific business state or API-backed behavior. |
| `src/components/business/` | Cross-page app-aware components that combine UI primitives with business labels, state, actions, or API data. | Components used by only one page. |
| `src/pages/<PageName>/` | Route screens and modules owned by one page. | Cross-page abstractions before real reuse exists. |
| `src/pages/<PageName>/components/` | Components private to that page. | Imports from another page's private modules. |
| `src/shared/constants/` | Small, named constant modules for stable cross-cutting contracts such as API endpoints, agent defaults, or structured message type strings. | A single catch-all constants file, page-private values, or domain logic. |
| `src/shared/i18n/` | Message catalogs, locale selection, and translation helpers. | One-off text that belongs to a single untranslated path. |
| `src/shared/storage/` | Storage keys and local/session storage wrappers. | Page-specific persistence policy. |
| `src/shared/realtime/` | Event bus, SSE/shared worker plumbing, realtime event parsing, and subscription helpers. | Page rendering or component-owned effects. |
| `src/shared/theme/` | Theme selection and theme-related shared logic. | Component CSS or page-specific visual rules. |
| `src/shared/styles/` | Global CSS, reset rules, design tokens, and app-wide CSS variables. | Component-owned or page-owned styles. |
| `src/shared/lib/` | Small generic helpers with no React, API, storage, or page dependency. | Domain helpers that belong in `src/models/`. |

Default to placing code near its owner. Promote code to `src/components`, `src/models`, `src/hooks`, or `src/shared` only after there is real cross-page reuse or a clear shared boundary.

If a subdirectory later needs its own rules, add a short README in that subdirectory and link it from this guide.

## Constants And Shared Contracts

- Page-private constants belong next to the page, component, hook, or helper that owns them.
- Pure domain constants used by model helpers belong in the owning `src/models/<domain>.ts` module. Routing constants, pane types, route segment aliases, and path builders should stay together in `src/models/routing.ts`.
- Stable cross-cutting contracts that are imported by multiple distant modules belong in focused files under `src/shared/constants/`, for example `api.ts`, `agents.ts`, `messages.ts`, or `workspace.ts`.
- For grouped string values, prefer enum-like `as const` objects plus derived union types over many flat exports. Use flat `export const` values only for isolated one-off constants or external protocol strings.
- Avoid TypeScript `enum` unless a runtime enum object is explicitly required by an external API; `as const` objects keep emitted JavaScript simpler and make aliases easier to model.

## Page Modules

- Put route-level screens under `src/pages/<PageName>/`.
- Put page-private components under `src/pages/<PageName>/components/`.
- Put page-private hooks, helpers, constants, and types next to the page that owns them.
- Move page-private code to `src/components`, `src/models`, or `src/shared` only after it has real cross-page reuse.
- Import page-private components through the page-local entrypoint, for example `./components`.

## Shared Components

- `src/components/ui/` contains presentation primitives and icons.
- `src/components/business/` contains shared app-aware components.
- Do not put page-private components in `src/components/`.
- Pure icon components belong under `src/components/ui/Icons/`.
- A component becomes business UI when it combines UI primitives with business state, labels, actions, or API-backed data.

## Component Naming

- Exported component packages use PascalCase directories.
- The primary implementation file uses the same PascalCase name as the component.
- Each exported component package has an `index.ts` entrypoint.
- `index.ts` files should only import required package CSS and re-export public symbols.
- Avoid kebab-case component directories such as `message-content` and vague grouping names such as `workspace-views`.

Example:

```text
src/pages/WorkspacePage/components/
  AgentDetailPane/
    AgentDetailPane.tsx
    AgentDetailPane.css
    index.ts
```

## Component Granularity

- Public components in `src/components/**` must be folder packages.
- Exported page components in `src/pages/<PageName>/components/**` should be folder packages.
- Tiny private subcomponents may stay inside the parent `.tsx` file when they are not exported and have no separate CSS, tests, types, or helpers.
- Promote a component to a folder package when it has CSS, tests, `types.ts`, `utils.ts`, constants, child components, two or more importers, or grows beyond about 150-200 lines.

## Imports

- Use the `@/` alias for shared app modules.
- Use relative imports for files inside the same feature or page package.
- Import shared components from package entrypoints, for example `@/components/ui` or `@/components/business/ProfileControls`.
- Do not add compatibility re-export folders for old paths after moving a component; update callers instead.
- Avoid importing page-private modules from another page. Promote truly shared code first.

## Styling

- Component-owned CSS lives next to the component and uses the component name, such as `AgentDetailPane.css`.
- Feature-level shared CSS can live in the feature components folder, such as `WorkspaceComponents.css`.
- Global styles and design tokens stay in `src/shared/styles/`.
- Prefer existing CSS variables and tokens before introducing new color, spacing, or shadow values.
- Keep CSS class names tied to component or feature semantics; avoid generic class names that can collide globally.
- Do not put page-specific styles in `src/shared/styles/`.

## State And Data

- Keep API request code in `src/api/`.
- Keep data shaping, normalization, formatting, and route parsing in `src/models/` when it is shared or large enough to test independently.
- Keep transient UI state inside the owning page or component.
- Add shared state only when state must be read or updated by multiple distant modules.
- Do not mix fetch calls, normalization, and rendering logic in one large component when the logic can be cleanly separated.

## Workspace Routing Maintenance

- Use the URL as the user-facing contract for workspace navigation. Each addressable pane needs a canonical route so deep links and browser back/forward keep working.
- Read the current pane from `paneFromLocation(useLocation().pathname)` and derive the visible workspace tab with `workspaceTabForPane`.
- When a user selects a conversation, agent, computer, hub, or workspace tab, build the destination with `pathForPane` and navigate through React Router.
- Keep route parsing and path building in `src/models/routing.ts`. Keep route declarations in `src/routes/`, and keep page behavior near `WorkspacePage`.
- Use Zustand only for supporting UI state that is not part of the URL, such as preferences, collapsed groups, hub selection, composer state, and transient UI flags.
- Route-change effects should stay explicit and limited to supporting state, such as updating `activeConversationId`, closing transient tools, or replacing route aliases with canonical paths.
- When adding or renaming a workspace pane, update route declarations, `src/models/routing.ts`, tab derivation, sidebar selection behavior, routing tests, and the architecture diagram together.

## Data Flow

- Treat `src/api/` as the transport boundary. API modules should own endpoint paths, request payloads, response types, low-level error mapping, and OpenAPI/server shape compatibility. They should not own React state, rendering decisions, or page-specific defaults.
- Convert raw API responses into app-facing shapes before they reach broad UI code when the conversion is reused, non-trivial, or needs regression coverage. Put those pure helpers in `src/models/<domain>.ts` or a focused model module.
- Route pages or page-owned hooks should compose data loading, mutations, loading/error/empty state, and page-specific defaults. Keep this orchestration near the route until another page needs the same behavior.
- Shared components should receive already-shaped props or focused callbacks. They may display business state, but they should not fetch directly unless they are intentionally app-aware components in `src/components/business/` with a documented cross-page use.
- UI primitives in `src/components/ui/` must stay data-source agnostic. They should not import `src/api/`, route models, storage clients, realtime clients, or page modules.
- Realtime events, polling, and shared subscriptions belong in `src/shared/realtime/` or in a page-owned hook when only one page consumes them. Normalize realtime payloads through the same model helpers used by HTTP data when possible.
- Keep mutation flows explicit: call the API at the page/controller layer, update local or shared state in one place, and surface loading, disabled, and error states through props. Use optimistic updates only when rollback behavior is clear.
- Introduce app-wide stores or context only for state that is genuinely shared across distant routes or layout areas. Prefer page-local hooks and derived props for data that has one visible owner.
- Test the pure parts of the flow first: API shape adapters, model normalizers, serializers, routing helpers, and state-transition helpers. Add component tests only for the user-visible wiring around those helpers.

### Workspace Controller Layering

- Keep `src/hooks/workspace/useWorkspaceController.ts` as the composition layer for `WorkspacePage`. It should gather shared data, call focused controller hooks, and assemble the props consumed by sidebar, route views, and overlays.
- Do not add large business flows directly to `useWorkspaceController.ts`. Put workspace behavior into focused hooks under `src/hooks/workspace/`:
  - `useConversationController`: rooms, invites, selected conversation state, IM realtime events, message composer, mentions, message list scrolling, and conversation modal props.
  - `useAgentController`: agent list/page state, create/edit modal state, manager profile setup, manager rebuild, provider auth, agent actions, agent direct messages, and template publishing.
  - `useUpgradeController`: upgrade modal state, apply-upgrade mutation, upgrade status updates, and reconnect polling.
  - `useProfilePreviewController`: participant/agent preview popover state, outside-click handling, and preview actions.
- Keep small supporting composition hooks separate when they have clear ownership, such as shell preferences/theme/localStorage wiring or Hub selection refresh state.
- Keep URL navigation focused in `useWorkspaceNavigation`; it should navigate panes and synchronize route-derived state, not own feature UI state such as member menus or channel tools.
- Controller hooks may call API functions and update query/local state, but pure parsing, normalization, serialization, and route helpers should stay in `src/models/` or `src/shared/`.
- When adding a new workspace feature, first decide which controller owns the user-visible workflow. Add a new controller only when the feature has a distinct lifecycle or state surface that would otherwise bloat an existing one.

## i18n And Text

- Keep user-facing strings in the existing i18n message structure when the text is translated.
- Use translator functions passed into page-private components when that is the surrounding pattern.
- Do not hardcode new bilingual UI strings in components unless the existing code path is already untranslated.
- Keep internal code comments and developer docs in English unless a Chinese companion document is explicitly being maintained.

## Accessibility

- Use native buttons and form controls for interactive elements whenever possible.
- Icon-only buttons need `aria-label` and usually `title`.
- Preserve keyboard access for clickable non-button elements, or convert them to buttons.
- Keep visible focus states intact.
- Make loading, disabled, and error states explicit in the UI when actions can fail.

## Tests And Verification

- Run `pnpm --dir web/app typecheck` after TypeScript or import-path changes.
- Run `pnpm --dir web/app lint` after frontend source changes.
- Run `pnpm --dir web/app format:check` before sharing changes, or `pnpm --dir web/app format` to apply Prettier formatting.
- Run `pnpm --dir web/app test` for the frontend Vitest suite.
- Run `pnpm --dir web/app check` for the standard frontend verification bundle.
- Run `pnpm --dir web/app exec vite build --outDir /private/tmp/csgclaw-web-build --emptyOutDir` when validating bundling without touching `web/static-dist`.
- Run `pnpm --dir web/app build` only when the generated `web/static-dist` output is intended.
- Add or update pure unit tests for data shaping, routing, formatting, parser, serializer, and state-transition helpers. Good targets include `src/models/**`, `src/shared/lib/**`, and logic-only helpers inside component packages.
- Put pure unit tests under `tests/models`, `tests/shared`, or a matching focused folder, and keep them small: table-test edge cases, invalid input, defaults, and regression cases without rendering React when a function call is enough.
- Use React Testing Library with jsdom for component behavior: render the public component, query by role/label/text, drive interactions with `userEvent.setup()`, and assert user-visible output, disabled/loading/error states, or emitted callbacks.
- Do not replace unit tests with component tests when logic is already extracted. Test pure helpers directly, then add one or two component tests for the wiring that a user can observe.
- Use browser or e2e verification only for behavior jsdom cannot represent well, such as layout, responsive behavior, canvas/media rendering, real browser APIs, or full app workflows.

## Generated Output

- Do not manually edit `web/static-dist`.
- Use the app build pipeline to regenerate embedded assets.
- If a verification build writes to `web/static-dist` accidentally, call that out and clean it only with explicit approval or a clearly safe generated-output workflow.
