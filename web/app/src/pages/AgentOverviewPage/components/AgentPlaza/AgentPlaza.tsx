import { AgentProfileHeader } from "@/components/business/AgentDetails";
import { useState, type ReactNode } from "react";
import { Check, Cloud, ExternalLink, Plus, Search, Sparkles } from "lucide-react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { Button, EmptyState, LinkButton, TextInput } from "@/components/ui";
import { filterCloudAgents, type CloudAgent } from "@/models/cloudAgents";
import type { TranslateFn } from "@/models/conversations";
import styles from "./AgentPlaza.module.css";

export function AgentPlazaLayout({ t, children }: { t: TranslateFn; children: ReactNode }) {
  return (
    <div className={styles.page}>
      <section className={styles.intro}>
        <h1>
          <Sparkles size={21} />
          {t("agentPlazaTitle")}
        </h1>
        <strong>{t("agentPlazaTagline")}</strong>
        <p>{t("agentPlazaDescription")}</p>
      </section>
      {children}
    </div>
  );
}

type Props = {
  agents: readonly CloudAgent[];
  addedIDs: readonly string[];
  communityPlazaURL: string;
  t: TranslateFn;
  onAdd: (id: string) => void;
  onOpen: (id: string) => void;
};
export function AgentPlaza({ agents, addedIDs, communityPlazaURL, t, onAdd, onOpen }: Props) {
  const [query, setQuery] = useState("");
  const [scope, setScope] = useState<"public" | "mine">("public");
  const shown = filterCloudAgents(
    agents.filter((agent) =>
      scope === "mine" ? agent.visibility === "private" || addedIDs.includes(agent.id) : agent.visibility === "public",
    ),
    query,
  );
  return (
    <AgentPlazaLayout t={t}>
      <div className={styles.toolbar}>
        <label className={styles.search}>
          <Search size={16} aria-hidden="true" />
          <TextInput
            variant="plain"
            aria-label={t("agentPlazaSearch")}
            placeholder={t("agentPlazaSearch")}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <span>{t("agentPlazaCount", { shown: shown.length, total: agents.length })}</span>
        <nav className={styles.filters} aria-label={t("agentPlazaScope")}>
          {(["public", "mine"] as const).map((value) => (
            <Button
              key={value}
              size="sm"
              variant={scope === value ? "primary" : "secondaryGray"}
              aria-pressed={scope === value}
              onClick={() => setScope(value)}
            >
              {t(value === "public" ? "agentPlazaPublic" : "taskEntryOverview_personal")}
            </Button>
          ))}
        </nav>
      </div>
      <div className={styles.grid}>
        {shown.map((agent) => (
          <article className={styles.card} key={agent.id}>
            <header>
              <span className={styles.avatar}>
                <AgentAvatarContent avatar={agent.avatar} fallback={agent.name.slice(0, 1)} />
              </span>
              <div className={styles.identity}>
                <h2>{agent.name}</h2>
              </div>
              <span className={styles.status} data-online={agent.status === "online"}>
                {t(agent.status === "online" ? "agentPlazaOnline" : "agentPlazaOffline")}
              </span>
            </header>
            <div className={styles.tags}>
              <span>
                <Cloud size={12} />
                {t("agentPlazaCloud")}
              </span>
              <span>{t(agent.visibility === "public" ? "agentPlazaPublic" : "agentPlazaPrivate")}</span>
              {addedIDs.includes(agent.id) ? (
                <span>
                  <Check size={12} />
                  {t("agentPlazaAdded")}
                </span>
              ) : null}
            </div>
            <p className={styles.description}>{agent.description}</p>
            <AgentPlazaMetadata agent={agent} t={t} />
            <footer>
              <Button
                variant={addedIDs.includes(agent.id) ? "secondaryGray" : "primary"}
                size="sm"
                onClick={() => (addedIDs.includes(agent.id) ? onOpen(agent.id) : onAdd(agent.id))}
              >
                {addedIDs.includes(agent.id) ? <Check size={14} /> : <Plus size={14} />}
                {t(addedIDs.includes(agent.id) ? "agentPlazaOpen" : "agentPlazaAdd")}
              </Button>
            </footer>
          </article>
        ))}
      </div>
      {!shown.length ? (
        <EmptyState
          title={t("workspaceSearchNoResults")}
          description={t(scope === "mine" && !query ? "agentPlazaMineEmpty" : "agentPlazaSearchHint")}
          actions={
            communityPlazaURL ? (
              <LinkButton
                variant="primary"
                size="md"
                href={communityPlazaURL}
                target="_blank"
                rel="noopener noreferrer"
              >
                {t("agentPlazaOpenCommunity")}
                <ExternalLink size={14} aria-hidden="true" />
              </LinkButton>
            ) : undefined
          }
        />
      ) : null}
    </AgentPlazaLayout>
  );
}

export function AgentPlazaMetadata({ agent, t }: { agent: CloudAgent; t: TranslateFn }) {
  return (
    <dl className={styles.metadata}>
      <div>
        <dt>{t("agentPlazaSource")}</dt>
        <dd>{agent.name}</dd>
      </div>
      <div>
        <dt>{t("agentPlazaPublisher")}</dt>
        <dd>{agent.publisher}</dd>
      </div>
    </dl>
  );
}

export function CloudAgentDetail({
  agent,
  t,
  onNewTask,
  onRemove,
}: {
  agent: CloudAgent;
  t: TranslateFn;
  onNewTask: () => void;
  onRemove: () => void;
}) {
  const online = agent.status === "online";
  return (
    <section className="entity-pane agent-detail-pane">
      <div className="agent-profile-fixed-header">
        <AgentProfileHeader
          avatar={
            <div className="entity-avatar">
              <AgentAvatarContent avatar={agent.avatar} fallback={agent.name.slice(0, 1)} />
            </div>
          }
          title={
            <>
              <h1>{agent.name}</h1>
              <span className={`agent-status-dot ${online ? "online" : ""}`} aria-hidden="true" />
              <span className={`status-pill ${online ? "online" : ""}`}>
                {t(online ? "agentPlazaOnline" : "agentPlazaOffline")}
              </span>
              <span className={styles.cloudLabel}>
                <Cloud size={14} aria-hidden="true" />
                {t("agentPlazaCloud")}
              </span>
            </>
          }
          description={agent.description ? <div className="entity-description-text">{agent.description}</div> : null}
          actions={
            <>
              <Button variant="primary" size="md" onClick={onNewTask}>
                {t("agentStartTask")}
              </Button>
              <Button variant="secondaryGray" size="md" onClick={onRemove}>
                {t("agentPlazaRemove")}
              </Button>
            </>
          }
        />
        <div className="agent-profile-section-nav">
          <h2 className={`agent-profile-section-tab active ${styles.profileLabel}`}>{t("agentProfileTab")}</h2>
        </div>
      </div>
      <div className="agent-profile-scroll-region">
        <div className="profile-editor-shell agent-page-editor">
          <p className={styles.cloudHint}>{t("agentPlazaCloudHint")}</p>
          <AgentPlazaMetadata agent={agent} t={t} />
        </div>
      </div>
    </section>
  );
}
