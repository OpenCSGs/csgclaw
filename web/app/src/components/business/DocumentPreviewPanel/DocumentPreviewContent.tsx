import { lazy, Suspense, useMemo } from "react";
import { FileText } from "lucide-react";
import type { AttachmentPreviewItem } from "@/models/attachments";
import { MarkdownPreview } from "./MarkdownPreview";
import { documentPreviewKind, formatPreviewText } from "./previewTypes";
import { syntaxLanguageForFile } from "./syntaxLanguages";

const DocxPreview = lazy(() => import("./DocxPreview"));
const PdfPreview = lazy(() => import("./PdfPreview"));
const PowerPointPreview = lazy(() => import("./PowerPointPreview"));
const SpreadsheetPreview = lazy(() => import("./SpreadsheetPreview"));
const SyntaxHighlightedText = lazy(() =>
  import("./SyntaxHighlightedText").then((module) => ({ default: module.SyntaxHighlightedText })),
);

export function DocumentPreviewContent({
  data,
  item,
  objectURL,
  scale,
  t,
}: {
  data: ArrayBuffer;
  item: AttachmentPreviewItem;
  objectURL: string;
  scale: number;
  t: (key: string) => string;
}) {
  const kind = documentPreviewKind(item, data);
  const text = useMemo(
    () => (kind === "markdown" || kind === "text" ? formatPreviewText(data, item.mediaType) : ""),
    [data, item.mediaType, kind],
  );
  const syntaxLanguage = useMemo(
    () => (kind === "text" ? syntaxLanguageForFile(item.name, item.mediaType) : null),
    [item.mediaType, item.name, kind],
  );
  const loading = <div className="document-preview-status">{t("attachmentPreviewLoading")}</div>;

  if (kind === "image") {
    return (
      <div className="document-preview-image-stage">
        <img
          className="document-preview-image"
          src={objectURL}
          alt={item.name}
          style={{ transform: `scale(${scale})` }}
        />
      </div>
    );
  }
  if (kind === "markdown") {
    return <MarkdownPreview scale={scale} text={text} t={t} />;
  }
  if (kind === "text") {
    if (syntaxLanguage) {
      return (
        <Suspense fallback={loading}>
          <SyntaxHighlightedText language={syntaxLanguage} scale={scale} text={text} t={t} />
        </Suspense>
      );
    }
    return (
      <pre className="document-preview-text" style={{ fontSize: `${scale}em` }}>
        {text}
      </pre>
    );
  }
  if (kind === "pdf") {
    return (
      <Suspense fallback={loading}>
        <PdfPreview data={data} scale={scale} t={t} />
      </Suspense>
    );
  }
  if (kind === "docx") {
    return (
      <Suspense fallback={loading}>
        <DocxPreview data={data} scale={scale} t={t} />
      </Suspense>
    );
  }
  if (kind === "spreadsheet") {
    return (
      <Suspense fallback={loading}>
        <SpreadsheetPreview data={data} scale={scale} t={t} />
      </Suspense>
    );
  }
  if (kind === "powerpoint") {
    return (
      <Suspense fallback={loading}>
        <PowerPointPreview data={data} scale={scale} t={t} />
      </Suspense>
    );
  }
  return (
    <div className="document-preview-unsupported">
      <FileText aria-hidden="true" size={40} />
      <p>{t("attachmentPreviewUnavailable")}</p>
    </div>
  );
}
