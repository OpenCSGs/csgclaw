const MIN_PANEL_WIDTH = 420;
const MIN_CONVERSATION_WIDTH = 360;

export function clampDocumentPreviewPanelWidth(width: number, containerWidth: number) {
  const maximum = Math.max(0, containerWidth - MIN_CONVERSATION_WIDTH);
  const minimum = Math.min(MIN_PANEL_WIDTH, maximum);
  return Math.max(minimum, Math.min(width, maximum));
}
