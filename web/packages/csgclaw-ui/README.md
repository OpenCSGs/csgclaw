# csgclaw-ui

React UI primitives adapted from CSGClaw. Includes Button, Breadcrumb, Checkbox, Dialog, DropdownMenu, Field, TextInput, TextArea, Select, Combobox, Popover, SegmentedControl, Slider, Switch, Tooltip, GlobalNotice, EmptyState, DismissibleAlert, and PanelHeader.

## Install

```sh
pnpm add csgclaw-ui
```

The package uses React 19.2 and exports ESM, TypeScript declarations, and compiled CSS.

```tsx
import { Button, Checkbox, Field, TextInput } from "csgclaw-ui";
import "csgclaw-ui/styles.css";

export function Example() {
  return (
    <form>
      <Field label="Workspace name" required>
        <TextInput name="name" required />
      </Field>
      <Checkbox aria-label="Select all" checked="indeterminate" />
      <Button type="submit" variant="primary">
        Save
      </Button>
    </form>
  );
}
```

Load the CSS once in your app entry. Components ship with compiled CSS Modules and do not require Tailwind or an application-wide reset.

## Theme and overlays

The default theme is light. Set `data-csgclaw-ui-theme="dark"` on the document root for a dark theme that includes portalled overlays. Override `--csgclaw-ui-*` variables to customize the theme. For a nested theme, pass a host inside that theme to the overlay's `portalContainer` option.

Use a `TooltipProvider` around any subtree that uses tooltips, including the shared `DialogCloseButton`.

## Develop in CSGClaw

Run `corepack pnpm install --frozen-lockfile` from `web/`.
The app uses `csgclaw-ui: workspace:*` and reads this package's source during Vite development and builds. Run `make web-dev` from the repository root or `corepack pnpm dev` from `web/packages/app/` to edit and preview the app and primitives together.

Run `corepack pnpm --filter csgclaw-ui build` from `web/` to create the npm artifacts. Use `npm pack` from this package to test the release in another app before publishing.

## Releases

Bump `version` in this package's `package.json` and merge the change to upstream `main`. The release workflow skips unchanged and already published versions. Stable releases go to `latest` and prereleases go to `next`.

For one-time setup, restrict the GitHub Actions environment `csgclaw-ui-release` to deployments from `main`. In the npm package's Trusted Publisher settings, select GitHub Actions and authorize `OpenCSGs/csgclaw`, workflow `publish-csgclaw-ui.yml`, and environment `csgclaw-ui-release`. Enable direct publishing for that publisher. The workflow uses OIDC for npm authentication.

## Source and license

See [NOTICE](./NOTICE) for the upstream source and [LICENSE](./LICENSE) for its terms.
