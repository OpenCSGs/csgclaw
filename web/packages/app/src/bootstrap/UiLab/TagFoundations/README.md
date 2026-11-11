# Tag foundations

UI Lab reference only; no business tag replacement or shared Tag API is introduced.

The source page and each of its 72 variant nodes are recorded in `../figmaTags.json`. High-fidelity design contexts were retrieved for 69 variants before the MCP quota was exhausted. Three lg avatar action variants are metadata-only and explicitly marked pending. Do not infer those missing styles.

Reference frames retain source dimensions and unselected checkboxes; the interactive sample adapts to the label and truncates long text while retaining its full title and accessible text. Icons, the AU flag and avatar are downloaded original assets. SVGs retain their root dimensions; the avatar raster is cropped into the source 16px circular slot.

Light export colors remain fixed within white reference stages; the Lab shell follows the selected theme. Typography is MiSans Regular 330 with system fallback when the font is unavailable. Count size and layout follow the sm/md/lg helpers. Figma inside strokes are compensated in the fixed frame padding.

Checkbox and remove actions compose existing Checkbox and Button primitives. Their interactive checked, hover and focus rendering remains the current library behavior, not a claim of parity with unread Figma subcomponent states. Removal is local to the sample and reversible with Restore.

Font revision2026-10-30: current UI Lab design previews use PingFang SC following the updated shared Figma text styles. Light/Regular/Medium/Semibold resolve to300/400/500/600. Typography node1023:36826 contains all44 updated styles; button3287:427299, input1091:61583, tag3307:417516 and toggle1102:4406 were spot-checked. Earlier MiSans notes above describe the previous capture. Current runtime copy is bilingual and reflects PingFang SC. Product font tokens and existing-code comparison mode remain unchanged. No font files are bundled or downloaded; native CSS family matching selects available weights and the typography page probes local Regular availability separately.
