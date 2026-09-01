import { useEffect, useState } from "react";
import DOMPurify from "dompurify";
import { copyPreviewBuffer } from "./previewBuffer";

export default function DocxPreview({
  data,
  scale,
  t,
}: {
  data: ArrayBuffer;
  scale: number;
  t: (key: string) => string;
}) {
  const [html, setHTML] = useState("");
  const [headings, setHeadings] = useState<string[]>([]);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setHTML("");
    setHeadings([]);
    setError(false);
    void import("mammoth")
      .then((mammoth) =>
        mammoth.convertToHtml(
          { arrayBuffer: copyPreviewBuffer(data) },
          {
            convertImage: mammoth.images.imgElement(async (image) => ({
              src: `data:${image.contentType};base64,${await image.read("base64")}`,
            })),
          },
        ),
      )
      .then((result) => {
        if (!cancelled) {
          const sanitized = DOMPurify.sanitize(result.value);
          const document = new DOMParser().parseFromString(sanitized, "text/html");
          setHeadings(
            Array.from(document.querySelectorAll("h1, h2, h3, h4, h5, h6"))
              .map((heading) => heading.textContent?.trim() || "")
              .filter(Boolean),
          );
          setHTML(sanitized);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setError(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [data]);

  if (error) {
    return <div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>;
  }
  if (!html) {
    return <div className="document-preview-status">{t("attachmentPreviewLoading")}</div>;
  }
  return (
    <div className={`document-preview-docx-shell ${headings.length > 0 ? "has-outline" : "without-outline"}`}>
      {headings.length > 0 ? (
        <aside className="document-preview-docx-outline" aria-label={t("attachmentPreviewOutline")}>
          {headings.map((heading, index) => (
            <span key={`${heading}-${index}`}>{heading}</span>
          ))}
        </aside>
      ) : null}
      <article
        className="document-preview-docx"
        style={{ fontSize: `${scale}em` }}
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}
