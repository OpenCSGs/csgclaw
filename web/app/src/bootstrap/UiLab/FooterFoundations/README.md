# Section footers

Implements all eight variants from Figma page `3275:371793` as interactive React components, using the shared Button and Radix ToggleGroup. The earlier draft form was removed. This package is UI Lab only.

| Type    | Breakpoint | Width | Height | Divider gap | Horizontal padding | Bottom padding |
| ------- | ---------- | ----- | ------ | ----------- | ------------------ | -------------- |
| Section | Desktop    | 1216  | 61     | 20          | 0                  | 0              |
| Section | Mobile     | 688   | 57     | 16          | 0                  | 0              |
| Card    | Desktop    | 1216  | 73     | 16          | 24                 | 16             |
| Card    | Mobile     | 688   | 65     | 12          | 16                 | 12             |

Each row has both grouped and ungrouped variants. All dimensions are pixels. `FooterFoundations.tsx` records the eight original node IDs and links. Gallery samples retain source widths and scroll locally. The playground wraps below 688px; this narrower adaptation extends beyond the supplied Mobile frame.

Typography: PingFang SC 500, 14px/20px. Buttons: 40px high, 8px radius, 12px action gaps. Tertiary/link text #535862, secondary text #414651, primary fill #3250bd with white text. Card content gap is 16px; Section content gap is 20px. The 20px plus and both 1px divider assets are unchanged Figma downloads, imported through Vite. Their intrinsic SVG dimensions remain intact.

Range selection uses a single-select roving keyboard group. Optional divider, group, learn-more and action controls change the playground only. All action feedback stays outside the design frame so it does not change sample height. No server writes are performed.

For verification, run the focused footer/navigation/localization tests and inspect the UI Lab playground in Chinese and English at desktop and narrow widths. Check the eight gallery frames separately from the adaptive playground; the gallery intentionally preserves the original Figma dimensions.
