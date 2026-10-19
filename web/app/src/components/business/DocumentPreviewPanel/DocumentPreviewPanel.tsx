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
  AtSign,
  ChevronLeft,
  ChevronRight,
  Download,
  Maximize2,
  Minimize2,
  Minus,
  Plus,
  RotateCcw,
  RefreshCw,
  X,
} from "lucide-react";
import { Button, EmptyState, LinkButton, Tooltip } from "@/components/ui";
import { formatAttachmentSize } from "@/models/attachments";
import { DOCUMENT_PREVIEW_PANEL_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import { DocumentPreviewContent } from "./DocumentPreviewContent";
import { clampDocumentPreviewPanelWidth } from "./panelWidth";
import { documentPreviewKind } from "./previewTypes";
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

function splitFileName(name: string) {
  const extensionStart = name.lastIndexOf(".");
  return extensionStart > 0 && extensionStart < name.length - 1
    ? { base: name.slice(0, extensionStart), extension: name.slice(extensionStart) }
    : { base: name, extension: "" };
}

export function DocumentPreviewPanel({
  anchor,
  index,
  items,
  mode = "panel",
  onClose,
  onReference,
  active = true,
  toolbarAccessory,
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
  const [contentType, setContentType] = useState("");
  const [data, setData] = useState<ArrayBuffer | null>(null);
  const [objectURL, setObjectURL] = useState("");
  const [fullscreen, setFullscreen] = useState(false);
  const [retry, setRetry] = useState(0);
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
    setLoadState("loading");
    const initialKind = documentPreviewKind(item);
    if (initialKind === "video" || initialKind === "audio") {
      nextObjectURL = item.file ? URL.createObjectURL(item.file) : "";
      setObjectURL(nextObjectURL || item.previewURL || item.downloadURL || "");
      setData(new ArrayBuffer(0));
      setContentType(item.mediaType);
      setLoadState("ready");
      return () => {
        if (nextObjectURL) URL.revokeObjectURL(nextObjectURL);
      };
    }
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
        const buffer = initialKind === "image" ? new ArrayBuffer(0) : await blob.arrayBuffer();
        return { blob, buffer };
      })
      .then(({ blob, buffer }) => {
        if (cancelled) {
          return;
        }
        nextObjectURL = URL.createObjectURL(blob);
        setObjectURL(nextObjectURL);
        setData(buffer);
        setContentType(blob.type);
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
  }, [item, retry]);

  useEffect(() => {
    if (mode === "embedded") return;
    const handleKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape" && !document.fullscreenElement) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose, mode]);

  useEffect(() => {
    const handleFullscreenChange = () => {
      setFullscreen(document.fullscreenElement === panelRef.current);
    };
    handleFullscreenChange();
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, []);

  useEffect(() => {
    if (mode === "embedded") return;
    return () => anchor?.focus({ preventScroll: true });
  }, [anchor, mode]);

  useEffect(() => {
    if (!active)
      panelRef.current?.querySelectorAll("video,audio").forEach((media) => (media as HTMLMediaElement).pause());
  }, [active]);

  const downloadURL = useMemo(() => {
    if (item?.downloadURL) {
      return item.downloadURL;
    }
    return objectURL;
  }, [item?.downloadURL, objectURL]);

  const fileName = splitFileName(item?.name ?? "");

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
          <strong title={item.name}>
            <span className="document-preview-title-base">{fileName.base}</span>
            {fileName.extension ? <span className="document-preview-title-extension">{fileName.extension}</span> : null}
          </strong>
          <span>{formatAttachmentSize(item.sizeBytes)}</span>
        </div>
        <div className="document-preview-toolbar">
          {toolbarAccessory}
          {onReference ? (
            <Tooltip content={t("chatFileReference")} contentProps={{ side: "bottom" }}>
              <Button
                iconOnly
                size="sm"
                variant="tertiaryGray"
                aria-label={t("chatFileReference")}
                onClick={onReference}
              >
                <AtSign size={16} />
              </Button>
            </Tooltip>
          ) : null}
          <Tooltip content={t("chatPreviewReload")} contentProps={{ side: "bottom" }}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatPreviewReload")}
              onClick={() => setRetry((value) => value + 1)}
            >
              <RefreshCw size={15} />
            </Button>
          </Tooltip>
          {items.length > 1 ? (
            <>
              <Tooltip content={t("attachmentsScrollPrevious")} contentProps={{ side: "bottom" }}>
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
              <Tooltip content={t("attachmentsScrollNext")} contentProps={{ side: "bottom" }}>
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
          <Tooltip content={t("attachmentPreviewZoomOut")} contentProps={{ side: "bottom" }}>
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
          <Tooltip content={t("attachmentPreviewZoomIn")} contentProps={{ side: "bottom" }}>
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
          <Tooltip content={t("attachmentPreviewResetZoom")} contentProps={{ side: "bottom" }}>
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
          {mode !== "embedded" ? (
            <>
              <Tooltip
                content={t(fullscreen ? "attachmentPreviewExitFullscreen" : "attachmentPreviewFullscreen")}
                contentProps={{ side: "bottom" }}
              >
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
            </>
          ) : null}
          {downloadURL ? (
            <Tooltip content={t("downloadAttachment")} contentProps={{ side: "bottom" }}>
              <LinkButton
                className="document-preview-icon-link"
                href={downloadURL}
                download={item.name}
                aria-label={t("downloadAttachment")}
                size="sm"
                variant="tertiaryGray"
              >
                <Download aria-hidden="true" size={16} />
              </LinkButton>
            </Tooltip>
          ) : null}
          <Tooltip content={t("close")} contentProps={{ side: "bottom" }}>
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
          <EmptyState
            className="document-preview-status is-error document-preview-error-state"
            title={t("attachmentPreviewErrorTitle")}
            description={t("attachmentPreviewFailed")}
            actions={
              <div className="document-preview-error-actions">
                <Button size="sm" variant="secondaryGray" onClick={() => setRetry((value) => value + 1)}>
                  {t("chatPreviewReload")}
                </Button>
              </div>
            }
          />
        ) : null}
        {loadState === "ready" && data ? (
          <>
            <DocumentPreviewContent
              onError={() => setLoadState("error")}
              contentType={contentType}
              data={data}
              item={item}
              objectURL={objectURL}
              scale={scale}
              t={t}
            />
          </>
        ) : null}
      </div>
    </aside>
  );
}
