import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
} from "react";
import {
  ChevronLeft,
  ChevronRight,
  Download,
  Expand,
  Maximize2,
  Minimize2,
  Minus,
  Plus,
  RotateCcw,
  X,
} from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import { formatAttachmentSize } from "@/models/attachments";
import { DOCUMENT_PREVIEW_PANEL_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import { DocumentPreviewContent } from "./DocumentPreviewContent";
import { clampDocumentPreviewPanelWidth } from "./panelWidth";
import { documentPreviewKind, MAX_TEXT_PREVIEW_BYTES } from "./previewTypes";
import type { DocumentPreviewPanelProps } from "./types";

const DEFAULT_PANEL_WIDTH = 640;
const MIN_PANEL_WIDTH = 420;
const MIN_CONVERSATION_WIDTH = 360;

function readPanelWidth() {
  try {
    const value = Number(window.localStorage.getItem(DOCUMENT_PREVIEW_PANEL_WIDTH_STORAGE_KEY));
    return Number.isFinite(value) && value >= MIN_PANEL_WIDTH ? value : DEFAULT_PANEL_WIDTH;
  } catch {
    return DEFAULT_PANEL_WIDTH;
  }
}

export function DocumentPreviewPanel({
  anchor,
  index,
  items,
  mode = "panel",
  onClose,
  onIndexChange,
  t,
}: DocumentPreviewPanelProps) {
  const item = items[index] ?? items[0];
  const panelRef = useRef<HTMLElement | null>(null);
  const resizeRef = useRef({ pointerID: -1, startWidth: DEFAULT_PANEL_WIDTH, startX: 0 });
  const [panelWidth, setPanelWidth] = useState(readPanelWidth);
  const [containerWidth, setContainerWidth] = useState(() => window.innerWidth);
  const [resizing, setResizing] = useState(false);
  const [scale, setScale] = useState(1);
  const [data, setData] = useState<ArrayBuffer | null>(null);
  const [objectURL, setObjectURL] = useState("");
  const [previewTruncated, setPreviewTruncated] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [loadState, setLoadState] = useState<"error" | "loading" | "ready">("loading");

  useEffect(() => {
    setScale(1);
  }, [item?.id]);

  useLayoutEffect(() => {
    const updateWidth = () => {
      const measured = panelRef.current?.parentElement?.getBoundingClientRect().width ?? 0;
      const nextContainerWidth = measured > 0 ? measured : window.innerWidth;
      setContainerWidth(nextContainerWidth);
      setPanelWidth((current) => clampDocumentPreviewPanelWidth(current, nextContainerWidth));
    };
    updateWidth();
    const parent = panelRef.current?.parentElement;
    const observer = parent && typeof ResizeObserver !== "undefined" ? new ResizeObserver(updateWidth) : null;
    if (parent) {
      observer?.observe(parent);
    }
    window.addEventListener("resize", updateWidth);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updateWidth);
    };
  }, []);

  useEffect(() => {
    if (!item) {
      return undefined;
    }
    const controller = new AbortController();
    let cancelled = false;
    let nextObjectURL = "";
    setData(null);
    setObjectURL("");
    setPreviewTruncated(false);
    setLoadState("loading");
    const load = item.file
      ? Promise.resolve(item.file)
      : fetch(item.previewURL || item.downloadURL || "", {
          credentials: "same-origin",
          signal: controller.signal,
        }).then((response) => {
          if (!response.ok) {
            throw new Error(`Attachment preview failed with status ${response.status}`);
          }
          return response.blob();
        });
    void load
      .then(async (blob) => {
        const initialKind = documentPreviewKind(item);
        const limitTextBytes = initialKind === "markdown" || initialKind === "text" || initialKind === "unsupported";
        const truncated = limitTextBytes && blob.size > MAX_TEXT_PREVIEW_BYTES;
        const previewBlob = limitTextBytes ? blob.slice(0, MAX_TEXT_PREVIEW_BYTES) : blob;
        const buffer = initialKind === "image" ? new ArrayBuffer(0) : await previewBlob.arrayBuffer();
        return { blob, buffer, truncated };
      })
      .then(({ blob, buffer, truncated }) => {
        if (cancelled) {
          return;
        }
        nextObjectURL = URL.createObjectURL(blob);
        setObjectURL(nextObjectURL);
        setData(buffer);
        setPreviewTruncated(truncated);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if (!cancelled && !(error instanceof DOMException && error.name === "AbortError")) {
          setLoadState("error");
        }
      });
    return () => {
      cancelled = true;
      controller.abort();
      if (nextObjectURL) {
        URL.revokeObjectURL(nextObjectURL);
      }
    };
  }, [item]);

  useEffect(() => {
    const handleKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape" && !document.fullscreenElement) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  useEffect(() => {
    const handleFullscreenChange = () => {
      setFullscreen(document.fullscreenElement === panelRef.current);
    };
    handleFullscreenChange();
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, []);

  useEffect(() => {
    return () => anchor?.focus({ preventScroll: true });
  }, [anchor]);

  const downloadURL = useMemo(() => {
    if (item?.downloadURL) {
      return item.downloadURL;
    }
    return objectURL;
  }, [item?.downloadURL, objectURL]);

  if (!item) {
    return null;
  }

  function clampWidth(width: number) {
    return clampDocumentPreviewPanelWidth(width, containerWidth);
  }

  function finishResize() {
    setResizing(false);
    resizeRef.current.pointerID = -1;
    try {
      window.localStorage.setItem(DOCUMENT_PREVIEW_PANEL_WIDTH_STORAGE_KEY, String(Math.round(panelWidth)));
    } catch {
      // Resizing remains available when browser storage is disabled.
    }
  }

  function handleResizePointerDown(event: PointerEvent<HTMLDivElement>) {
    resizeRef.current = { pointerID: event.pointerId, startWidth: panelWidth, startX: event.clientX };
    event.currentTarget.setPointerCapture(event.pointerId);
    setResizing(true);
  }

  function handleResizePointerMove(event: PointerEvent<HTMLDivElement>) {
    if (resizeRef.current.pointerID !== event.pointerId) {
      return;
    }
    setPanelWidth(clampWidth(resizeRef.current.startWidth + resizeRef.current.startX - event.clientX));
  }

  function handleResizeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowLeft" ? 24 : -24;
    const width = clampWidth(panelWidth + delta);
    setPanelWidth(width);
    try {
      window.localStorage.setItem(DOCUMENT_PREVIEW_PANEL_WIDTH_STORAGE_KEY, String(Math.round(width)));
    } catch {
      // Keyboard resizing remains available when browser storage is disabled.
    }
  }

  async function toggleFullscreen() {
    if (document.fullscreenElement === panelRef.current) {
      await document.exitFullscreen?.();
      return;
    }
    await panelRef.current?.requestFullscreen?.();
  }

  const style = { "--document-preview-panel-width": `${panelWidth}px` } as CSSProperties;
  return (
    <aside
      ref={panelRef}
      className={`document-preview-panel is-${mode}${resizing ? " is-resizing" : ""}`}
      aria-label={t("attachmentPreview")}
      style={style}
    >
      {mode === "panel" ? (
        <div
          className="document-preview-resize-handle"
          role="separator"
          tabIndex={0}
          aria-label={t("attachmentPreviewResize")}
          aria-orientation="vertical"
          aria-valuemin={Math.min(MIN_PANEL_WIDTH, Math.max(0, containerWidth - MIN_CONVERSATION_WIDTH))}
          aria-valuemax={Math.max(0, containerWidth - MIN_CONVERSATION_WIDTH)}
          aria-valuenow={Math.round(panelWidth)}
          onKeyDown={handleResizeKeyDown}
          onPointerDown={handleResizePointerDown}
          onPointerMove={handleResizePointerMove}
          onPointerUp={finishResize}
          onPointerCancel={finishResize}
        />
      ) : null}
      <header className="document-preview-header">
        <div className="document-preview-title-group">
          <strong title={item.name}>{item.name}</strong>
          <span>{formatAttachmentSize(item.sizeBytes)}</span>
        </div>
        <div className="document-preview-toolbar">
          {items.length > 1 ? (
            <>
              <Tooltip content={t("attachmentsScrollPrevious")}>
                <Button
                  iconOnly
                  size="sm"
                  variant="tertiaryGray"
                  aria-label={t("attachmentsScrollPrevious")}
                  disabled={index <= 0}
                  onClick={() => onIndexChange(index - 1)}
                >
                  <ChevronLeft aria-hidden="true" size={16} />
                </Button>
              </Tooltip>
              <span>
                {index + 1}/{items.length}
              </span>
              <Tooltip content={t("attachmentsScrollNext")}>
                <Button
                  iconOnly
                  size="sm"
                  variant="tertiaryGray"
                  aria-label={t("attachmentsScrollNext")}
                  disabled={index >= items.length - 1}
                  onClick={() => onIndexChange(index + 1)}
                >
                  <ChevronRight aria-hidden="true" size={16} />
                </Button>
              </Tooltip>
            </>
          ) : null}
          <Tooltip content={t("attachmentPreviewZoomOut")}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("attachmentPreviewZoomOut")}
              disabled={scale <= 0.5}
              onClick={() => setScale((value) => Math.max(0.5, value - 0.1))}
            >
              <Minus aria-hidden="true" size={16} />
            </Button>
          </Tooltip>
          <span>{Math.round(scale * 100)}%</span>
          <Tooltip content={t("attachmentPreviewZoomIn")}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("attachmentPreviewZoomIn")}
              disabled={scale >= 2.5}
              onClick={() => setScale((value) => Math.min(2.5, value + 0.1))}
            >
              <Plus aria-hidden="true" size={16} />
            </Button>
          </Tooltip>
          <Tooltip content={t("attachmentPreviewResetZoom")}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("attachmentPreviewResetZoom")}
              onClick={() => setScale(1)}
            >
              <RotateCcw aria-hidden="true" size={15} />
            </Button>
          </Tooltip>
          <Tooltip content={t("attachmentPreviewFit")}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("attachmentPreviewFit")}
              onClick={() => setScale(0.85)}
            >
              <Expand aria-hidden="true" size={15} />
            </Button>
          </Tooltip>
          <Tooltip content={t(fullscreen ? "attachmentPreviewExitFullscreen" : "attachmentPreviewFullscreen")}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t(fullscreen ? "attachmentPreviewExitFullscreen" : "attachmentPreviewFullscreen")}
              onClick={() => void toggleFullscreen()}
            >
              {fullscreen ? <Minimize2 aria-hidden="true" size={15} /> : <Maximize2 aria-hidden="true" size={15} />}
            </Button>
          </Tooltip>
          {downloadURL ? (
            <Tooltip content={t("downloadAttachment")}>
              <a
                className="document-preview-icon-link"
                href={downloadURL}
                download={item.name}
                aria-label={t("downloadAttachment")}
              >
                <Download aria-hidden="true" size={16} />
              </a>
            </Tooltip>
          ) : null}
          <Tooltip content={t("close")}>
            <Button
              className="document-preview-close-button"
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("close")}
              onClick={onClose}
            >
              <X aria-hidden="true" size={17} />
            </Button>
          </Tooltip>
        </div>
      </header>
      <div className="document-preview-body">
        {loadState === "loading" ? (
          <div className="document-preview-status">{t("attachmentPreviewLoading")}</div>
        ) : null}
        {loadState === "error" ? (
          <div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>
        ) : null}
        {loadState === "ready" && data ? (
          <>
            {previewTruncated ? (
              <div className="document-preview-status is-truncated" role="status">
                {t("attachmentPreviewTruncated")}
              </div>
            ) : null}
            <DocumentPreviewContent data={data} item={item} objectURL={objectURL} scale={scale} t={t} />
          </>
        ) : null}
      </div>
    </aside>
  );
}
