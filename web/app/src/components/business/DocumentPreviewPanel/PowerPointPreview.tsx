import { useCallback, useEffect, useRef, useState, type UIEvent } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { copyPreviewBuffer } from "./previewBuffer";

type PPTXPreviewer = {
  destroy: () => void;
  preview: (data: ArrayBuffer) => Promise<unknown>;
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
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const stageRef = useRef<HTMLDivElement | null>(null);
  const [slideCount, setSlideCount] = useState(0);
  const [slide, setSlide] = useState(1);
  const [error, setError] = useState(false);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) {
      return undefined;
    }
    let cancelled = false;
    let previewer: PPTXPreviewer | null = null;
    stage.replaceChildren();
    setError(false);
    setSlide(1);
    setSlideCount(0);
    void import("pptx-preview")
      .then(({ init }) => {
        previewer = init(stage, { mode: "list", width: 960 });
        return previewer.preview(copyPreviewBuffer(data));
      })
      .then(() => {
        if (cancelled) {
          return;
        }
        if (!previewer) {
          throw new Error("PowerPoint preview is unavailable");
        }
        setSlideCount(previewer.slideCount);
        setSlide(1);
      })
      .catch(() => {
        if (!cancelled) {
          setError(true);
        }
      });
    return () => {
      cancelled = true;
      previewer?.destroy();
      stage.replaceChildren();
    };
  }, [data]);

  const goToSlide = useCallback(
    (nextSlide: number) => {
      const boundedSlide = Math.min(Math.max(nextSlide, 1), slideCount);
      setSlide(boundedSlide);
      stageRef.current
        ?.querySelector<HTMLElement>(`.pptx-preview-slide-wrapper-${boundedSlide - 1}`)
        ?.scrollIntoView?.({ behavior: "smooth", block: "start" });
    },
    [slideCount],
  );

  function handleScroll(event: UIEvent<HTMLDivElement>) {
    const container = event.currentTarget;
    const containerTop = container.getBoundingClientRect().top;
    let closestSlide = slide;
    let closestDistance = Number.POSITIVE_INFINITY;
    container.querySelectorAll<HTMLElement>(".pptx-preview-slide-wrapper").forEach((slideElement, index) => {
      const distance = Math.abs(slideElement.getBoundingClientRect().top - containerTop - 20);
      if (distance < closestDistance) {
        closestDistance = distance;
        closestSlide = index + 1;
      }
    });
    setSlide((current) => (current === closestSlide ? current : closestSlide));
  }

  if (error) {
    return <div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>;
  }
  return (
    <div className="document-preview-powerpoint">
      <div ref={scrollRef} className="document-preview-ppt-stage-shell" onScroll={handleScroll}>
        <div ref={stageRef} className="document-preview-ppt-stage" style={{ transform: `scale(${scale})` }} />
      </div>
      {slideCount > 0 ? (
        <div className="document-preview-page-controls">
          <button
            type="button"
            aria-label={t("attachmentsScrollPrevious")}
            disabled={slide <= 1}
            onClick={() => goToSlide(slide - 1)}
          >
            <ChevronLeft aria-hidden="true" size={16} />
          </button>
          <span>{t("attachmentPreviewSlideCount", { page: slide, count: slideCount })}</span>
          <button
            type="button"
            aria-label={t("attachmentsScrollNext")}
            disabled={slide >= slideCount}
            onClick={() => goToSlide(slide + 1)}
          >
            <ChevronRight aria-hidden="true" size={16} />
          </button>
        </div>
      ) : null}
    </div>
  );
}
