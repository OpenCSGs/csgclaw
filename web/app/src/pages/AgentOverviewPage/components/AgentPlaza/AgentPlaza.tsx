import { CloudAgentChat } from "../CloudAgentChat";
import { useState } from "react";
import { Check, Cloud, Plus, Search, Sparkles } from "lucide-react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { Button, EmptyState, TextInput } from "@/components/ui";
import { filterCloudAgents, type CloudAgent } from "@/models/cloudAgents";
import type { TranslateFn } from "@/models/conversations";
import styles from "./AgentPlaza.module.css";

type Props = {
  agents: readonly CloudAgent[];
  addedIDs: readonly string[];
  t: TranslateFn;
  onAdd: (id: string) => void;
  onOpen: (id: string) => void;
};
export function AgentPlaza({ agents, addedIDs, t, onAdd, onOpen }: Props) {
  const [query, setQuery] = useState("");
  const [scope, setScope] = useState<"public" | "mine">("public");
  const shown = filterCloudAgents(
    agents.filter((agent) => (scope === "mine" ? addedIDs.includes(agent.id) : agent.visibility === "public")),
    query,
  );
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
      </div>
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
        />
      ) : null}
    </div>
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

export function CloudAgentDetail({ agent, t, onRemove }: { agent: CloudAgent; t: TranslateFn; onRemove: () => void }) {
  const [chatOpen, setChatOpen] = useState(false);
  return (
    <section className={styles.detail}>
      <header className={styles.detailHeader}>
        <span className={styles.avatar}>
          <AgentAvatarContent avatar={agent.avatar} fallback={agent.name.slice(0, 1)} />
        </span>
        <div>
          <h1>{agent.name}</h1>
          <span className={styles.status} data-online={agent.status === "online"}>
            {t(agent.status === "online" ? "agentPlazaOnline" : "agentPlazaOffline")}
          </span>
        </div>
        <span className={styles.cloudLabel}>
          <Cloud size={14} />
          {t("agentPlazaCloud")}
        </span>
      </header>
      <p className={styles.description}>{agent.description}</p>
      <p className={styles.cloudHint}>{t("agentPlazaCloudHint")}</p>
      <AgentPlazaMetadata agent={agent} t={t} />
      <div className={styles.detailActions}>
        <Button variant="primary" size="sm" onClick={() => setChatOpen((open) => !open)}>
          {t(chatOpen ? "cloudChatHide" : "cloudChatStart")}
        </Button>
        <Button variant="secondaryGray" size="sm" onClick={onRemove}>
          {t("agentPlazaRemove")}
        </Button>
      </div>
      {chatOpen ? <CloudAgentChat key={`${agent.connectionID}:${agent.id}`} agent={agent} t={t} /> : null}
    </section>
  );
}
