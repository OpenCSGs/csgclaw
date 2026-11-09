# Layout foundations

UI Lab reference only. Values are a Figma snapshot captured on 2026-10-30; source node IDs are recorded in `../figmaLayout.json`. Keep these reference values isolated from production tokens.

The comparison reads literal pixel values from the current CSS token source. Equal-value candidates do not imply semantic equivalence. Radius xs/sm have naming conflicts, and the 6px spacing primitive has no space-1-5 token.

Grid presets use the source column layoutGrids, including Desktop's 112px offset (80px outer margin plus 32px container padding). Tablet has six columns, not eight. Presets are reference frame widths, not application breakpoints. Row grids and supplementary margin overlays are excluded. Preview uses CSS pixels with local scrolling and thematic illustrative colors.
