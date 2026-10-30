# UI Lab navigation

Page-private directory for the standalone UI Lab, not a Storybook dependency. The registry groups all19 sections (including the Dropdowns introduction awaiting design examples); add future sections to both this registry and the matching hidden panel in UiLab. Existing foundation IDs and selection-controls anchors remain supported.

Native hash links support deep linking and browser back/forward. useSyncExternalStore in UiLab follows hashchange; unknown or empty hashes display Colors. Panels stay mounted and hidden so local sample values survive section changes. On a changed section, focus moves to main and its scroll resets; the desktop sidebar scrolls independently. Search filters only directory links, including English/Chinese labels and group names.

Below701px the sidebar becomes a compact header with a disclosure button; choosing a destination closes the directory. The expanded directory has its own40dvh scroll limit. Theme colors use existing semantic roles. No product routes or component APIs change.

Verification2026-10-30:14 UI Lab tests, typecheck, lint, formatting and temporary production build passed. IAB at390/768/1149px, default zoom: LAY-01/03 pass for independent scrolling, collapsed/expanded compact navigation and no viewport overflow; FORM-03/FLOW-04 pass for hash navigation, browser Back, input retention and existing cross-section links. Search/current-page semantics and compact disclosure are covered by focused tests. Product locales/routes are not affected.
