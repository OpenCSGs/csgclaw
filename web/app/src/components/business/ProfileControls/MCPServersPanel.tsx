import { useEffect, useId, useState } from "react";
import {
  MCP_SERVERS_EXAMPLE,
  mcpServersText,
  parseMCPServersText,
  setMCPServers,
  type AgentDraft,
  type JSONRecord,
} from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { Button } from "@/components/ui/Button/Button";

export type MCPServersPanelProps = {
  draft: AgentDraft;
  onDraftChange: (draft: AgentDraft) => void;
  onInvalidChange?: (invalid: boolean) => void;
  t: TranslateFn;
};

function cloneMCPServersExample(): JSONRecord {
  return JSON.parse(JSON.stringify(MCP_SERVERS_EXAMPLE)) as JSONRecord;
}

function errorMessageForKey(key: "invalid_json" | "object_required", t: TranslateFn): string {
  return key === "invalid_json" ? t("profileMCPServersInvalidJSON") : t("profileMCPServersObjectRequired");
}

export function MCPServersPanel({ draft, onDraftChange, onInvalidChange, t }: MCPServersPanelProps) {
  const textareaId = useId();
  const draftText = mcpServersText(draft.mcpServers);
  const [text, setText] = useState(draftText);
  const [error, setError] = useState("");

  useEffect(() => {
    setText(draftText);
    setError("");
    onInvalidChange?.(false);
  }, [draftText, onInvalidChange]);

  function commitText(nextText: string) {
    setText(nextText);
    const parsed = parseMCPServersText(nextText);
    if (!parsed.ok) {
      setError(errorMessageForKey(parsed.error, t));
      onInvalidChange?.(true);
      return;
    }
    setError("");
    onInvalidChange?.(false);
    onDraftChange({
      ...draft,
      mcpServers: setMCPServers(parsed.value),
    });
  }

  function fillExample() {
    const example = cloneMCPServersExample();
    setError("");
    onInvalidChange?.(false);
    setText(JSON.stringify(example, null, 2));
    onDraftChange({
      ...draft,
      mcpServers: setMCPServers(example),
    });
  }

  function clearMCPServers() {
    setError("");
    onInvalidChange?.(false);
    setText("");
    onDraftChange({
      ...draft,
      mcpServers: setMCPServers(null),
    });
  }

  return (
    <div className="field span-2 mcp-servers-panel">
      <div className="mcp-servers-header">
        <label htmlFor={textareaId}>{t("profileMCPServers")}</label>
        <div className="mcp-servers-actions">
          <Button variant="secondaryGray" size="sm" onClick={fillExample}>
            {t("profileMCPServersUseExample")}
          </Button>
          <Button variant="secondaryGray" size="sm" onClick={clearMCPServers}>
            {t("profileMCPServersClear")}
          </Button>
        </div>
      </div>
      <textarea
        id={textareaId}
        className="compact-textarea mcp-servers-textarea"
        value={text}
        aria-invalid={error ? "true" : undefined}
        aria-describedby={`${textareaId}-hint`}
        placeholder={t("profileMCPServersPlaceholder")}
        spellCheck={false}
        onInput={(event) => commitText(event.currentTarget.value)}
      />
      <span id={`${textareaId}-hint`} className={`field-hint${error ? " error" : ""}`.trim()}>
        {error || t("profileMCPServersHint")}
      </span>
    </div>
  );
}
