# Interaction Design Rules

Apply these rules when changing user-visible layout, controls, navigation, fields, icons, copy, localization, or interaction states in `web/app`. This English document is the agent-facing source of truth; [Chinese companion](interaction-design.zh.md).

[Development](development.md) owns code structure, styling, i18n mechanisms and test commands. [UI components](ui-components.md) owns primitive APIs, keyboard contracts and layering. This document owns user-visible outcomes. The [walkthrough plan](../../ux/interaction-quality-plan.zh.md) owns rollout, issue records and evaluation.

## Before Implementation

- Identify affected pages, shared components, locales and user tasks. Select applicable rules below before coding.
- Reuse the approved design, existing component contracts and established terminology. Do not copy a known defect as a convention.
- Fix the shared cause when a defect affects multiple consumers, and verify affected consumers. Keep unrelated redesigns outside the task.
- Use the agreed browser, viewport and role support scope. If none exists, record the tested scope and unresolved limits; do not silently establish a mobile support promise or a new minimum width.

## Layout And Content

- **LAY-01:** At supported sizes, navigation, content and actions must remain readable and reachable without overlap or unintended page-wide horizontal scrolling. Wide tables may scroll within a deliberate container.
- **LAY-02:** Long text, long unbroken identifiers, empty values and large lists must not push essential actions out of reach. Choose wrapping or truncation by content purpose; provide access to essential full values. Do not shrink text or hide required information just to make it fit.
- **LAY-03:** Dialogs and drawers must keep their title, close and completion actions reachable on short and narrow viewports. Fixed regions must not cover content; scroll ownership must be clear.
- **LAY-04:** Open menus, selects, popovers and tooltips must remain usable near viewport edges and inside scroll containers or dialogs. Use shared collision and layer behavior rather than arbitrary z-index increases.
- **LAY-05:** Preserve approved alignment, hierarchy and primary-action placement using existing layout components and tokens.

## Icons, Copy And Localization

- **TXT-01:** Use established icons and terms for the same action or object across navigation, titles, fields, dialogs and feedback. Explain intentional contextual differences in the change.
- **TXT-02:** Action labels must describe the actual result; entry labels and destination titles must agree in meaning. Icon-only controls need a localized accessible name and, when unclear, a visible hint available on hover and focus.
- **TXT-03:** Follow existing conventions for punctuation, capitalization, units and date/time formatting. Do not invent translations for product/domain terms when an established term exists.
- **TXT-04:** Keep button labels concise and understandable: name the action and only the object needed in context. Move explanations and prerequisites into nearby supporting text instead of long sentences inside ordinary action buttons. Preserve meaningful consequences such as publishing, signing out or restarting, and distinguish toggle and pending states. Review actual Chinese and English labels in the available space and relevant states; labels must not crowd adjacent actions, overlap or lose their essential meaning through truncation. Do not solve excessive length by shrinking text or replacing a clear action with an unexplained icon. Character/word counts are triage hints, not universal limits; product names, necessary compound actions and descriptive selection cards need contextual review.
- **LOC-01:** Update all supported message catalogs for changed translated UI paths, including placeholders, validation, empty states, tooltips, menus and asynchronous feedback. Do not expose raw translation keys or unintended mixed-language fallback text.
- **LOC-02:** Verify translated layouts with actual Chinese and English strings where supported, including long labels. Locale switching must update newly opened overlays and feedback without discarding editing state.
- **LOC-03:** Localize system labels and enumerated statuses through existing mappings; preserve user content, IDs, paths and protocol values. For example, translate the label of a status rather than changing its API value.

## Fields, Controls And Keyboard

- **FORM-01:** Give fields persistent, associated labels; distinguish required, optional, readonly and disabled states. Explain meaningful defaults and units. A placeholder must not be the only label.
- **FORM-02:** Validate relevant empty, boundary and invalid inputs. Associate actionable errors with their fields and preserve valid user input on failure; do not persist secrets to implement recovery.
- **FORM-03:** Keep logical Tab order and visible focus. Preserve shared controls' Enter/Escape and focus-restoration contracts; IME confirmation must not trigger submission or selection. Do not make Enter submit multiline text unexpectedly.
- **FORM-04:** Distinguish applicable focus, hover, selected, disabled, invalid and loading states. Do not rely on color alone to convey required information or recreate primitive behavior in pages.

## Feedback And Task Continuity

- **FLOW-01:** Distinguish initial empty data, no search results, loading, unavailable data and failures. Provide a useful next action when one exists; never display failed requests as successful empty results.
- **FLOW-02:** Show pending feedback for asynchronous mutations, prevent duplicate submission, and restore usable controls after completion or failure. Success must be reflected in the relevant view; feedback need not always be a toast or modal.
- **FLOW-03:** Failures must explain recovery where possible, retain appropriate edits and allow retry when supported. If an operation's outcome is uncertain, reconcile its state before encouraging a potentially duplicate mutation.
- **FLOW-04:** Verify task entry, completion, cancel and return paths, including browser back/forward for routed views. Follow the agreed policy for unsaved edits and destructive actions; use confirmation or undo where appropriate to the risk. Do not invent autosave, persistence or cancellation support.
- **FLOW-05:** Where role restrictions apply, present unavailable actions and permission feedback consistently, without unexplained inert controls.

## Verification And Handoff

- Run the applicable checks in [development.md](development.md). Add focused regression coverage for changed behavior using the existing tooling; documentation-only edits do not require frontend runtime tests.
- For layout and interaction changes, use a real browser. Exercise the affected task, applicable states, supported locales, long/empty content and relevant viewport limits. For shared primitives, use the existing UI Lab for isolated states and verify affected page integration as needed.
- Test intermediate window widths and relevant zoom behavior when changing layout. The walkthrough plan's suggested dimensions are samples, not a new product support contract.
- Report rule IDs, tested version, browser, locale, viewport/zoom and results as **pass / fail / not applicable / not verified**. Explain exclusions and blockers; do not infer browser success from compilation or code inspection.
- Include reproduction steps and before/after evidence for visible defects; retest the original scenario and affected shared consumers after fixing them. Do not accept new screenshot baselines merely to silence failures.
- Use a short handoff for small changes; include only applicable evidence. Missing design decisions should be stated explicitly while independently verifiable work continues. Do not claim design acceptance before it occurs.
