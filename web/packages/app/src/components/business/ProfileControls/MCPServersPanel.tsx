import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import profileControlsStyles from "./ProfileControls.module.css";
import { Button, Field, TextArea } from "@/components/ui";
import { useEffect, useState } from "react";
import {
  MCP_SERVERS_EXAMPLE,
  mcpServersText,
  parseMCPServersText,
  setMCPServers,
  type AgentDraft,
  type JSONRecord,
} from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";

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
    <Field
      className={`span-2 ${profileFormLayoutStyles["span-2"]} ${profileControlsStyles["mcp-servers-panel"]}`}
      label={t("profileMCPServers")}
      hint={error ? undefined : t("profileMCPServersHint")}
      error={error}
      labelAction={
        <div className={profileControlsStyles["mcp-servers-actions"]}>
          <Button variant="secondaryGray" size="sm" onClick={fillExample}>
            {t("profileMCPServersUseExample")}
          </Button>
          <Button variant="secondaryGray" size="sm" onClick={clearMCPServers}>
            {t("profileMCPServersClear")}
          </Button>
        </div>
      }
    >
      <TextArea
        className={`compact-textarea ${profileControlsStyles["mcp-servers-textarea"]}`}
        value={text}
        placeholder={t("profileMCPServersPlaceholder")}
        spellCheck={false}
        onInput={(event) => commitText(event.currentTarget.value)}
      />
    </Field>
  );
}
