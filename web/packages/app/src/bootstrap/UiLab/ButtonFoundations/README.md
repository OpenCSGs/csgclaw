# Button reference catalog

The catalog is UI Lab only. Shared Button now consumes the confirmed standard dimensions, 8px boxed radius, and inline loading text/spinner behavior. Global tokens are unchanged.

`../figmaButtons.json` records 334 nodes: standard (160), destructive (140), utility (16) and close (18). Full design contexts were retrieved for 95 nodes before the Figma MCP quota was exhausted. The remaining entries contain metadata dimensions only and render an explicit pending state. Never synthesize their visual styles from nearby variants.

Reference styles use CSS fallback values from high-fidelity design contexts, preserving per-node source links and downloaded SVGs. Some source screenshots use a different variable mode; these resolved export defaults do not establish complete light/dark semantics. Reference surfaces remain fixed while the surrounding gallery and production samples follow the Lab theme. MiSans is not bundled; local availability and fallback affect text appearance.

Reference samples compose the existing Button with local inline reference properties inside an inert region. They preserve source frame dimensions, icon slots, natural SVG dimensions, label padding, borders, backgrounds and effects. They are frozen illustrations. Production samples use unchanged Button props and require real hover/keyboard focus; loading behavior is deliberately shown as it currently works.

Asset files are imported with Vite URLs, never temporary Figma URLs. Social login and app store badges are excluded.

Known mapping gaps: xs utility size is absent from Button; utility and close families still map to generic standard icon buttons. Button font weight now uses the exported Medium 380, with MiSans preferred and synthesis disabled. Verified light text colors are mapped to dedicated button roles; exact MiSans glyphs, dark design modes and effects still require review. The 2xl size is a retained product extension. Equal labels or matching dimensions do not imply complete design acceptance.

Font revision2026-10-30: current UI Lab design previews use PingFang SC following the updated shared Figma text styles. Light/Regular/Medium/Semibold resolve to300/400/500/600. Typography node1023:36826 contains all44 updated styles; button3287:427299, input1091:61583, tag3307:417516 and toggle1102:4406 were spot-checked. Earlier MiSans notes above describe the previous capture. Current runtime copy is bilingual and reflects PingFang SC. Product font tokens and existing-code comparison mode remain unchanged. No font files are bundled or downloaded; native CSS family matching selects available weights and the typography page probes local Regular availability separately.
