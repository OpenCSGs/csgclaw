import { useEffect, useRef, useState, type ReactNode } from "react";
import { Check, Copy, MessageSquareReply } from "lucide-react";
import { resolveRequestPath } from "@/api/client";
import type { MessageAttachment } from "@/models/attachments";
import { flattenMentionText } from "@/components/business/MessageContent";
import type { TranslateFn } from "@/models/conversations";
import { renderSlashCommandPreviewText } from "@/models/slashCommands";
import type { VoidOrPromise } from "./types";

export type ConversationMessageActionsProps = {
  className?: string;
  leading?: ReactNode;
  content?: string | null;
  image?: MessageAttachment | null;
  onOpenThread?: () => VoidOrPromise;
  t: TranslateFn;
};

export function ConversationMessageActions({
  className = "",
  content,
  image,
  leading,
  onOpenThread,
  t,
}: ConversationMessageActionsProps) {
  const [copied, setCopied] = useState(false);
  const [copying, setCopying] = useState(false);
  const [copyError, setCopyError] = useState("");
  const copiedTimerRef = useRef<number | null>(null);
  const copyText = flattenMentionText(renderSlashCommandPreviewText(content));
  const imageURL = image?.download_url || image?.preview_url || "";
  const canCopy = Boolean(imageURL || copyText.replace(/\u200b/g, ""));
  const copyLabel = copied
    ? t("copiedToClipboard")
    : copying && imageURL
      ? t("copyingImage")
      : imageURL
        ? t("copyImage")
        : t("copyToClipboard");

  useEffect(
    () => () => {
      if (copiedTimerRef.current != null) {
        window.clearTimeout(copiedTimerRef.current);
      }
    },
    [],
  );

  async function copyMessage() {
    if (!canCopy || copying) return;
    setCopyError("");
    setCopied(false);
    setCopying(true);
    try {
      if (imageURL) {
        await writeImageToClipboard(imageURL);
      } else if (!(await writeTextToClipboard(copyText))) {
        throw new Error("copy_failed");
      }
    } catch {
      setCopyError(t(imageURL ? "copyImageFailed" : "copyTextFailed"));
      return;
    } finally {
      setCopying(false);
    }
    setCopied(true);
    if (copiedTimerRef.current != null) {
      window.clearTimeout(copiedTimerRef.current);
    }
    copiedTimerRef.current = window.setTimeout(() => {
      copiedTimerRef.current = null;
      setCopied(false);
    }, 2000);
  }

  if (!canCopy && !onOpenThread && !leading) {
    return null;
  }

  return (
    <div className={`message-action-controls ${className} ${leading ? "has-leading" : ""}`.trim()}>
      {leading ? <span className="message-action-leading">{leading}</span> : null}
      {canCopy ? (
        <button
          type="button"
          className="message-action-button copy-message-button"
          aria-label={copyLabel}
          data-tooltip={copyLabel}
          data-tooltip-side="bottom"
          disabled={copying}
          onClick={() => void copyMessage()}
        >
          {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
        </button>
      ) : null}
      {copyError ? (
        <span className="message-copy-error" role="alert">
          {copyError}
        </span>
      ) : null}
      {onOpenThread ? (
        <button
          type="button"
          className="message-action-button thread-hover-button"
          aria-label={t("replyInThread")}
          data-tooltip={t("replyInThread")}
          data-tooltip-side="bottom"
          onClick={() => void onOpenThread()}
        >
          <MessageSquareReply aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
}

async function writeTextToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    try {
      return document.execCommand("copy");
    } catch {
      return false;
    } finally {
      textarea.remove();
    }
  }
}

async function writeImageToClipboard(url: string): Promise<void> {
  if (!navigator.clipboard?.write || typeof ClipboardItem === "undefined") throw new Error("clipboard_unavailable");
  // Start the clipboard write during the click gesture. Safari requires this;
  // fetching/decoding first can lose user activation before clipboard.write.
  const item = new ClipboardItem({ "image/png": loadClipboardPNG(url) });
  await navigator.clipboard.write([item]);
}

async function loadClipboardPNG(url: string): Promise<Blob> {
  const response = await fetch(resolveRequestPath(url), { credentials: "same-origin", referrerPolicy: "no-referrer" });
  if (!response.ok) throw new Error("image_download_failed");
  const blob = await response.blob();
  if (blob.type === "image/png") return blob;
  const bitmap = await createImageBitmap(blob);
  try {
    const canvas = document.createElement("canvas");
    canvas.width = bitmap.width;
    canvas.height = bitmap.height;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("image_decode_failed");
    context.drawImage(bitmap, 0, 0);
    return await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob((png) => (png ? resolve(png) : reject(new Error("image_encode_failed"))), "image/png"),
    );
  } finally {
    bitmap.close();
  }
}
