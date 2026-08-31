export function copyPreviewBuffer(data: ArrayBuffer): ArrayBuffer {
  try {
    return data.slice(0);
  } catch {
    return new ArrayBuffer(0);
  }
}
