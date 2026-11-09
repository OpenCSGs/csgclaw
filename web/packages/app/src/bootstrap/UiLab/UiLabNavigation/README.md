# UI Lab navigation

Page-private navigation for the standalone UI Lab, without a Storybook dependency. The registry groups 46 entries into Foundations, Components, Application components and Playgrounds. Application components include the four earlier navigation foundations and 22 categories from `ApplicationFoundations/registry.ts`. Existing foundation IDs and selection-control anchors remain supported.

Native hash links support deep linking and browser back/forward. `useSyncExternalStore` follows `hashchange`; unknown or empty hashes display Colors. `UiLabPanel` mounts content on first visit and retains it afterward, preserving local edits without rendering all unused fixtures at startup. On section changes, focus moves to main and its scroll resets; the desktop sidebar scrolls independently.

Search includes English and Chinese labels and group names. Foundation and component entry labels use the selected language. Below 701px the directory collapses into a compact disclosure; choosing a destination closes it. Product routes and shared component APIs are unchanged.
