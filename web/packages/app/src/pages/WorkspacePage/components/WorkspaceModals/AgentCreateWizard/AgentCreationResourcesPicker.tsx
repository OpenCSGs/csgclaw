import { useRef, useState } from "react";
import { Button, Checkbox, TextInput, Tooltip } from "@/components/ui";
import { AppIcon, appStatus } from "@/components/business/Apps";
import { mcpManagedKnowledgeBaseSource, mcpServerDisplayName, mcpServersFromCatalogResponse } from "@/models/mcp";
import { mergeRemoteKnowledgeBasePages } from "@/models/knowledgeBases";
import type { AgentCreationResources } from "@/models/agentCreation";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";
import type { useAgentCreationResourceQueries } from "./useAgentCreationResourceQueries";
import styles from "./AgentCreateWizard.module.css";

function ResourceDescription({ text }: { text: string }) {
  const ref = useRef<HTMLElement>(null);
  const [open, setOpen] = useState(false);
  return (
    <Tooltip
      content={text}
      open={open}
      onOpenChange={(next) => setOpen(next && !!ref.current && ref.current.scrollWidth > ref.current.clientWidth)}
      contentProps={{ className: styles.descriptionTooltip, side: "top" }}
    >
      <small ref={ref}>{text}</small>
    </Tooltip>
  );
}

type Props = {
  step: number;
  queries: ReturnType<typeof useAgentCreationResourceQueries>;
  value: AgentCreationResources;
  onChange: (value: AgentCreationResources) => void;
  authenticated: boolean;
  onRequireAuth: () => boolean;
  t: TranslateFn;
};

export function AgentCreationResourcesPicker({
  step,
  queries,
  value,
  onChange,
  authenticated,
  onRequireAuth,
  t,
}: Props) {
  const [search, setSearch] = useState("");
  const { skills, mcp, apps, knowledge } = queries;
  const knowledgeItems = mergeRemoteKnowledgeBasePages(knowledge.data?.pages ?? []);
  const toggle = (kind: "skills" | "connectors" | "apps", key: string, checked: boolean) =>
    onChange({ ...value, [kind]: checked ? [...value[kind], key] : value[kind].filter((item) => item !== key) });
  const groups =
    step === 2
      ? [
          {
            label: t("agentProfileSkillsTab"),
            query: skills,
            items: (skills.data ?? []).map((item) => ({
              key: item.name,
              name: item.name,
              description: item.description,
            })),
            selected: value.skills,
            toggle: (key: string, checked: boolean) => toggle("skills", key, checked),
          },
        ]
      : step === 3
        ? [
            {
              label: t("iaConnectorsGroupApps"),
              query: apps,
              items: (apps.data ?? []).map((item) => ({
                key: item.installation_id,
                name: item.name,
                description: `${t("appManagedMCP")} · ${appStatus(item, t)}`,
                appID: item.app_id,
                disabled: item.resource_enabled === false || item.status !== "connected",
              })),
              selected: value.apps,
              toggle: (key: string, checked: boolean) => toggle("apps", key, checked),
            },
            {
              label: t("iaConnectorsGroupTools"),
              query: mcp,
              items: mcpServersFromCatalogResponse(mcp.data ?? null)
                .filter((item) => !mcpManagedKnowledgeBaseSource(item.config))
                .map((item) => ({
                  key: item.name,
                  name: mcpServerDisplayName(item),
                  description: item.description,
                  disabled: item.config.disabled === true,
                })),
              selected: value.connectors,
              toggle: (key: string, checked: boolean) => toggle("connectors", key, checked),
            },
          ]
        : [
            {
              label: t("resourcesKnowledgeBasesLabel"),
              query: knowledge,
              items: knowledgeItems.map((item) => ({
                key: item.id,
                name: item.name,
                description: item.description,
                disabled: item.availability !== "available",
              })),
              selected: value.knowledgeBases.map((item) => item.id),
              toggle: (key: string, checked: boolean) => {
                const item = knowledgeItems.find((item) => item.id === key);
                if (item)
                  onChange({
                    ...value,
                    knowledgeBases: checked
                      ? [...value.knowledgeBases, item]
                      : value.knowledgeBases.filter((item) => item.id !== key),
                  });
              },
            },
          ];
  if (step === 4 && !authenticated)
    return (
      <div>
        <p>{t("iaKnowledgeLoginRequired")}</p>
        <Button variant="secondaryGray" onClick={() => onRequireAuth()}>
          {t("modelProviderOpenCSGAuthorize")}
        </Button>
      </div>
    );
  return (
    <div className={styles.resources}>
      {step === 2 ? <p className={styles.resourceIntro}>{t("agentWizardSkillsDescription")}</p> : null}
      {step === 4 ? <p className={styles.resourceIntro}>{t("agentWizardKnowledgeDescription")}</p> : null}
      <TextInput
        aria-label={t("search")}
        placeholder={t("search")}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />
      {groups.map((group) => {
        const loading = group.query.data === undefined && group.query.isFetching;
        const items = group.items.filter((item) =>
          `${item.name} ${item.description || ""}`.toLowerCase().includes(search.trim().toLowerCase()),
        );
        return (
          <section key={group.label} className={styles.resourceGroup} aria-busy={loading}>
            <h4>{group.label}</h4>
            {group.query.isError ? (
              <div role="alert" className={styles.resourceError}>
                {localizeAPIError(group.query.error, t, t("loadingFailed"))}
                <Button variant="tertiaryGray" onClick={() => void group.query.refetch()}>
                  {t("retry")}
                </Button>
              </div>
            ) : null}
            {loading ? (
              <div role="status" aria-label={t("agentWizardLoadingResources")}>
                <span className="sr-only">{t("agentWizardLoadingResources")}</span>
                <div className={styles.resourceList} aria-hidden="true">
                  {[0, 1, 2].map((index) => (
                    <div key={index} className={styles.skeletonRow}>
                      <span className={styles.skeletonCheckbox} />
                      <span className={styles.skeletonContent}>
                        <span className={styles.skeletonTitle} />
                        <span className={styles.skeletonDescription} />
                        <span className={styles.skeletonDescription} />
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}
            <ul
              className={`${styles.resourceList} ${items.length > 5 ? styles.scrollableResourceList : ""}`}
              aria-label={group.label}
              tabIndex={items.length > 5 ? 0 : undefined}
            >
              {items.map((item) => {
                const description =
                  "disabled" in item && item.disabled ? t("agentWizardResourceUnavailable") : item.description;
                return (
                  <li key={item.key}>
                    <label
                      className={`${styles.resourceRow} ${step === 2 ? styles.skillRow : ""} ${step === 3 || step === 4 ? styles.connectorRow : ""} ${"appID" in item ? styles.appRow : ""}`}
                    >
                      <span className={styles.resourceCheckbox}>
                        <Checkbox
                          checked={group.selected.includes(item.key)}
                          disabled={"disabled" in item && item.disabled === true}
                          onCheckedChange={(checked) => group.toggle(item.key, checked === true)}
                        />
                      </span>
                      {"appID" in item && typeof item.appID === "string" ? (
                        <span className={styles.resourceIcon} aria-hidden="true">
                          <AppIcon appID={item.appID} />
                        </span>
                      ) : null}
                      <span className={styles.resourceCopy}>
                        <strong>{item.name}</strong>
                        {description ? <ResourceDescription text={description} /> : null}
                      </span>
                    </label>
                  </li>
                );
              })}
            </ul>
            {!items.length && !loading && !group.query.isError ? <p>{t("iaListSearchEmpty")}</p> : null}
          </section>
        );
      })}
      {step === 4 && knowledge.hasNextPage ? (
        <Button
          variant="secondaryGray"
          disabled={knowledge.isFetchingNextPage}
          loading={knowledge.isFetchingNextPage}
          onClick={() => void knowledge.fetchNextPage()}
        >
          {t("taskEntryLoadMore")}
        </Button>
      ) : null}
    </div>
  );
}
