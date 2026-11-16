# Application foundations

This UI Lab package covers the 22 previously missing Application component categories from Figma file `2s3AVSZ5FfBnCR5nI2AriR`. Tabs, section footers, pagination and progress steps remain in `NavigationFoundations`.

Each category contains a localized description, a representative interactive playground, one native DOM design reference, a Figma source link and a searchable design-node inventory. The inventory includes internal primitives as well as composed variants; it does not imply implementation coverage.

- `registry.ts` owns the category inventory and sidebar entries.
- `figmaInventory.json` records source page IDs, the selected design node and the other indexed nodes.
- `figmaReferences.json` stores static DOM fixtures derived from high-fidelity Figma design contexts, rather than screenshot images or executable code.
- `FigmaReferences.module.css` contains isolated, compiled reference styles. Literal fallback values retain the exported design palette; private CSS properties use the `--lab-figma-` prefix. Reference text uses the confirmed PingFang SC weights.
- `assets/` contains unchanged Figma source assets. SVG root dimensions are retained. Vite imports these assets from this development-only entry; they are not copied into the production public directory.
- `FigmaReference.tsx` renders the static fixtures inside an inert, locally scrolling container. Original source copy remains unchanged. References and long node lists are only mounted when expanded.
- Playground modules compose existing production primitives. State, filtering, dates and validation are local examples; no business API is called. Date selection requires explicit apply. File selection only validates type and size locally. Messaging and calendars retain data in memory.

The snapshots establish representative coverage, not approval of every size, state, responsive variant or design mode. Full variant parity, pixel acceptance and production migration remain separate work. Some Figma screenshots were rendered with a different variable mode than the returned context fallbacks; the reference uses the exported fallback palette and does not assert light/dark mode parity.

UI Lab preserves each visited application's demo state. Application panels are mounted on first visit to keep initial rendering and unrelated localization tests lightweight.

## Verification (2026-10-30)

- Type checking, ESLint, formatting and a temporary build including `ui-lab.html`: pass.
- Complete frontend suite: 2260 passed, 7 failed in the existing diagnostics ZIP and cp1252 encoding tests. UI Lab and ownership tests pass.
- IAB: Chinese desktop at 1280×720, English mobile at 390×844, and table layout at 768×900. All 22 new pages were visited; no page-wide horizontal overflow was observed. Original references and visible image slots were inspected, including intrinsic SVG dimensions. Fractional SVG widths below 1px can report zero rounded natural width while retaining their declared dimensions.
- LAY-01/03/04: pass for the inspected layouts and modal/drawer/date-popover flows. Date popovers now scroll within Radix's available height.
- FORM-02/03, FLOW-02/04 and LOC-01/02: pass for the tested validation, keyboard selection, cancel/apply, focus return and language-retention paths. Other complete Figma variant matrices and alternate design variable modes: not verified. Separate browser zoom configurations: not verified.
