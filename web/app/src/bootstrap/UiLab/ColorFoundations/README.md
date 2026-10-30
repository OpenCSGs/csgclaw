# Color foundations reference

Source: Figma file `2s3AVSZ5FfBnCR5nI2AriR`, Colors page `1023:36350`, captured 2026-10-30.

- `figmaColors.json`: Colors frame `5225:371288` (363 primitive swatches) and Color variables frame `6472:97620` (83 semantic color references after excluding base white/black).
- `figmaUtilityColors.json`: Color utility variables frame `6487:347428` (154 component color references).
- The snapshot preserves Figma variable paths and resolved values. The three unbound fills (Gray dark 25/50 and Success 200) come from the individual row design contexts: `6685:113867` and `1525:270342`.
- Some printed HEX labels and contrast ratios are stale, notably Brand; use bound variable/fill values rather than printed labels. Contrast ratios are deliberately not imported as verified accessibility results.
- Semantic references can resolve differently across nodes/modes. This export does not establish a complete light/dark alias mapping. Theme switching changes the preview shell only.
- Code matching compares a candidate primitive name to literal values in the first root declaration of production `tokens.css`. Matching values do not imply design acceptance. Missing and different values remain visible; this reference does not modify global tokens.
- Gradients (`1525:272676`) and documentation-only frames are not included in this first color reference. They contain rendered assets and need a separate asset/mode inventory before implementation.

Refresh from Figma tools, keep variable names, inspect unbound swatch fills and compare values before replacing snapshots. Do not infer missing theme aliases from matching HEX values.
