import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowDownWideNarrow, ArrowLeft, ArrowUpWideNarrow, ChevronRight, RefreshCw } from "lucide-react";
import { Button, IconButton, Select, TextInput } from "@/components/ui";
import { MessageContent } from "@/components/business/MessageContent";
import type { CitationSelectHandler } from "@/components/business/MessageContent/types";
import {
  collaborationTurnExcerpt,
  collaborationTurnMatches,
  type CollaborationTurn,
} from "@/models/collaborationTurns";
import {
  localIdentitiesMatch,
  userDisplayName,
  type LocaleCode,
  type TranslateFn,
  type UsersById,
} from "@/models/conversations";
import { progressDuration } from "@/models/turnProgress";
import { isAutonomousCoordinationTool } from "@/models/agentActivity";
import { ActivityPagination, LoadEarlierActivity } from "../ActivityPagination";
import { useActivityPage } from "../useActivityPage";
import {
  activityEntryHasMessage,
  conversationActivityEntries,
  isFailedActivityTool,
  type ConversationActivityAgent,
  type ConversationActivityEntry,
} from "../conversationActivity";
import styles from "./CollaborationActivityTurns.module.css";

type Selection = { agentID?: string; turnID?: string; requestID?: string };
type Row = {
  turn: CollaborationTurn;
  agent: ConversationActivityAgent;
  entries?: ConversationActivityEntry[];
  name: string;
  agentID: string;
  toolCount: number;
  errorCount: number;
};
type Props = {
  autonomous: boolean;
  turns: CollaborationTurn[];
  unassignedCount: number;
  agents: ConversationActivityAgent[];
  usersById: UsersById;
  locale: LocaleCode;
  t: TranslateFn;
  selection?: Selection;
  initialEntryID?: string | null;
  loading: boolean;
  loadError: string;
  onRefresh: () => void;
  onLoadEarlier?: () => void;
  onShowEvents: () => void;
  onCitationSelect?: CitationSelectHandler;
  renderEntry: (entry: ConversationActivityEntry, expandAll: boolean, selected: boolean) => ReactNode;
};

export function CollaborationActivityTurns({
  autonomous,
  turns,
  unassignedCount,
  agents,
  usersById,
  locale,
  t,
  selection,
  initialEntryID,
  loading,
  loadError,
  onRefresh,
  onLoadEarlier,
  onShowEvents,
  onCitationSelect,
  renderEntry,
}: Props) {
  const [actor, setActor] = useState(selection?.agentID || "");
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [newest, setNewest] = useState(true);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [expandAll, setExpandAll] = useState(false);
  const appliedTarget = useRef<unknown>(null);
  const cards = useRef(new Map<string, HTMLButtonElement>());
  const detailHeading = useRef<HTMLHeadingElement>(null);
  const detailBody = useRef<HTMLDivElement>(null);
  const turnList = useRef<HTMLDivElement>(null);
  const rows = useMemo<Row[]>(
    () =>
      turns.map((turn) => {
        const agent = agents.find((agent) =>
          agent.identities.some((id) => localIdentitiesMatch(id, turn.senderID)),
        ) || {
          id: turn.senderID,
          name: userDisplayName(turn.senderID, usersById),
          identities: [turn.senderID],
        };
        // A projected turn can contain thousands of tools in one message.
        // Its list row needs counts; build detail rows only for the open turn.
        if (
          turn.progress &&
          turn.messages.length === 1 &&
          !(turn.messages[0]?.metadata?.csgclaw as { runtime_error?: boolean } | undefined)?.runtime_error
        ) {
          let toolCount = 0;
          let errorCount = 0;
          for (const { tool } of turn.progress.items) {
            if (!tool || (autonomous && isAutonomousCoordinationTool(tool.name))) continue;
            toolCount++;
            if (isFailedActivityTool(tool)) errorCount++;
          }
          return { turn, agent, name: agent.name, agentID: agent.id, toolCount, errorCount };
        }
        const entries = conversationActivityEntries(turn.messages, [agent], [], usersById, autonomous);
        return {
          turn,
          agent,
          entries,
          name: agent.name,
          agentID: agent.id,
          toolCount: entries.filter((entry) => entry.activity?.content.tool || entry.command).length,
          errorCount: entries.filter((entry) => entry.tone === "error").length,
        };
      }),
    [turns, agents, usersById, autonomous],
  );
  const actors = useMemo(() => [...new Map(rows.map((row) => [row.agentID, row.name])).entries()], [rows]);
  const filtered = useMemo(
    () =>
      rows.filter(
        (row) =>
          (!actor || localIdentitiesMatch(actor, row.agentID)) &&
          (!status ||
            (status === "errors"
              ? row.errorCount > 0 || row.turn.status === "failed"
              : status === "active"
                ? ["running", "waiting"].includes(row.turn.status)
                : row.turn.status === status)) &&
          collaborationTurnMatches(row.turn, query),
      ),
    [rows, actor, status, query],
  );
  const displayed = newest ? [...filtered].reverse() : filtered;
  const selected = rows.find((row) => row.turn.key === selectedKey);
  const selectedEntries = useMemo(
    () =>
      selected
        ? (selected.entries ??
          conversationActivityEntries(selected.turn.messages, [selected.agent], [], usersById, autonomous))
        : [],
    [selected, usersById, autonomous],
  );
  const page = useActivityPage(displayed, JSON.stringify([actor, query, status, newest]));
  const detailPage = useActivityPage(
    selectedEntries,
    JSON.stringify([selectedKey, initialEntryID]),
    initialEntryID ? selectedEntries.findIndex((entry) => entry.id === initialEntryID) : -1,
  );

  useEffect(() => {
    setActor(selection?.agentID || "");
    setQuery("");
    setStatus("");
    setSelectedKey(null);
    appliedTarget.current = null;
  }, [selection]);
  useEffect(() => {
    const target = selection?.turnID || selection?.requestID || initialEntryID;
    if (!target || appliedTarget.current === target) return;
    const match = rows.find(
      (row) =>
        (!selection?.agentID || localIdentitiesMatch(selection.agentID, row.agentID)) &&
        (row.turn.id === selection?.turnID ||
          (selection?.requestID && row.turn.sourceID === selection.requestID) ||
          (initialEntryID &&
            (row.entries?.some((entry) => entry.id === initialEntryID) ||
              row.turn.messages.some((message) => activityEntryHasMessage(initialEntryID, message.id))))),
    );
    if (match) {
      setSelectedKey(match.turn.key);
      appliedTarget.current = target;
    }
  }, [rows, selection, initialEntryID]);
  useEffect(() => {
    setExpandAll(false);
    if (selectedKey) detailHeading.current?.focus();
  }, [selectedKey]);

  const back = () => {
    const key = selectedKey;
    setSelectedKey(null);
    requestAnimationFrame(() => {
      if (key) cards.current.get(key)?.focus();
    });
  };
  if (selected)
    return (
      <div className={styles.panel}>
        <div className={styles.detailToolbar}>
          <Button size="sm" variant="tertiaryGray" onClick={back}>
            <ArrowLeft size={14} />
            {t("collaborationTurnBack")}
          </Button>
          <IconButton
            size="sm"
            variant="tertiaryGray"
            label={t("agentActivityRefresh")}
            icon={<RefreshCw size={14} />}
            disabled={loading}
            onClick={onRefresh}
          />
        </div>
        <div className={styles.detailBody} ref={detailBody}>
          <div className={styles.detailHeading}>
            <h3 ref={detailHeading} tabIndex={-1}>
              {selected.name} <span>· {t("collaborationTurnNumber", { count: selected.turn.index })}</span>
            </h3>
            <span className={styles.status} data-state={selected.turn.status}>
              {turnStatus(selected.turn.status, t)}
            </span>
            <div className={styles.metrics}>
              <time dateTime={selected.turn.startedAt}>{formatTime(selected.turn.startedAt, locale)}</time>
              <span>{t("agentActivityToolCallsCount", { count: selected.toolCount })}</span>
              {duration(selected.turn) ? <span>{duration(selected.turn)}</span> : null}
            </div>
          </div>
          {loadError ? (
            <p role="alert" className={styles.error}>
              {loadError}
            </p>
          ) : null}
          <details className={styles.trigger}>
            <summary>
              <span>{t("collaborationTurnTrigger")}</span>
              <p>{collaborationTurnExcerpt(selected.turn.source?.content) || t("collaborationTurnMissingSource")}</p>
            </summary>
            {selected.turn.source ? (
              <MessageContent
                showTurnProgress={false}
                content={selected.turn.source.content}
                message={selected.turn.source}
                actionBusy=""
                actionFeedback={{ key: "", message: "" }}
                onAction={() => undefined}
                onCitationSelect={onCitationSelect}
                t={t}
              />
            ) : (
              <p>{selected.turn.sourceID || "—"}</p>
            )}
          </details>
          <div className={styles.recordHeading}>
            <strong>{t("collaborationTurnSequence")}</strong>
            <Button size="sm" variant="linkGray" onClick={() => setExpandAll(!expandAll)}>
              {t(
                expandAll
                  ? "collaborationTurnCollapse"
                  : detailPage.count > 1
                    ? "activityExpandPage"
                    : "collaborationTurnExpand",
              )}
            </Button>
          </div>
          {selected.turn.progress?.items.some((item) => item.tool?.truncated) ? (
            <p className={styles.notice}>{t("collaborationTurnTruncated")}</p>
          ) : null}
          {selected.toolCount > 0 &&
          (!selected.turn.progress ||
            selected.turn.progress.items.some((item) => item.tool && !item.tool.details_version)) ? (
            <p className={styles.notice}>{t("collaborationTurnHistoricalSummary")}</p>
          ) : null}
          <div role="list" aria-label={t("collaborationTurnSequence")} className={styles.records}>
            {detailPage.items.map((entry) => renderEntry(entry, expandAll, entry.id === initialEntryID))}
          </div>
          <ActivityPagination
            {...detailPage}
            t={t}
            onChange={(index) => {
              if (detailBody.current) detailBody.current.scrollTop = 0;
              setExpandAll(false);
              detailPage.onChange(index);
            }}
          />
          {onLoadEarlier ? <LoadEarlierActivity loading={loading} onLoad={onLoadEarlier} t={t} /> : null}
          {!selectedEntries.length ? <p className={styles.empty}>{t("collaborationTurnQuiet")}</p> : null}
        </div>
      </div>
    );
  return (
    <div className={styles.panel}>
      <div className={styles.filters}>
        <div className={styles.summary}>
          <span>
            {t("collaborationTurnCount", { count: filtered.length })}{" "}
            <span className={styles.total}>/ {rows.length}</span>
          </span>
          <div className={styles.actions}>
            <IconButton
              size="sm"
              variant="tertiaryGray"
              label={t(newest ? "agentActivityChronological" : "agentActivityNewestFirst")}
              icon={newest ? <ArrowUpWideNarrow size={15} /> : <ArrowDownWideNarrow size={15} />}
              onClick={() => setNewest(!newest)}
            />
            <IconButton
              size="sm"
              variant="tertiaryGray"
              label={t("agentActivityRefresh")}
              icon={<RefreshCw size={14} />}
              disabled={loading}
              onClick={onRefresh}
            />
          </div>
        </div>
        <TextInput
          size="sm"
          aria-label={t("collaborationTurnSearch")}
          placeholder={t("collaborationTurnSearch")}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <div className={styles.selects}>
          <Select
            size="sm"
            value={actor}
            onValueChange={setActor}
            triggerProps={{ "aria-label": t("conversationActivityActorFilter") }}
            options={[
              { value: "", label: t("conversationActivityAllActors") },
              ...actors.map(([value, label]) => ({ value, label })),
            ]}
          />
          <Select
            size="sm"
            value={status}
            onValueChange={setStatus}
            triggerProps={{ "aria-label": t("collaborationTurnStatusFilter") }}
            options={[
              { value: "", label: t("collaborationTurnAllStates") },
              { value: "active", label: t("collaborationTurnActive") },
              { value: "errors", label: t("collaborationTurnWithErrors") },
              { value: "succeeded", label: turnStatus("succeeded", t) },
              { value: "canceled", label: turnStatus("canceled", t) },
            ]}
          />
        </div>
      </div>
      <div className={styles.turnList} ref={turnList}>
        {loadError ? (
          <p role="alert" className={styles.error}>
            {loadError}
          </p>
        ) : null}
        {onLoadEarlier ? <p className={styles.notice}>{t("activityPartialHistory")}</p> : null}
        {page.items.map((row) => (
          <Button
            key={row.turn.key}
            ref={(node) => {
              if (node) cards.current.set(row.turn.key, node);
              else cards.current.delete(row.turn.key);
            }}
            variant="ghost"
            className={styles.turn}
            onClick={() => setSelectedKey(row.turn.key)}
            aria-label={`${row.name} · ${t("collaborationTurnNumber", { count: row.turn.index })}`}
          >
            <span className={styles.turnBody}>
              <span className={styles.turnTop}>
                <strong>{row.name}</strong>
                <span className={styles.number}>#{row.turn.index}</span>
                <span className={styles.status} data-state={row.turn.status}>
                  {turnStatus(row.turn.status, t)}
                </span>
                <ChevronRight size={14} />
              </span>
              <span className={styles.subject}>
                {collaborationTurnExcerpt(row.turn.source?.content) ||
                  collaborationTurnExcerpt(row.turn.progress?.items.find((item) => item.kind === "commentary")?.text) ||
                  t("collaborationTurnMissingSource")}
              </span>
              <span className={styles.metrics}>
                <time dateTime={row.turn.startedAt}>{formatTime(row.turn.startedAt, locale)}</time>
                <span>{t("agentActivityToolCallsCount", { count: row.toolCount })}</span>
                {duration(row.turn) ? <span>{duration(row.turn)}</span> : null}
                {row.errorCount ? (
                  <span className={styles.error}>{t("collaborationTurnErrors", { count: row.errorCount })}</span>
                ) : null}
                {row.turn.threadRootID ? <span>{t("collaborationTurnThread")}</span> : null}
              </span>
            </span>
          </Button>
        ))}
        {!loadError && !displayed.length ? (
          <p className={styles.empty}>{t(loading ? "agentActivityLoading" : "agentActivityNoFilteredResults")}</p>
        ) : null}
        {unassignedCount ? (
          <Button size="sm" variant="linkGray" className={styles.unassigned} onClick={onShowEvents}>
            {t("collaborationTurnUnassigned", { count: unassignedCount })}
            <ChevronRight size={13} />
          </Button>
        ) : null}
        <ActivityPagination
          {...page}
          t={t}
          onChange={(index) => {
            if (turnList.current) turnList.current.scrollTop = 0;
            page.onChange(index);
          }}
        />
        {onLoadEarlier ? <LoadEarlierActivity loading={loading} onLoad={onLoadEarlier} t={t} /> : null}
      </div>
    </div>
  );
}
function duration(turn: CollaborationTurn): string {
  return turn.progress && turn.endedAt ? progressDuration(turn.progress, Date.now()) : "";
}
function turnStatus(status: string, t: TranslateFn): string {
  if (status === "succeeded") return t("collaborationTurnCompleted");
  return t(
    ["running", "waiting", "failed", "canceled", "interrupted"].includes(status)
      ? `progressStatus_${status}`
      : "collaborationTurnLegacy",
  );
}
function formatTime(value: string, locale: LocaleCode): string {
  const date = new Date(value);
  return Number.isFinite(date.getTime())
    ? new Intl.DateTimeFormat(locale, {
        month: "numeric",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      }).format(date)
    : "—";
}
