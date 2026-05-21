# Web Documentation

This directory documents the CSGClaw frontend under `web/app`.

## Documents

- `development.md`: The main frontend development guide. Use this as the source of truth for tooling, source layout, component boundaries, imports, styling, state/data flow, accessibility, tests, and generated output rules.
- `development.zh.md`: Chinese companion to `development.md`. It mirrors the development guide for Chinese readers.
- `ui-components.md`: UI component library rules for `src/components/ui`, including Radix wrappers, package shape, styling, overlay layering, and Select conventions.
- `ui-components.zh.md`: Chinese companion to `ui-components.md`.
- `architecture.md`: A single ASCII architecture diagram for the frontend. Use it for a quick visual overview of build boundaries, runtime boundaries, source ownership, UI composition, data flow, dependency direction, component packaging, and quality gates.
- `architecture.zh.md`: Chinese companion to `architecture.md`. It explains the same architecture diagram in Chinese.

## Reading Order

Start with `architecture.md` or `architecture.zh.md` when you need a fast mental model of the frontend.

Use `development.md` or `development.zh.md` when you are creating, moving, or reviewing frontend code and need the exact rules.

Use `ui-components.md` or `ui-components.zh.md` when you are adding or changing shared UI primitives, Radix-based components, form controls, or overlay layering.
