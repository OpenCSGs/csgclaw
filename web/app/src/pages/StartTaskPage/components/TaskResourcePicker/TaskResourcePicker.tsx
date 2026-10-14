import { useState } from "react";
import { X, Search } from "lucide-react";
import { Button } from "@/components/ui";
import { useAgentResourceLists } from "@/hooks/workspace/useAgentResourceLists";
import { mcpManagedKnowledgeBaseSource } from "@/models/mcp";
import type { TranslateFn } from "@/models/conversations";
import type { TaskResourceKind } from "@/components/business/ConversationPane/ComposerAddMenu";
import styles from "./TaskResourcePicker.module.css";
export function TaskResourcePicker({
  agentID,
  kind,
  t,
  onClose,
  onPick,
  onManage,
}: {
  agentID: string;
  kind: TaskResourceKind;
  t: TranslateFn;
  onClose: () => void;
  onPick: (names: string[]) => void;
  onManage: () => void;
}) {
  const lists = useAgentResourceLists(agentID);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const resource = kind === "skill" ? lists.skills : lists.mcp;
  const items =
    kind === "skill"
      ? lists.skills.items.filter((item) => item.enabled !== false)
      : lists.mcp.items.filter(
          (item) =>
            item.config.disabled !== true &&
            (kind === "knowledge"
              ? Boolean(mcpManagedKnowledgeBaseSource(item.config))
              : !mcpManagedKnowledgeBaseSource(item.config)),
        );
  const label = t(
    kind === "skill"
      ? "composerResourceSkill"
      : kind === "connector"
        ? "composerResourceConnector"
        : "composerResourceKnowledge",
  );
  return (
    <aside
      className={styles.picker}
      role="dialog"
      aria-label={label}
      onKeyDown={(event) => {
        if (event.key === "Escape") onClose();
      }}
    >
      <header>
        <strong>{label}</strong>
        <Button variant="tertiaryGray" size="sm" aria-label={t("close")} onClick={onClose}>
          <X size={16} />
        </Button>
      </header>
      <label className={styles.search}>
        <Search size={14} />
        <input
          autoFocus
          aria-label={t("search")}
          placeholder={t("search")}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>
      <div className={styles.list}>
        {items
          .filter((item) => item.name.toLowerCase().includes(query.toLowerCase()))
          .map((item) => (
            <label key={item.name}>
              <input
                type="checkbox"
                checked={selected.includes(item.name)}
                onChange={(event) =>
                  setSelected((value) =>
                    event.target.checked ? [...value, item.name] : value.filter((name) => name !== item.name),
                  )
                }
              />
              <span>
                {item.name}
                <small>{item.description}</small>
              </span>
            </label>
          ))}
        {resource.continuation.loading ? <p>{t("loading")}</p> : null}
        {resource.continuation.failed ? <Button onClick={resource.continuation.retry}>{t("retry")}</Button> : null}
        {resource.continuation.hasMore ? (
          <Button variant="tertiaryGray" size="sm" onClick={() => void resource.continuation.loadMore()}>
            {t("taskEntryLoadMore")}
          </Button>
        ) : null}
        {!resource.continuation.loading && !items.length ? <p>{t("iaListSearchEmpty")}</p> : null}
      </div>
      <footer>
        <Button variant="tertiaryGray" size="sm" onClick={onManage}>
          {t("taskRuntimeEditAgent")}
        </Button>
        <Button size="sm" disabled={!selected.length} onClick={() => onPick(selected)}>
          {t("addAttachmentResource")}
        </Button>
      </footer>
    </aside>
  );
}
