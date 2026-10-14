import { isComposerKeyboardEventComposing } from "@/models/composer";
import { useState } from "react";
import { X, Search } from "lucide-react";
import { Button } from "@/components/ui";
import { useConversationResources } from "@/hooks/workspace/useConversationResources";
import { maxResourceReferences, resourceKey, type ResourceSnapshot } from "@/models/conversationResources";
import { Link } from "react-router-dom";
import { pathForPane, WorkspacePaneTypes } from "@/models/routing";
import { localizeAPIError } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";
import type { TaskResourceKind } from "@/components/business/ConversationPane/ComposerAddMenu";
import styles from "./ConversationResourcePicker.module.css";
export function ConversationResourcePicker({
  agentID,
  kind,
  t,
  onClose,
  onPick,
  onManage,
  existing = [],
}: {
  agentID: string;
  kind: TaskResourceKind;
  t: TranslateFn;
  onClose: () => void;
  onPick: (resources: ResourceSnapshot[]) => void;
  existing?: readonly ResourceSnapshot[];
  onManage?: () => void;
}) {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<ResourceSnapshot[]>([]);
  const resource = useConversationResources(agentID, kind === "knowledge" ? "knowledge_base" : kind, query);
  const items = resource.items;
  const unavailableLabels: Record<string, string> = {
    disabled: "errors.resource_disabled",
    connecting: "resourceConnecting",
    disconnected: "resourceDisconnected",
    authorization_required: "resourceAuthorizationRequired",
    no_tools: "resourceNoTools",
    connection_error: "resourceConnectionError",
  };
  const existingKeys = new Set(existing.map(resourceKey));
  const label = t(
    kind === "skill"
      ? "composerResourceSkill"
      : kind === "connector"
        ? "composerResourceConnector"
        : "composerResourceKnowledge",
  );
  return (
    <section
      className={styles.picker}
      aria-label={label}
      onKeyDown={(event) => {
        if (isComposerKeyboardEventComposing(event)) return;
        if (event.key === "Enter" && event.target instanceof HTMLInputElement && event.target.type === "checkbox") {
          event.preventDefault();
          event.target.click();
        }
        if (event.key === "ArrowDown" || event.key === "ArrowUp") {
          const inputs = Array.from(
            event.currentTarget.querySelectorAll<HTMLInputElement>('input[type="checkbox"]:not(:disabled)'),
          );
          const current = inputs.indexOf(document.activeElement as HTMLInputElement);
          const next = event.key === "ArrowDown" ? current + 1 : current - 1;
          if (inputs.length) {
            event.preventDefault();
            inputs[(next + inputs.length) % inputs.length]?.focus();
          }
        }
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
        {items.map((item) => {
          const key = resourceKey(item);
          const checked = selected.some((ref) => resourceKey(ref) === key);
          const alreadySelected = existingKeys.has(key);
          return (
            <label key={key} title={key}>
              <input
                type="checkbox"
                checked={alreadySelected || checked}
                disabled={
                  !item.available ||
                  alreadySelected ||
                  (!checked && existing.length + selected.length >= maxResourceReferences)
                }
                onChange={(event) =>
                  setSelected((value) =>
                    event.target.checked ? [...value, item] : value.filter((ref) => resourceKey(ref) !== key),
                  )
                }
              />
              <span>
                {item.name}
                <small>{item.description}</small>
                <small>{key}</small>
                {!item.available ? (
                  <small>{t(unavailableLabels[item.unavailable_reason ?? ""] ?? "errors.resource_unavailable")}</small>
                ) : null}
              </span>
            </label>
          );
        })}
        {resource.loading ? <p>{t("loading")}</p> : null}
        {resource.continuation.failed ? (
          <div role="alert">
            <p>{localizeAPIError(resource.query.error, t, t("errors.resource_unavailable"))}</p>
            <Button onClick={resource.continuation.retry}>{t("retry")}</Button>
          </div>
        ) : null}
        {resource.continuation.hasMore ? (
          <Button variant="tertiaryGray" size="sm" onClick={() => void resource.continuation.loadMore()}>
            {t("taskEntryLoadMore")}
          </Button>
        ) : null}
        {!resource.loading && !resource.continuation.failed && !items.length ? <p>{t("iaListSearchEmpty")}</p> : null}
      </div>
      <footer>
        {onManage ? (
          <Button variant="tertiaryGray" size="sm" onClick={onManage}>
            {t("taskRuntimeEditAgent")}
          </Button>
        ) : (
          <Link to={pathForPane({ type: WorkspacePaneTypes.agent, id: agentID })} onClick={onClose}>
            {t("taskRuntimeEditAgent")}
          </Link>
        )}
        <Button size="sm" disabled={!selected.length} onClick={() => onPick(selected)}>
          {t("addAttachmentResource")}
        </Button>
      </footer>
    </section>
  );
}
