import { useEffect, useMemo, useRef } from "react";
import { ActionCard } from "./ActionCard";
import { renderMarkdown } from "./markdown";
import { prepareMermaidBlocks, renderMermaidBlocks } from "./mermaid";
import { StructuredMessageCard } from "./StructuredMessageCard";
import { parseStructuredMessage } from "./structuredMessages";
import type { ActionCardPayload, MessageContentProps } from "./types";
import "./MessageContent.css";

export function MessageContent({ content, message, actionBusy, actionError, onAction }: MessageContentProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const structured = useMemo(() => parseStructuredMessage(content), [content]);
  const markup = useMemo(() => (structured ? "" : renderMarkdown(content)), [content, structured]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return undefined;
    }

    const diagrams = prepareMermaidBlocks(container);
    let cancelled = false;
    renderMermaidBlocks(diagrams)?.catch((error) => {
      if (!cancelled) {
        console.warn("Failed to render Mermaid diagram", error);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [markup]);

  if (structured) {
    if ("kind" in structured && structured.kind === "action_card") {
      return (
        <ActionCard
          data={structured as ActionCardPayload}
          message={message}
          busyKey={actionBusy}
          error={actionError}
          onAction={onAction}
        />
      );
    }
    return <StructuredMessageCard data={structured} />;
  }

  return <div ref={containerRef} className="message-content" dangerouslySetInnerHTML={{ __html: markup }} />;
}
