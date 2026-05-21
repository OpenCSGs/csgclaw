# UI Component Library

These rules apply to `web/app/src/components/ui`, the shared presentation component library for the Vite web app.

Chinese companion: `ui-components.zh.md`.

## Scope

`src/components/ui` owns presentation primitives: layout shells, buttons, icons, form controls, and Radix-based interaction primitives. UI components must stay data-source agnostic and should not import API clients, route modules, workspace controllers, page modules, storage clients, or realtime clients.

Business-aware shared components belong in `src/components/business`. Page-private components belong under the owning page directory.

## Package Shape

- Use one PascalCase folder per exported component package.
- Keep implementation, CSS, and `index.ts` together in that folder.
- Import package CSS from the package `index.ts`.
- Re-export public UI components from `src/components/ui/index.ts`.
- Avoid compatibility re-export paths after moving or replacing a component; update callers instead.

## Styling

- Prefer tokens from `src/shared/styles/tokens.css` for color, radius, shadow, focus rings, and shared layer values.
- Keep component-owned styles beside the component.
- Utility classes are fine for small layout details, but stable component styling should use component-owned class names.
- Do not use raw global color, shadow, or z-index values in UI components when a token exists.
- Keep UI primitives visually neutral enough for reuse across pages. Business-specific layout and copy should stay outside `src/components/ui`.

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

- Wrap Radix primitives behind local UI components before using them broadly.
- Keep the Radix interaction contract intact, and compose CSGClaw styling around it.
- Prefer data-driven APIs for common form controls, with compound exports available only for custom layouts.
- If jsdom lacks browser APIs needed by a Radix primitive, add the smallest stable polyfill in `web/app/tests/setup.ts` and keep component tests focused on user-visible behavior.

## Select

`Select` is built on `@radix-ui/react-select`. Prefer the data-driven `options` prop for normal forms. Use the compound exports (`SelectRoot`, `SelectTrigger`, `SelectContent`, `SelectItem`) only when a custom layout is needed.

`Select` maps an empty business value (`""`) to an internal Radix item value, because Radix reserves empty string for clearing selection. Callers should continue to read and write `""` normally.
