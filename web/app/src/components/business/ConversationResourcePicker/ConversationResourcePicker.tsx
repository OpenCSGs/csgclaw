import { useId, useLayoutEffect, useRef, useState } from "react";
import { BookOpen, Boxes, Wrench } from "lucide-react";
import { Button, LinkButton, TextInput } from "@/components/ui";
import { useConversationResources } from "@/hooks/workspace/useConversationResources";
import { isComposerKeyboardEventComposing } from "@/models/composer";
import { maxResourceReferences, resourceKey, type ResourceSnapshot } from "@/models/conversationResources";
import { pathForAgentResources } from "@/models/routing";
import { localizeAPIError } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";
import type { TaskResourceKind } from "@/components/business/ConversationPane/ComposerAddMenu";
import styles from "./ConversationResourcePicker.module.css";

const unavailableLabels: Record<string, string> = {
  disabled: "errors.resource_disabled",
  connecting: "resourceConnecting",
  disconnected: "resourceDisconnected",
  authorization_required: "resourceAuthorizationRequired",
  no_tools: "resourceNoTools",
  connection_error: "resourceConnectionError",
};

export function ConversationResourcePicker({
  agentID,
  kind,
  t,
  onClose,
  onPick,
  existing = [],
  className,
}: {
  agentID: string;
  kind: TaskResourceKind;
  t: TranslateFn;
  onClose: (restoreFocus?: boolean) => void;
  onPick: (resource: ResourceSnapshot) => void;
  existing?: readonly ResourceSnapshot[];
  className?: string;
}) {
  const [query, setQuery] = useState("");
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const activeRef = useRef<HTMLButtonElement>(null);
  const listID = useId();
  const resourceType = kind === "knowledge" ? "knowledge_base" : kind;
  const resource = useConversationResources(agentID, resourceType, query);
  const existingKeys = new Set(existing.map(resourceKey));
  const available = resource.items.filter(
    (item) => item.available && !existingKeys.has(resourceKey(item)) && existing.length < maxResourceReferences,
  );
  const active = available.find((item) => resourceKey(item) === activeKey) ?? available[0];
  const activeIndex = resource.items.indexOf(active);
  const label = t(
    kind === "skill"
      ? "composerResourceSkill"
      : kind === "connector"
        ? "composerResourceConnector"
        : "composerResourceKnowledge",
  );
  const searchLabel = t("composerResourceSearchPlaceholder", { kind: label });
  const manageLabel = t("composerResourceManage", { kind: label });
  const Icon = kind === "skill" ? Wrench : kind === "connector" ? Boxes : BookOpen;
  useLayoutEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeKey, activeIndex]);

  return (
    <div
      className={`${styles.search} ${className ?? ""}`.trim()}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) onClose(false);
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape" && !isComposerKeyboardEventComposing(event)) {
          event.preventDefault();
          event.stopPropagation();
          onClose();
        }
      }}
    >
      <section className={styles.picker} aria-label={label}>
        <div id={listID} role="listbox" aria-label={label} aria-busy={resource.loading} className={styles.list}>
          {resource.items.map((item, index) => {
            const key = resourceKey(item);
            const disabled = !available.includes(item);
            return (
              <button
                key={key}
                id={`${listID}-${index}`}
                type="button"
                role="option"
                aria-selected={item === active}
                disabled={disabled}
                tabIndex={-1}
                ref={item === active ? activeRef : undefined}
                className={styles.option}
                title={key}
                onMouseDown={(event) => event.preventDefault()}
                onMouseMove={() => {
                  if (!disabled) setActiveKey(key);
                }}
                onClick={() => onPick(item)}
              >
                <Icon size={18} strokeWidth={1.8} aria-hidden="true" />
                <span className={styles.copy}>
                  <strong>{item.name}</strong>
                  {item.description ? <span className={styles.description}>{item.description}</span> : null}
                  {!item.available ? (
                    <span className={styles.reason}>
                      {t(unavailableLabels[item.unavailable_reason ?? ""] ?? "errors.resource_unavailable")}
                    </span>
                  ) : null}
                </span>
              </button>
            );
          })}
          {resource.loading ? <p role="status">{t("loading")}</p> : null}
          {!resource.loading && !resource.continuation.failed && !resource.items.length ? (
            <p>{t(query ? "composerResourceSearchEmpty" : "composerResourceEmpty", { kind: label })}</p>
          ) : null}
        </div>
        {resource.continuation.failed ? (
          <div className={styles.status} role="alert">
            {localizeAPIError(resource.query.error, t, t("errors.resource_unavailable"))}
            <Button variant="tertiaryGray" size="sm" onClick={resource.continuation.retry}>
              {t("retry")}
            </Button>
          </div>
        ) : null}
        {resource.continuation.hasMore ? (
          <Button
            variant="tertiaryGray"
            size="sm"
            disabled={resource.continuation.loading}
            onClick={() => void resource.continuation.loadMore()}
          >
            {t(resource.continuation.loading ? "loading" : "taskEntryLoadMore")}
          </Button>
        ) : null}
        <footer className={styles.footer}>
          <LinkButton
            variant="linkColor"
            size="sm"
            href={`#${pathForAgentResources(agentID, resourceType)}`}
            onClick={() => onClose(false)}
          >
            {manageLabel}
          </LinkButton>
        </footer>
      </section>
      <TextInput
        autoFocus
        variant="plain"
        role="combobox"
        aria-label={searchLabel}
        placeholder={searchLabel}
        aria-autocomplete="list"
        aria-expanded="true"
        aria-controls={listID}
        aria-activedescendant={activeIndex >= 0 ? `${listID}-${activeIndex}` : undefined}
        value={query}
        onChange={(event) => {
          setQuery(event.target.value);
          setActiveKey(null);
        }}
        onKeyDown={(event) => {
          if (isComposerKeyboardEventComposing(event)) return;
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            event.stopPropagation();
            const next =
              available[
                (available.indexOf(active) + (event.key === "ArrowDown" ? 1 : -1) + available.length) % available.length
              ];
            if (next) setActiveKey(resourceKey(next));
          }
          if (event.key === "Enter") {
            event.preventDefault();
            event.stopPropagation();
            if (active) onPick(active);
          }
        }}
      />
    </div>
  );
}
