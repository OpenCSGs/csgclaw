# UI Component Library

These rules apply to `web/app/src/components/ui`, the shared presentation component library for the Vite web app.

Chinese companion: `ui-components.zh.md`.

## Scope

`src/components/ui` owns presentation primitives: layout shells, buttons, icons, form controls, and Radix-based interaction primitives. UI components must stay data-source agnostic and should not import API clients, route modules, workspace controllers, page modules, storage clients, or realtime clients.

Business-aware shared components belong in `src/components/business`. Page-private components belong under the owning page directory.

## Component Layering

Business features should prefer the project's existing component library instead of rebuilding primitive interactions or styles inside pages:

1. Page and feature code should compose `src/components/business` and `src/components/ui` first.
2. `src/components/business` may combine UI primitives with business labels, state, and actions, but should not reinvent base controls.
3. `src/components/ui` wraps reusable presentation primitives and turns Radix primitive behavior into CSGClaw-local APIs and visual conventions.
4. Radix primitives are the interaction foundation for the component library, not the default direct dependency for business pages.

If a page needs a new highly customized UI, start with a page-private component. Promote it to `src/components/ui` or `src/components/business` when it has cross-page reuse, a stable interaction contract, or repeated copies across pages. Update this guide or the styling/component rules in `docs/tech/web/development.md` when extracting the pattern.

## Package Shape

- Use one PascalCase folder per exported component package.
- Keep implementation, CSS, and `index.ts` together in that folder.
- Import the component's `.module.css` from its implementation. Keep `index.ts` limited to public exports.
- Re-export public UI components from `src/components/ui/index.ts`.
- Avoid compatibility re-export paths after moving or replacing a component; update callers instead.

## Styling

- Prefer tokens from `src/shared/styles/tokens.css` for color, radius, shadow, focus rings, and shared layer values.
- Keep component-owned styles beside the component in CSS Modules, including animation names. UI modules do not use `:global`, per-component theme selectors or `!important`.
- Tailwind CSS utility classes are fine for small layout details, but stable component styling should use component-owned class names.
- Do not use raw global color, shadow, or z-index values in UI components when a token exists.
- Keep UI primitives visually neutral enough for reuse across pages. Business-specific layout and copy should stay outside `src/components/ui`.
- Keep semantic theme roles in `shared/styles/tokens.css`; light/dark changes update these roles rather than repeating primitive selectors. Tailwind remains suitable for small layout details.
- Existing root/content layout class hooks are retained only where business consumers still use them. They are compatibility surfaces, not permission to override primitive colors, fonts or states. New integrations use props and caller-owned wrappers.
- Combobox shares the selection presentation module inside the UI library; business consumers import component entrypoints rather than private stylesheets.

## Buttons And Navigation Links

Use `Button` or `IconButton` for commands and `LinkButton` for navigation. `LinkButton` renders a native anchor, preserves `href`, `target`, `rel` and keyboard activation, and shares Button's scoped variant, size, content and focus styles. Do not recreate a button by adding global `btn` classes to native elements.

## Breadcrumb

`Breadcrumb` is a semantic navigation pattern in the shared UI library; Radix Primitives does not provide a dedicated breadcrumb primitive. Use the `home` item for the home/root action and `items` for path segments. Mark the current segment with `current`, and provide either `href` or `onClick` for navigable segments. Long paths collapse their middle segments into the shared Radix Dropdown Menu; `maxVisibleItems` and `overflowLabel` allow callers to adapt the layout and localization.

## Overlay Layers

Layer values are shared design tokens. Local component stacking such as `z-index: 1` or `2` is acceptable inside a contained stacking context, but cross-component overlays must use these tokens:

| Token                | Use                                                                                           |
| -------------------- | --------------------------------------------------------------------------------------------- |
| `--z-page-popover`   | Page-local menus and popovers that stay inside the app shell.                                 |
| `--z-page-overlay`   | Fixed non-modal preview panels above the app shell.                                           |
| `--z-modal`          | Modal backdrops and blocking modal surfaces.                                                  |
| `--z-portal-popover` | Portalled dropdowns, selects, and popovers that must escape scroll containers or modal cards. |
| `--z-tooltip`        | Tooltips and help flyouts that should sit above popovers.                                     |

When adding a Radix component with a portal, expose a container escape hatch when practical. If a floating child belongs to a specific modal or panel, prefer rendering it into that layer's container instead of raising its z-index. Use high portal tokens only for UI primitives that must work from any page or modal context.

## Radix Primitives

- Radix Primitives documentation: <https://www.radix-ui.com/primitives/docs/overview/introduction>.
- Radix primitives provide accessibility, keyboard navigation, focus management, controlled/uncontrolled state, portals, and other interaction behavior. They are unstyled by default; CSGClaw local components own styling.
- Wrap Radix primitives behind local UI components before using them broadly.
- Keep the Radix interaction contract intact, and compose CSGClaw styling around it.
- Prefer data-driven APIs for common form controls, with compound exports available only for custom layouts.
- Pages and business components should not depend on Radix primitives directly unless it is an explicit page-private exploration. Extract the wrapper to `src/components/ui` once the interaction needs reuse or becomes a stable convention.
- Import Radix namespaces from the unified `radix-ui` package, for example `import { Select as RadixSelect } from "radix-ui"`. Do not add `@radix-ui/react-*` package dependencies for primitives that are available through `radix-ui`.
- Add a separate Radix package only when the unified package does not expose the needed primitive, and document why in the change.
- If jsdom lacks browser APIs needed by a Radix primitive, add the smallest stable polyfill in `web/app/tests/setup.ts` and keep component tests focused on user-visible behavior.

## Select

`Select` is built on the `Select` namespace from `radix-ui`. Prefer the data-driven `options` prop for normal forms. Use the compound exports (`SelectRoot`, `SelectTrigger`, `SelectContent`, `SelectItem`) only when a custom layout is needed.

`Select` is for finite, non-searchable choices. Use `Combobox` for searchable single selection.

`Select` maps an empty business value (`""`) to an internal Radix item value, because Radix reserves empty string for clearing selection. Callers should continue to read and write `""` normally.

## Combobox And Model Fields

`Combobox` composes the shared Popover and TextInput with a single input-focus model. Arrow keys skip disabled options, Enter confirms, Escape closes the picker, and Tab/Shift+Tab leave it in form order. IME confirmation must not select an option. `value`/`open` may be controlled; `defaultValue`/`defaultOpen` support uncontrolled use. Use `textValue` for JSX labels, and supply translated search, empty and loading labels.

Empty business values remain `""`; a hidden native select preserves `name`/`required` submission. Filtering does not change the selected value, and a saved value remains visible when options are missing or loading. Disabling an option prevents selection. Loading prevents confirmation. Callers clear selection by setting `value` to `""` or providing an explicit empty option.

Content portals to the document body to escape scroll-container clipping. Inside a Dialog it uses a nested modal focus scope; Escape leaves the parent dialog open and Tab returns to its next field. `contentProps.portalContainer` remains an escape hatch for a suitable unclipped host. Keep collision detection enabled and verify long lists and narrow viewports in UI Lab.

Provider metadata, authentication guards, model discovery and retained unavailable models belong to `components/business/AgentModelFields`. Its `ModelPicker` wraps the generic Combobox; creation and detail views share the same provider/model field rules.

## Fields And Reusable Layouts

`Field` supplies control/label IDs, hint/error descriptions, invalid state and required accessibility state to TextInput, TextArea, SelectTrigger and Combobox. Explicit IDs and accessible names take precedence. `required` on Field marks the field; set `required` on the control when native form validation is needed. Keep one logical control per Field.

TextInput and TextArea own their local, data-marked styles. Legacy input and workspace selectors exclude all `data-csg-control` elements. Compound native forms remain in `shared/styles/inputs.css` during the scoped migration; the managed Field has local label/hint/error styles. Use `size="sm"` for compact text inputs and `variant="plain"` when a composed search container owns the border/background. Avoid overrides with `!important`.

PanelHeader composes content and an optional actions slot, EmptyState displays a title/description/actions, and SegmentedControl provides a named single-choice group with keyboard navigation. Business copy and persistence remain with callers.
Use `size="sm"` for compact segmented controls in toolbars or preview headers.
Use `variant="text"` when a compact switch should have no surrounding border or selected fill; its selected text uses the accent color.

Semantic tokens own menu surfaces, hover/selected states, tooltip colors and dialog backdrops. Add a theme role in `shared/styles/tokens.css` when a primitive needs a new color relationship. Component CSS consumes the role without repeating light/dark branches.

The standalone `ui-lab.html` entry demonstrates these production components without application data, including integration with the real legacy workspace stylesheet entrypoints. Start it with `corepack pnpm ui:lab` from `web/app`; it is a development fixture and is excluded from production build inputs.

## Slider

`Slider` wraps Radix's single-thumb slider with shared theme tokens, optional step marks, a translated accessible label and value text. Use `onValueChange` for live previews and `onValueCommit` for persistence after pointer release or keyboard changes. `neutral` represents an automatic value outside the numeric scale. Reasoning-specific levels, descriptions and default actions live in `components/business/ReasoningEffortPicker`, shared by the start panel and conversation toolbar.

## Dropdown Menu

`DropdownMenu` is built on the `DropdownMenu` namespace from `radix-ui`. Compose menus with the shared `DropdownMenuRoot`, `DropdownMenuTrigger`, `DropdownMenuContent`, `DropdownMenuItem`, and `DropdownMenuSeparator` exports so pages inherit consistent hover, keyboard, danger-state, portal, and focus behavior.

Use `size="sm"` on `DropdownMenuContent` for compact menus. Section headings use `DropdownMenuLabel`; reserve disabled menu items for unavailable actions.

## Dialog

`Dialog` is built on the `Dialog` namespace from `radix-ui`. Use the shared dialog exports for modal workflows so pages inherit consistent backdrop, portal, focus trap, close button, title, description, and layout behavior. Expose `portalContainer` when a dialog or nested floating child needs to stay inside a specific layer.

## Switch

`Switch` wraps the `Switch` namespace from `radix-ui` for boolean settings. Use `checked`, `onCheckedChange`, and `disabled` for controlled updates, and provide an accessible name through `aria-label` or `aria-labelledby`. The shared styles support keyboard focus, disabled states, and reduced motion.
