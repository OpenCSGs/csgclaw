import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Document, Page, pdfjs } from "react-pdf";
import pdfWorkerURL from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import { copyPreviewBuffer } from "./previewBuffer";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

pdfjs.GlobalWorkerOptions.workerSrc = pdfWorkerURL;

export default function PdfPreview({
  data,
  scale,
  t,
}: {
  data: ArrayBuffer;
  scale: number;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const [pageCount, setPageCount] = useState(0);
  const [page, setPage] = useState(1);
  const file = useMemo(() => ({ data: new Uint8Array(copyPreviewBuffer(data)) }), [data]);

  return (
    <div className="document-preview-pdf">
      <div className="document-preview-pdf-main">
        <Document
          file={file}
          loading={<div className="document-preview-status">{t("attachmentPreviewLoading")}</div>}
          error={<div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>}
          onLoadSuccess={(pdf) => {
            setPageCount(pdf.numPages);
            setPage((current) => Math.min(Math.max(current, 1), pdf.numPages));
          }}
        >
          <Page pageNumber={page} scale={scale} />
        </Document>
      </div>
      {pageCount > 0 ? (
        <div className="document-preview-page-controls">
          <button
            type="button"
            aria-label={t("attachmentsScrollPrevious")}
            disabled={page <= 1}
            onClick={() => setPage((value) => value - 1)}
          >
            <ChevronLeft aria-hidden="true" size={16} />
          </button>
          <span>{t("attachmentPreviewPageCount", { page, count: pageCount })}</span>
          <button
            type="button"
            aria-label={t("attachmentsScrollNext")}
            disabled={page >= pageCount}
            onClick={() => setPage((value) => value + 1)}
          >
            <ChevronRight aria-hidden="true" size={16} />
          </button>
        </div>
      ) : null}
    </div>
  );
}
