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

If a page needs a new highly customized UI, start with a page-private component. Promote it to `src/components/ui` or `src/components/business` when it has cross-page reuse, a stable interaction contract, or repeated copies across pages. Update this guide or the styling/component rules in `docs/web/development.md` when extracting the pattern.

## Package Shape

- Use one PascalCase folder per exported component package.
- Keep implementation, CSS, and `index.ts` together in that folder.
- Import package CSS from the package `index.ts`.
- Re-export public UI components from `src/components/ui/index.ts`.
- Avoid compatibility re-export paths after moving or replacing a component; update callers instead.

## Styling

- Prefer tokens from `src/shared/styles/tokens.css` for color, radius, shadow, focus rings, and shared layer values.
- Keep component-owned styles beside the component.
- Tailwind CSS utility classes are fine for small layout details, but stable component styling should use component-owned class names.
- Do not use raw global color, shadow, or z-index values in UI components when a token exists.
- Keep UI primitives visually neutral enough for reuse across pages. Business-specific layout and copy should stay outside `src/components/ui`.
- Shared styling will be gradually moved into Tailwind CSS and shared tokens. Page-specific styling may start near the page or feature CSS, then be extracted after its reuse value is clear.

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
- When adding Radix dependencies, follow the project's existing dependency style and keep Radix-related package versions aligned when practical to avoid duplicate shared dependencies.
- If jsdom lacks browser APIs needed by a Radix primitive, add the smallest stable polyfill in `web/app/tests/setup.ts` and keep component tests focused on user-visible behavior.

## Select

`Select` is built on `@radix-ui/react-select`. Prefer the data-driven `options` prop for normal forms. Use the compound exports (`SelectRoot`, `SelectTrigger`, `SelectContent`, `SelectItem`) only when a custom layout is needed.

`Select` maps an empty business value (`""`) to an internal Radix item value, because Radix reserves empty string for clearing selection. Callers should continue to read and write `""` normally.
