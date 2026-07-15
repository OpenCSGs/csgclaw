export function filesFromDataTransfer(dataTransfer: DataTransfer | null | undefined): File[] {
  if (!dataTransfer) {
    return [];
  }
  const files = Array.from(dataTransfer.files || []).filter((file) => file.size > 0);
  if (files.length > 0) {
    return files;
  }
  return Array.from(dataTransfer.items || [])
    .filter((item) => item.kind === "file")
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file && file.size > 0));
}
