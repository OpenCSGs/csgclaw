import { useEffect, useMemo, useRef, useState } from "react";
import { ActionCard } from "./ActionCard";
import { AgentActivityCard } from "./AgentActivityCard";
import { LongMessageCollapse } from "./LongMessageCollapse";
import { CitationSources } from "./CitationSources";
import { renderMarkdownWithCitations } from "./markdown";
import { SlashCommandCard } from "./SlashCommandCard";
import { parseSlashCommand } from "./slashCommands";
import { StructuredMessageCard } from "./StructuredMessageCard";
import { parseStructuredMessage } from "./structuredMessages";
import type { ActionCardPayload, MessageContentProps } from "./types";
import { mentionMarkupPattern, escapeHTML } from "./mentions";
import { hasAgentActivityMetadata, parseAgentActivity } from "@/models/agentActivity";
import { AgentActivityMsgTypes } from "@/shared/constants/messages";
import { prepareMermaidBlocks, renderMermaidBlocks } from "./mermaid";
import "./MessageContent.css";

export function MessageContent({
  content,
  message,
  actionBusy,
  actionFeedback,
  enableLongMessageCollapse = false,
  longMessageExpanded,
  onAction,
  onCitationSelect,
  onLongMessageExpandedChange,
  onQuestionSelect,
  t,
}: MessageContentProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [activeCitationID, setActiveCitationID] = useState<string | null>(null);
  const blankTurnPlaceholder = isBlankTurnPlaceholder(content);
  const activity = useMemo(
    () => (blankTurnPlaceholder ? null : parseAgentActivity(message ?? content)),
    [blankTurnPlaceholder, content, message],
  );
  const resolvedQuestion =
    hasAgentActivityMetadata(message) &&
    activity?.content.msgtype === AgentActivityMsgTypes.question &&
    activity.content.question?.status !== "pending";
  const slashCommand = useMemo(() => (activity ? null : parseSlashCommand(content)), [activity, content]);
  const slashCommandText = useMemo(() => renderSlashCommandText(slashCommand), [slashCommand]);
  const structured = useMemo(
    () => (activity || slashCommandText ? null : parseStructuredMessage(content)),
    [activity, content, slashCommandText],
  );
  const displayContent = String(content ?? "");
  const rendered = useMemo(
    () =>
      renderMarkdownWithCitations(displayContent, (title) => {
        const translated = t?.("citationLinkLabel", { title });
        return translated && translated !== "citationLinkLabel" ? translated : `查看引用：${title}`;
      }),
    [displayContent, t],
  );
  const markup = useMemo(
    () => ((activity && !resolvedQuestion) || slashCommandText || structured ? "" : rendered.html),
    [activity, rendered.html, resolvedQuestion, slashCommandText, structured],
  );
  const markdownHTML = useMemo(() => ({ __html: markup }), [markup]);

  useEffect(() => {
    if (enableLongMessageCollapse) {
      return undefined;
    }

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
  }, [enableLongMessageCollapse, markup]);

  if (blankTurnPlaceholder) {
    return (
      <span className="message-loading-dots" role="status" aria-label="Waiting for response">
        <span className="message-loading-dot" aria-hidden="true" />
        <span className="message-loading-dot" aria-hidden="true" />
        <span className="message-loading-dot" aria-hidden="true" />
      </span>
    );
  }

  if (activity && !resolvedQuestion) {
    return <AgentActivityCard activity={activity} onQuestionSelect={onQuestionSelect} t={t} />;
  }

  if (slashCommandText) {
    return <div className="message-content" dangerouslySetInnerHTML={{ __html: slashCommandText }} />;
  }

  if (slashCommand) {
    return <SlashCommandCard command={slashCommand} />;
  }

  if (structured) {
    if ("kind" in structured && structured.kind === "action_card") {
      return (
        <ActionCard
          data={structured as ActionCardPayload}
          message={message}
          busyKey={actionBusy}
          feedback={actionFeedback}
          onAction={onAction}
        />
      );
    }
    return <StructuredMessageCard data={structured} />;
  }

  const markdownContent =
    enableLongMessageCollapse && t ? (
      <LongMessageCollapse
        expanded={longMessageExpanded}
        html={markup}
        onExpandedChange={onLongMessageExpandedChange}
        t={t}
      />
    ) : (
      <div
        ref={rendered.cited.length ? undefined : containerRef}
        className="message-content"
        dangerouslySetInnerHTML={markdownHTML}
      />
    );

  if (!rendered.cited.length) return markdownContent;

  return (
    <div
      ref={containerRef}
      className="message-citation-shell"
      onClick={(event) => {
        const button = citationButton(event.target);
        if (!button) return;
        const activeID = button.dataset.citationId || "";
        if (!rendered.byID.has(activeID)) return;
        if (onCitationSelect) {
          onCitationSelect({ activeID, anchor: button, cited: rendered.cited });
        } else {
          setActiveCitationID(activeID);
        }
      }}
    >
      {markdownContent}
      {!onCitationSelect ? (
        <CitationSources
          activeID={activeCitationID}
          cited={rendered.cited}
          onActiveChange={setActiveCitationID}
          t={t}
        />
      ) : null}
    </div>
  );
}

function citationButton(target: EventTarget | null): HTMLButtonElement | null {
  return target instanceof Element ? target.closest<HTMLButtonElement>("button[data-citation-id]") : null;
}

function isBlankTurnPlaceholder(content: string | null | undefined): boolean {
  return typeof content === "string" && content.includes("\u200b") && content.replace(/\u200b/g, "").trim() === "";
}

function renderSlashCommandText(command: ReturnType<typeof parseSlashCommand>): string {
  if (!command) {
    return "";
  }

  let prefix = "";
  if (command.name === "use-skill") {
    prefix = `<span class="message-slash-token">/${escapeHTML(command.arg)}</span>`;
  } else if (command.name === "new" && (command.arg === "" || command.arg === "conversation")) {
    prefix = '<span class="message-slash-token">/new</span>';
  }
  if (!prefix) {
    return "";
  }
  const body = renderSlashCommandBodyMarkup(command.body);
  return body ? `${prefix} ${body}` : prefix;
}

function renderSlashCommandBodyMarkup(body: string): string {
  if (!body) {
    return "";
  }

  let result = "";
  let cursor = 0;
  for (const match of body.matchAll(mentionMarkupPattern)) {
    const index = match.index || 0;
    result += escapeHTML(body.slice(cursor, index));
    const userID = match[1] || "";
    const userName = match[2] || "";
    result += `<span class="message-mention" data-user-id="${escapeHTML(userID)}">@${escapeHTML(userName)}</span>`;
    cursor = index + match[0].length;
  }

  const safeBody = `${result}${escapeHTML(body.slice(cursor)).replace(/\n/g, "<br />")}`;
  return `<span class="slash-command-body">${safeBody}</span>`;
}
