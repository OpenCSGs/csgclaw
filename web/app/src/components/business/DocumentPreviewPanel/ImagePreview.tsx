import { useLayoutEffect, useRef, useState } from "react";

export function ImagePreview({
  name,
  objectURL,
  onError,
  scale,
}: {
  name: string;
  objectURL: string;
  onError?: () => void;
  scale: number;
}) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const [natural, setNatural] = useState({ width: 0, height: 0 });

  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const measure = () => setViewport({ width: viewport.clientWidth, height: viewport.clientHeight });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(viewport);
    return () => observer.disconnect();
  }, []);

  const fit =
    natural.width && natural.height
      ? Math.min(
          1,
          Math.max(1, viewport.width - 48) / natural.width,
          Math.max(1, viewport.height - 48) / natural.height,
        )
      : 1;

  return (
    <div ref={viewportRef} className="document-preview-image-viewport">
      <div className="document-preview-image-stage">
        <img
          className="document-preview-image"
          src={objectURL}
          alt={name}
          onError={onError}
          onLoad={(event) =>
            setNatural({ width: event.currentTarget.naturalWidth, height: event.currentTarget.naturalHeight })
          }
          style={
            natural.width ? { width: natural.width * fit * scale, height: natural.height * fit * scale } : undefined
          }
        />
      </div>
    </div>
  );
}
