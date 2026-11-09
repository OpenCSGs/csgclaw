const FILE_SIZE_UNITS = ["B", "KB", "MB", "GB", "TB", "PB", "EB"] as const;

// Use decimal units (1 KB = 1,000 B), with at most one decimal place.
export function formatFileSize(sizeBytes: number | null | undefined): string {
  let size = typeof sizeBytes === "number" && Number.isFinite(sizeBytes) ? Math.max(0, Math.floor(sizeBytes)) : 0;
  let unit = 0;
  while (size >= 1000 && unit < FILE_SIZE_UNITS.length - 1) {
    size /= 1000;
    unit++;
  }
  size = Math.round(size * 10) / 10;
  // Rounding must not leave a value such as "1000 KB" at a unit boundary.
  if (size >= 1000 && unit < FILE_SIZE_UNITS.length - 1) {
    size /= 1000;
    unit++;
  }
  return `${size} ${FILE_SIZE_UNITS[unit]}`;
}
