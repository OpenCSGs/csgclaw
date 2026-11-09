# Component Modules

This directory holds the app's shared component entrypoint and cross-page compositions.

- `ui/index.ts` only re-exports `csgclaw-ui`. Implement reusable UI in the package.
- `business/` holds shared components with app dependencies, including product icons. Assess reusable parts for migration to `csgclaw-ui` when their props and theme contract are independent of the app.
- Page-private components belong under `src/pages/<PageName>/components/`.

See [docs/tech/web/development.md](../../../../../docs/tech/web/development.md) for the full frontend rules.
