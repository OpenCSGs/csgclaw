import { useEffect, useRef, useState } from "react";
import { Check, Copy, MessageSquareReply } from "lucide-react";
import { flattenMentionText } from "@/components/business/MessageContent";
import type { TranslateFn } from "@/models/conversations";
import { renderSlashCommandPreviewText } from "@/models/slashCommands";
import type { VoidOrPromise } from "./types";

export type ConversationMessageActionsProps = {
  className?: string;
  content?: string | null;
  onOpenThread?: () => VoidOrPromise;
  t: TranslateFn;
};

export function ConversationMessageActions({
  className = "",
  content,
  onOpenThread,
  t,
}: ConversationMessageActionsProps) {
  const [copied, setCopied] = useState(false);
  const copiedTimerRef = useRef<number | null>(null);
  const copyText = flattenMentionText(renderSlashCommandPreviewText(content));
  const canCopy = Boolean(copyText.replace(/\u200b/g, ""));
  const copyLabel = copied ? t("copiedToClipboard") : t("copyToClipboard");

  useEffect(
    () => () => {
      if (copiedTimerRef.current != null) {
        window.clearTimeout(copiedTimerRef.current);
      }
    },
    [],
  );

  async function copyMessage() {
    if (!canCopy || !(await writeTextToClipboard(copyText))) {
      return;
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

  if (!canCopy && !onOpenThread) {
    return null;
  }

  return (
    <div className={`message-action-controls ${className}`.trim()}>
      {canCopy ? (
        <button
          type="button"
          className="message-action-button copy-message-button"
          aria-label={copyLabel}
          data-tooltip={copyLabel}
          data-tooltip-side="bottom"
          onClick={() => void copyMessage()}
        >
          {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
        </button>
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
