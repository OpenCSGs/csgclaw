import { useCallback, useLayoutEffect, useMemo, useRef, useState, type UIEvent } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Document, Page, pdfjs } from "react-pdf";
import { Button, TextInput } from "@/components/ui";
import pdfWorkerURL from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import { copyPreviewBuffer } from "./previewBuffer";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

pdfjs.GlobalWorkerOptions.workerSrc = pdfWorkerURL;

type Translate = (key: string, params?: Record<string, string | number>) => string;

function LazyPdfPage({
  active,
  pageNumber,
  width,
  t,
}: {
  active: boolean;
  pageNumber: number;
  width: number;
  t: Translate;
}) {
  return (
    <div className="document-preview-pdf-page" data-page-number={pageNumber}>
      {active ? (
        <Page
          pageNumber={pageNumber}
          width={width}
          loading={<div className="document-preview-pdf-page-placeholder" />}
          error={<div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>}
        />
      ) : (
        <div
          className="document-preview-pdf-page-placeholder"
          aria-label={t("attachmentPreviewPageNamed", { page: pageNumber })}
          style={{ width: `${width}px` }}
        />
      )}
    </div>
  );
}

export default function PdfPreview({ data, scale, t }: { data: ArrayBuffer; scale: number; t: Translate }) {
  const scrollRootRef = useRef<HTMLDivElement | null>(null);
  const pageInputRef = useRef<HTMLInputElement | null>(null);
  const [pageCount, setPageCount] = useState(0);
  const [page, setPage] = useState(1);
  const [editingPage, setEditingPage] = useState(false);
  const [pageDraft, setPageDraft] = useState("");
  const [fittedPageWidth, setFittedPageWidth] = useState(612);
  const file = useMemo(() => ({ data: new Uint8Array(copyPreviewBuffer(data)) }), [data]);
  const handleLoadSuccess = useCallback((pdf: { numPages: number }) => {
    setPageCount(pdf.numPages);
    setPage(1);
    setEditingPage(false);
  }, []);
  const pageWidth = fittedPageWidth * scale;

  useLayoutEffect(() => {
    if (!editingPage) return;
    pageInputRef.current?.focus();
    pageInputRef.current?.select();
  }, [editingPage]);

  useLayoutEffect(() => {
    const container = scrollRootRef.current;
    if (!container) {
      return undefined;
    }
    const updateWidth = () => {
      if (container.clientWidth <= 0) {
        return;
      }
      const style = window.getComputedStyle(container);
      const horizontalPadding =
        (Number.parseFloat(style.paddingLeft) || 0) + (Number.parseFloat(style.paddingRight) || 0);
      const nextWidth = Math.max(240, Math.floor(container.clientWidth - horizontalPadding));
      setFittedPageWidth((current) => (current === nextWidth ? current : nextWidth));
    };
    updateWidth();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(updateWidth);
    observer?.observe(container);
    window.addEventListener("resize", updateWidth);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updateWidth);
    };
  }, []);

  function goToPage(nextPage: number) {
    const boundedPage = Math.min(Math.max(nextPage, 1), pageCount);
    setPage(boundedPage);
    scrollRootRef.current
      ?.querySelector<HTMLElement>(`.document-preview-pdf-page[data-page-number="${boundedPage}"]`)
      ?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function finishPageEdit(save: boolean) {
    if (save && /^\d+$/.test(pageDraft.trim())) goToPage(Number(pageDraft.trim()));
    setEditingPage(false);
  }

  function handleScroll(event: UIEvent<HTMLDivElement>) {
    const container = event.currentTarget;
    if (container.scrollTop > 0 && container.scrollHeight - container.clientHeight - container.scrollTop <= 1) {
      setPage(pageCount);
      return;
    }
    const containerTop = container.getBoundingClientRect().top;
    let closestPage = page;
    let closestDistance = Number.POSITIVE_INFINITY;
    container.querySelectorAll<HTMLElement>(".document-preview-pdf-page").forEach((pageElement) => {
      const pageNumber = Number(pageElement.dataset.pageNumber);
      const distance = Math.abs(pageElement.getBoundingClientRect().top - containerTop - 20);
      if (Number.isFinite(pageNumber) && distance < closestDistance) {
        closestDistance = distance;
        closestPage = pageNumber;
      }
    });
    setPage((current) => (current === closestPage ? current : closestPage));
  }

  return (
    <div className="document-preview-pdf">
      <div
        ref={scrollRootRef}
        className={`document-preview-pdf-main is-width-fitted${scale > 1.001 ? " allows-horizontal-pan" : ""}`}
        onScroll={handleScroll}
      >
        <Document
          file={file}
          loading={<div className="document-preview-status">{t("attachmentPreviewLoading")}</div>}
          error={<div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>}
          onLoadSuccess={handleLoadSuccess}
        >
          <div className="document-preview-pdf-pages">
            {Array.from({ length: pageCount }, (_, index) => (
              <LazyPdfPage
                key={index + 1}
                active={Math.abs(index + 1 - page) <= 1}
                pageNumber={index + 1}
                width={pageWidth}
                t={t}
              />
            ))}
          </div>
        </Document>
      </div>
      {pageCount > 0 ? (
        <div className="document-preview-page-controls">
          <Button
            iconOnly
            size="sm"
            variant="tertiaryGray"
            aria-label={t("attachmentsScrollPrevious")}
            title={t("attachmentsScrollPrevious")}
            disabled={page <= 1}
            onClick={() => goToPage(page - 1)}
          >
            <ChevronLeft aria-hidden="true" size={16} />
          </Button>
          {editingPage ? (
            <span className="document-preview-page-editor">
              <TextInput
                ref={pageInputRef}
                className="document-preview-page-input"
                size="sm"
                type="number"
                inputMode="numeric"
                min={1}
                max={pageCount}
                step={1}
                aria-label={t("attachmentPreviewPageInput")}
                value={pageDraft}
                onChange={(event) => setPageDraft(event.target.value)}
                onBlur={() => finishPageEdit(true)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    finishPageEdit(true);
                  } else if (event.key === "Escape") {
                    event.preventDefault();
                    finishPageEdit(false);
                  }
                }}
              />
              <span>{t("attachmentPreviewPageTotal", { count: pageCount })}</span>
            </span>
          ) : (
            <Button
              size="sm"
              variant="tertiaryGray"
              title={t("attachmentPreviewJumpToPage")}
              onClick={() => {
                setPageDraft(String(page));
                setEditingPage(true);
              }}
            >
              {t("attachmentPreviewPageCount", { page, count: pageCount })}
            </Button>
          )}
          <Button
            iconOnly
            size="sm"
            variant="tertiaryGray"
            aria-label={t("attachmentsScrollNext")}
            title={t("attachmentsScrollNext")}
            disabled={page >= pageCount}
            onClick={() => goToPage(page + 1)}
          >
            <ChevronRight aria-hidden="true" size={16} />
          </Button>
        </div>
      ) : null}
    </div>
  );
}
