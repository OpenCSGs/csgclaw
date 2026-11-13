import { useEffect, useState } from "react";
import { Button } from "@/components/ui";
import type { AttachmentPreviewItem } from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";
import { createUUID } from "@/shared/lib/uuid";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";
import { formatPreviewText } from "@/components/business/DocumentPreviewPanel";
import { BrowserTab } from "./BrowserTab";
import styles from "./ChatSidePanel.module.css";

export function HTMLBrowserTab({
  item,
  sourceKey,
  active,
  onPopup,
  t,
}: {
  item: AttachmentPreviewItem;
  sourceKey: string;
  active: boolean;
  onPopup: (url: string) => void;
  t: TranslateFn;
}) {
  const [url, setURL] = useState("");
  const [sourceURL, setSourceURL] = useState("");
  const [error, setError] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    const bridge = getDesktopBridge();
    const id = createUUID();
    let disposed = false;
    let objectURL = "";
    setURL("");
    setError(false);
    async function load() {
      const file =
        item.file ??
        (await fetch(item.previewURL || item.downloadURL || "", {
          credentials: "same-origin",
          signal: controller.signal,
        }).then((response) => {
          if (!response.ok) throw new Error(`HTML preview failed with status ${response.status}`);
          return response.blob();
        }));
      if (!file) throw new Error("HTML preview content is unavailable.");
      const text = formatPreviewText(await file.arrayBuffer(), item.mediaType, { contentType: file.type });
      if (disposed) return;
      const next = bridge
        ? (await bridge.browserCommand({ id, action: "previewHTML", html: text, sourceKey }))?.url
        : (objectURL = URL.createObjectURL(new Blob([text], { type: "text/html;charset=utf-8" })));
      if (disposed) {
        if (bridge) await bridge.browserCommand({ id, action: "releaseHTML" });
        return;
      }
      if (!next) throw new Error("HTML preview URL is unavailable.");
      setSourceURL(next);
      setURL(next);
    }
    void load().catch(() => {
      if (!disposed) setError(true);
    });
    return () => {
      disposed = true;
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
      if (bridge) void bridge.browserCommand({ id, action: "releaseHTML" }).catch(() => {});
    };
  }, [item, revision, sourceKey]);

  return url ? (
    <BrowserTab
      key={revision}
      url={url}
      active={active}
      onNavigate={setURL}
      onPopup={onPopup}
      onOpenAddress={onPopup}
      onReload={url === sourceURL ? () => setRevision((value) => value + 1) : undefined}
      t={t}
    />
  ) : (
    <div className={styles["chat-side-status"]} role={error ? "alert" : "status"}>
      {t(error ? "attachmentPreviewFailed" : "attachmentPreviewLoading")}
      {error ? <Button onClick={() => setRevision((value) => value + 1)}>{t("retry")}</Button> : null}
    </div>
  );
}
