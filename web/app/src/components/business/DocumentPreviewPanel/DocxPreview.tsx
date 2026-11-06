import documentPreviewPanelStyles from "./DocumentPreviewPanel.module.css";
import { useEffect, useState } from "react";
import DOMPurify from "dompurify";
import { copyPreviewBuffer } from "./previewBuffer";
import { prepareDocxData } from "./docxData";

export default function DocxPreview({
  data,
  scale,
  t,
}: {
  data: ArrayBuffer;
  scale: number;
  t: (key: string) => string;
}) {
  const [html, setHTML] = useState<string | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setHTML(null);
    setError(false);
    void Promise.all([import("mammoth"), prepareDocxData(copyPreviewBuffer(data))])
      .then(([mammoth, buffer]) =>
        mammoth.convertToHtml(
          { arrayBuffer: buffer },
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
    return (
      <div className={`${documentPreviewPanelStyles["document-preview-status"]} is-error`}>
        {t("attachmentPreviewFailed")}
      </div>
    );
  }
  if (html === null) {
    return <div className={documentPreviewPanelStyles["document-preview-status"]}>{t("attachmentPreviewLoading")}</div>;
  }
  return (
    <article
      className={documentPreviewPanelStyles["document-preview-docx"]}
      style={{ fontSize: `${scale}em` }}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
