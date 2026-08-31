import type { AttachmentPreviewItem } from "@/models/attachments";

export type DocumentPreviewRequest = {
  anchor?: HTMLElement | null;
  index: number;
  items: AttachmentPreviewItem[];
};

export type DocumentPreviewPanelProps = DocumentPreviewRequest & {
  mode?: "overlay" | "panel";
  onClose: () => void;
  onIndexChange: (index: number) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
};

export type PreviewKind = "docx" | "image" | "markdown" | "pdf" | "powerpoint" | "spreadsheet" | "text" | "unsupported";
