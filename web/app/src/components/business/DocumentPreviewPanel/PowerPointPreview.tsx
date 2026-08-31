import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { copyPreviewBuffer } from "./previewBuffer";

type PPTXPreviewer = {
  destroy: () => void;
  preview: (data: ArrayBuffer) => Promise<unknown>;
  renderSingleSlide: (index: number) => void;
  slideCount: number;
};

export default function PowerPointPreview({
  data,
  scale,
  t,
}: {
  data: ArrayBuffer;
  scale: number;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const stageRef = useRef<HTMLDivElement | null>(null);
  const mainPreviewerRef = useRef<PPTXPreviewer | null>(null);
  const [slideCount, setSlideCount] = useState(0);
  const [slide, setSlide] = useState(1);
  const [error, setError] = useState(false);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) {
      return undefined;
    }
    let cancelled = false;
    let mainPreviewer: PPTXPreviewer | null = null;
    stage.replaceChildren();
    setError(false);
    void import("pptx-preview")
      .then(({ init }) => {
        mainPreviewer = init(stage, { height: 540, mode: "slide", width: 960 });
        mainPreviewerRef.current = mainPreviewer;
        return mainPreviewer.preview(copyPreviewBuffer(data));
      })
      .then(() => {
        if (cancelled) {
          return;
        }
        if (!mainPreviewer) {
          throw new Error("PowerPoint preview is unavailable");
        }
        setSlideCount(mainPreviewer.slideCount);
        mainPreviewer.renderSingleSlide(0);
      })
      .catch(() => {
        if (!cancelled) {
          setError(true);
        }
      });
    return () => {
      cancelled = true;
      mainPreviewer?.destroy();
      mainPreviewerRef.current = null;
      stage.replaceChildren();
    };
  }, [data]);

  useEffect(() => {
    mainPreviewerRef.current?.renderSingleSlide(slide - 1);
  }, [slide]);

  if (error) {
    return <div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>;
  }
  return (
    <div className="document-preview-powerpoint">
      <div className="document-preview-ppt-stage-shell">
        <div ref={stageRef} className="document-preview-ppt-stage" style={{ transform: `scale(${scale})` }} />
      </div>
      {slideCount > 0 ? (
        <div className="document-preview-page-controls">
          <button
            type="button"
            aria-label={t("attachmentsScrollPrevious")}
            disabled={slide <= 1}
            onClick={() => setSlide((value) => value - 1)}
          >
            <ChevronLeft aria-hidden="true" size={16} />
          </button>
          <span>{t("attachmentPreviewSlideCount", { page: slide, count: slideCount })}</span>
          <button
            type="button"
            aria-label={t("attachmentsScrollNext")}
            disabled={slide >= slideCount}
            onClick={() => setSlide((value) => value + 1)}
          >
            <ChevronRight aria-hidden="true" size={16} />
          </button>
        </div>
      ) : null}
    </div>
  );
}
