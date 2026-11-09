# Shadow foundations

Source: Figma file `2s3AVSZ5FfBnCR5nI2AriR`, Effects page `1030:33573`, captured 2026-10-30.

`figmaShadows.json` records seven elevation levels from `5253:373440`, two focus rings from `5424:398994`, three ring/shadow combinations from `5424:399464`, and two skeuomorphic combinations from `7396:260015`. Design contexts and variable definitions were read together. CSS preserves generated layer ordering; inner shadows use inset layers. Colors use resolved eight-digit HEX values from effect variables rather than rounded alpha fallback values.

Samples retain the reference white surface, 12px radius and design border. Theme controls change only the surrounding UI Lab. Production mode applies the existing same-name token and explicitly labels absent tokens; no global token is created or changed. A ring specimen is a static visual example, not a focusable application control or a replacement for keyboard testing of actual components.

The focus-ring-shadow-sm design uses zero spread on its small lower layer, while the standalone shadow-sm uses -1px. Keep this distinction. Avoid promoting the snapshot to theme-aware global aliases until design review. Blurs and long documentation frames are outside this shadow reference.
