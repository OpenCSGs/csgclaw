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
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setHTML("");
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
          setHTML(DOMPurify.sanitize(result.value));
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
    <article
      className="document-preview-docx"
      style={{ fontSize: `${scale}em` }}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
