import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from "react";
import {
  ArrowDownNarrowWide,
  ArrowUpNarrowWide,
  ChevronRight,
  Clock,
  Filter,
  RefreshCw,
  UserRound,
  X,
} from "lucide-react";
import { fetchMessagesRequest } from "@/api/im";
import { errorMessage } from "@/api/client";
import { MessageContent } from "@/components/business/MessageContent";
import {
  Button,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Tooltip,
} from "@/components/ui";
import type { AgentLike } from "@/models/agents";
import {
  isDirectConversation,
  resolveUserByLocalIdentity,
  type IMConversation,
  type IMMessage,
  type LocaleCode,
  type TranslateFn,
  type UsersById,
  userIDForLocalIdentity,
  userDisplayName,
} from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import { CONVERSATION_ACTIVITY_PANEL_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import {
  conversationActivityAgents,
  conversationActivityDensitySegments,
  conversationActivityEntries,
  conversationActivityEntryDetails,
  conversationActivityEntrySummary,
  mergeConversationActivityMessages,
  type ConversationActivityAgent,
  type ConversationActivityEntry,
  type ConversationActivityTone,
} from "./conversationActivity";
import styles from "./ConversationActivityPanel.module.css";

type ConversationActivityPanelProps = {
  agents: readonly AgentLike[];
  conversation: IMConversation;
  locale: LocaleCode;
  onClose: () => void;
  t: TranslateFn;
  usersById: UsersById;
};

type SortMode = "chronological" | "newest_first";

type FilterOption = {
  count: number;
  id: string;
  tone: ConversationActivityTone;
};

type ActivityActorOption = {
  filterID: string;
  hueIndex: number;
  kind: "agent" | "human";
  name: string;
};

const DEFAULT_PANEL_WIDTH = 640;
const MIN_PANEL_WIDTH = 420;
const MIN_CONVERSATION_WIDTH = 360;

function readPanelWidth() {
  if (typeof window === "undefined") {
    return DEFAULT_PANEL_WIDTH;
  }
  try {
    const stored = Number(window.localStorage.getItem(CONVERSATION_ACTIVITY_PANEL_WIDTH_STORAGE_KEY));
    return Number.isFinite(stored) && stored > 0 ? Math.round(stored) : DEFAULT_PANEL_WIDTH;
  } catch {
    return DEFAULT_PANEL_WIDTH;
  }
}

function persistPanelWidth(width: number) {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(CONVERSATION_ACTIVITY_PANEL_WIDTH_STORAGE_KEY, String(Math.round(width)));
  } catch {
    // Storage can be unavailable in strict privacy modes; resizing should still work.
  }
}

export function ConversationActivityPanel({
  agents,
  conversation,
  locale,
  onClose,
  t,
  usersById,
}: ConversationActivityPanelProps) {
  const [messages, setMessages] = useState<IMMessage[]>(() => mergeConversationActivityMessages(conversation.messages));
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [sortMode, setSortMode] = useState<SortMode>("newest_first");
  const [selectedTypes, setSelectedTypes] = useState<Set<string>>(() => new Set());
  const [selectedActorID, setSelectedActorID] = useState("all");
  const [selectedEntryID, setSelectedEntryID] = useState<string | null>(null);
  const [panelWidth, setPanelWidth] = useState(readPanelWidth);
  const [resizing, setResizing] = useState(false);
  const mountedRef = useRef(true);
  const requestIDRef = useRef(0);
  const panelRef = useRef<HTMLElement | null>(null);
  const rowRefs = useRef<Map<string, HTMLElement>>(new Map());
  const resizeRef = useRef({ pointerID: -1, startWidth: DEFAULT_PANEL_WIDTH, startX: 0 });
  const direct = isDirectConversation(conversation);

  const activityAgents = useMemo(() => conversationActivityAgents(conversation, agents), [agents, conversation]);
  const entries = useMemo(
    () => conversationActivityEntries(messages, activityAgents, conversation.members, usersById),
    [activityAgents, conversation.members, messages, usersById],
  );
  const activityActors = useMemo(
    () => conversationActivityActorOptions(activityAgents, entries, usersById, t),
    [activityAgents, entries, t, usersById],
  );
  const actorEntries = useMemo(
    () =>
      selectedActorID === "all"
        ? entries
        : entries.filter((entry) => activityActorFilterID(entry, usersById) === selectedActorID),
    [entries, selectedActorID, usersById],
  );
  const filterOptions = useMemo(() => activityFilterOptions(actorEntries), [actorEntries]);
  const filteredEntries = useMemo(
    () =>
      selectedTypes.size === 0 ? actorEntries : actorEntries.filter((entry) => selectedTypes.has(entry.eventType)),
    [actorEntries, selectedTypes],
  );
  const displayEntries = useMemo(
    () => (sortMode === "newest_first" ? [...filteredEntries].reverse() : filteredEntries),
    [filteredEntries, sortMode],
  );
  const summary = useMemo(() => activitySummary(entries, filteredEntries.length), [entries, filteredEntries.length]);

  const refresh = useCallback(async () => {
    const requestID = ++requestIDRef.current;
    setLoading(true);
    setLoadError("");
    try {
      const fetched = await fetchMessagesRequest(conversation.id, { includeThreadReplies: true });
      if (!mountedRef.current || requestID !== requestIDRef.current) {
        return;
      }
      setMessages((current) => mergeConversationActivityMessages(fetched, current));
    } catch (error) {
      if (mountedRef.current && requestID === requestIDRef.current) {
        setLoadError(errorMessage(error, t("agentActivityLoadFailed")));
      }
    } finally {
      if (mountedRef.current && requestID === requestIDRef.current) {
        setLoading(false);
      }
    }
  }, [conversation.id, t]);

  useEffect(() => {
    mountedRef.current = true;
    const unsubscribe = subscribeIMEvents((payload) => {
      if (payload.type !== "message.created" || payload.room_id !== conversation.id || !payload.message) {
        return;
      }
      const message = payload.message;
      setMessages((current) => mergeConversationActivityMessages(current, [message]));
    });
    void refresh();
    return () => {
      mountedRef.current = false;
      requestIDRef.current += 1;
      unsubscribe();
    };
  }, [conversation.id, refresh]);

  useEffect(() => {
    setMessages((current) => mergeConversationActivityMessages(current, conversation.messages));
  }, [conversation.messages]);

  const scrollToEntry = useCallback((entryID: string) => {
    setSelectedEntryID(entryID);
    rowRefs.current.get(entryID)?.scrollIntoView({ behavior: "smooth", block: "center" });
  }, []);

  const toggleType = useCallback((eventType: string) => {
    setSelectedTypes((current) => {
      const next = new Set(current);
      if (next.has(eventType)) {
        next.delete(eventType);
      } else {
        next.add(eventType);
      }
      return next;
    });
  }, []);

  const resizeBounds = useCallback(() => {
    const containerWidth = panelRef.current?.parentElement?.getBoundingClientRect().width || window.innerWidth;
    return {
      max: Math.max(MIN_PANEL_WIDTH, containerWidth - MIN_CONVERSATION_WIDTH),
      min: Math.min(MIN_PANEL_WIDTH, Math.max(280, containerWidth - 120)),
    };
  }, []);

  const clampPanelWidth = useCallback(
    (width: number) => {
      const bounds = resizeBounds();
      return Math.min(bounds.max, Math.max(bounds.min, width));
    },
    [resizeBounds],
  );

  useEffect(() => {
    setPanelWidth((current) => clampPanelWidth(current));
  }, [clampPanelWidth]);

  useEffect(() => {
    if (!resizing) {
      persistPanelWidth(panelWidth);
    }
  }, [panelWidth, resizing]);

  const handleResizeStart = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      resizeRef.current = { pointerID: event.pointerId, startWidth: panelWidth, startX: event.clientX };
      event.currentTarget.setPointerCapture(event.pointerId);
      setResizing(true);
    },
    [panelWidth],
  );

  const handleResizeMove = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (resizeRef.current.pointerID !== event.pointerId) {
        return;
      }
      setPanelWidth(clampPanelWidth(resizeRef.current.startWidth + resizeRef.current.startX - event.clientX));
    },
    [clampPanelWidth],
  );

  const handleResizeEnd = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (resizeRef.current.pointerID !== event.pointerId) {
      return;
    }
    resizeRef.current.pointerID = -1;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    setResizing(false);
  }, []);

  const panelStyle = {
    "--conversation-activity-panel-width": `${panelWidth}px`,
  } as CSSProperties;

  return (
    <aside
      ref={panelRef}
      className={classNames("conversation-activity-panel", styles.panel, resizing && styles.resizing)}
      aria-labelledby="conversation-activity-title"
      style={panelStyle}
    >
      <div
        className={styles.resizeHandle}
        role="separator"
        tabIndex={0}
        aria-label={t("conversationActivityResize")}
        aria-orientation="vertical"
        aria-valuenow={Math.round(panelWidth)}
        onKeyDown={(event) => {
          if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
            return;
          }
          event.preventDefault();
          setPanelWidth((current) => clampPanelWidth(current + (event.key === "ArrowLeft" ? 24 : -24)));
        }}
        onPointerCancel={handleResizeEnd}
        onPointerDown={handleResizeStart}
        onPointerMove={handleResizeMove}
        onPointerUp={handleResizeEnd}
      />

      <header className={styles.header}>
        <div className={styles.titleGroup}>
          <h2 id="conversation-activity-title">{t("agentActivityTitle")}</h2>
          <p>
            {t(direct ? "conversationActivityCurrentConversation" : "conversationActivityRoomTimeline", {
              name: conversation.title || conversation.id,
            })}
          </p>
        </div>
        <Button
          className={styles.closeButton}
          iconOnly
          size="sm"
          variant="tertiaryGray"
          aria-label={t("close")}
          title={t("close")}
          onClick={onClose}
        >
          <X aria-hidden="true" size={17} />
        </Button>
      </header>

      <div className={styles.toolbar}>
        <div className={styles.toolbarTop}>
          <div className={styles.summary} aria-label={t("agentActivitySummaryLabel")}>
            {summary.duration ? (
              <MetaChip icon={<Clock aria-hidden="true" size={14} />}>{summary.duration}</MetaChip>
            ) : null}
            <MetaChip>{t("agentActivityToolCallsCount", { count: summary.toolCount })}</MetaChip>
            <MetaChip>
              {selectedTypes.size || selectedActorID !== "all"
                ? t("agentActivityFilteredEventsCount", { shown: summary.filteredCount, total: summary.eventCount })
                : t("agentActivityEventsCount", { count: summary.eventCount })}
            </MetaChip>
          </div>
          <div className={styles.controls}>
            <div className={styles.sortControl} role="group" aria-label={t("agentActivitySortLabel")}>
              <button
                type="button"
                className={classNames(styles.sortButton, sortMode === "chronological" && styles.active)}
                aria-pressed={sortMode === "chronological"}
                onClick={() => setSortMode("chronological")}
              >
                <ArrowDownNarrowWide aria-hidden="true" size={14} />
                <span>{t("agentActivityChronological")}</span>
              </button>
              <button
                type="button"
                className={classNames(styles.sortButton, sortMode === "newest_first" && styles.active)}
                aria-pressed={sortMode === "newest_first"}
                onClick={() => setSortMode("newest_first")}
              >
                <ArrowUpNarrowWide aria-hidden="true" size={14} />
                <span>{t("agentActivityNewestFirst")}</span>
              </button>
            </div>
            <ActivityFilterMenu
              options={filterOptions}
              selected={selectedTypes}
              t={t}
              onClear={() => setSelectedTypes(new Set())}
              onToggle={toggleType}
            />
            <Button
              className={styles.refreshButton}
              iconOnly
              size="sm"
              variant="secondaryGray"
              disabled={loading}
              aria-label={t("agentActivityRefresh")}
              title={t("agentActivityRefresh")}
              onClick={() => void refresh()}
            >
              <RefreshCw className={loading ? styles.spinning : undefined} aria-hidden="true" size={15} />
            </Button>
          </div>
        </div>

        {entries.length ? (
          <ActivityDensityBar
            entries={filteredEntries}
            locale={locale}
            selectedEntryID={selectedEntryID}
            t={t}
            usersById={usersById}
            onSelect={scrollToEntry}
          />
        ) : null}

        {!direct && activityActors.length ? (
          <div className={styles.actorFilters} role="group" aria-label={t("conversationActivityActorFilter")}>
            <button
              type="button"
              className={classNames(styles.actorChip, selectedActorID === "all" && styles.selectedActor)}
              aria-pressed={selectedActorID === "all"}
              onClick={() => setSelectedActorID("all")}
            >
              <span className={styles.allActorsDot} aria-hidden="true" />
              {t("conversationActivityAllActors")}
            </button>
            {activityActors.map((actor) => (
              <button
                key={actor.filterID}
                type="button"
                className={classNames(styles.actorChip, selectedActorID === actor.filterID && styles.selectedActor)}
                aria-pressed={selectedActorID === actor.filterID}
                onClick={() => setSelectedActorID(actor.filterID)}
              >
                {actor.kind === "human" ? (
                  <UserRound className={styles.humanActorIcon} aria-hidden="true" size={13} strokeWidth={2} />
                ) : (
                  <span
                    className={styles.agentDot}
                    style={{ "--agent-hue": agentHue(actor.hueIndex) } as CSSProperties}
                    aria-hidden="true"
                  />
                )}
                {actor.name}
              </button>
            ))}
          </div>
        ) : null}
      </div>

      <div className={styles.body}>
        {loadError ? <div className={styles.loadError}>{loadError}</div> : null}
        {loading && entries.length === 0 ? <div className={styles.empty}>{t("agentActivityLoading")}</div> : null}
        {!loading && entries.length === 0 ? <div className={styles.empty}>{t("agentActivityEmpty")}</div> : null}
        {!loading && entries.length > 0 && displayEntries.length === 0 ? (
          <div className={styles.empty}>{t("agentActivityNoFilteredResults")}</div>
        ) : null}
        {displayEntries.length ? (
          <div className={styles.list} role="list">
            {displayEntries.map((entry) => (
              <ActivityRow
                key={entry.id}
                direct={direct}
                entry={entry}
                locale={locale}
                selected={selectedEntryID === entry.id}
                t={t}
                usersById={usersById}
                rowRef={(node) => {
                  if (node) {
                    rowRefs.current.set(entry.id, node);
                  } else {
                    rowRefs.current.delete(entry.id);
                  }
                }}
              />
            ))}
          </div>
        ) : null}
      </div>
    </aside>
  );
}

function ActivityFilterMenu({
  options,
  selected,
  t,
  onClear,
  onToggle,
}: {
  options: readonly FilterOption[];
  selected: ReadonlySet<string>;
  t: TranslateFn;
  onClear: () => void;
  onToggle: (id: string) => void;
}) {
  return (
    <DropdownMenuRoot>
      <DropdownMenuTrigger asChild>
        <Button
          className={classNames(styles.filterButton, selected.size > 0 && styles.active)}
          size="sm"
          variant="secondaryGray"
        >
          <Filter aria-hidden="true" size={14} />
          <span>{t("agentActivityFilter")}</span>
          {selected.size ? <strong>{selected.size}</strong> : null}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className={styles.filterMenu} aria-label={t("agentActivityFilter")}>
        {options.map((option) => (
          <DropdownMenuCheckboxItem
            key={option.id}
            checked={selected.has(option.id)}
            onCheckedChange={() => onToggle(option.id)}
          >
            <span className={classNames(styles.filterDot, styles[option.tone])} aria-hidden="true" />
            <span className={styles.filterLabel}>{option.id}</span>
            <span className={styles.filterCount}>{option.count}</span>
          </DropdownMenuCheckboxItem>
        ))}
        {selected.size ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={onClear}>{t("agentActivityClearFilters")}</DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenuRoot>
  );
}

function ActivityDensityBar({
  entries,
  locale,
  selectedEntryID,
  t,
  usersById,
  onSelect,
}: {
  entries: readonly ConversationActivityEntry[];
  locale: LocaleCode;
  selectedEntryID: string | null;
  t: TranslateFn;
  usersById: UsersById;
  onSelect: (entryID: string) => void;
}) {
  const segments = conversationActivityDensitySegments(entries);
  return (
    <div className={styles.density} role="navigation" aria-label={t("agentActivityTimelineLabel")}>
      <div className={styles.densityTrack}>
        {segments.map(({ durationPercent, entry, startPercent }) => {
          const segmentStyle = {
            left: `${startPercent}%`,
            width: `max(3px, ${durationPercent}%)`,
          } as CSSProperties;
          return (
            <Tooltip
              key={entry.id}
              content={`${entry.eventType} · ${activityActorName(entry, usersById, t)} · ${formatTime(entry.createdAt, locale)}`}
            >
              <button
                type="button"
                className={classNames(
                  styles.densitySegment,
                  styles[entry.tone],
                  selectedEntryID === entry.id && styles.selectedSegment,
                )}
                style={segmentStyle}
                aria-label={`${entry.eventType} #${entry.index} ${formatTime(entry.createdAt, locale)}`}
                onClick={() => onSelect(entry.id)}
              />
            </Tooltip>
          );
        })}
      </div>
    </div>
  );
}

function ActivityRow({
  direct,
  entry,
  locale,
  rowRef,
  selected,
  t,
  usersById,
}: {
  direct: boolean;
  entry: ConversationActivityEntry;
  locale: LocaleCode;
  rowRef: (node: HTMLElement | null) => void;
  selected: boolean;
  t: TranslateFn;
  usersById: UsersById;
}) {
  const [expanded, setExpanded] = useState(false);
  const details = conversationActivityEntryDetails(entry);
  const hasDetail = details.length > 0;

  return (
    <article
      ref={rowRef}
      className={classNames(styles.row, styles[entry.tone], selected && styles.selectedRow)}
      role="listitem"
    >
      <div className={classNames(styles.rowMain, direct && styles.directRow)}>
        <span className={classNames(styles.typeBadge, styles[entry.tone])}>{entry.eventType}</span>
        {hasDetail ? (
          <button
            type="button"
            className={styles.rowSummary}
            aria-expanded={expanded}
            onClick={() => setExpanded((current) => !current)}
          >
            <ChevronRight className={expanded ? styles.expanded : undefined} aria-hidden="true" size={14} />
            <span>{conversationActivityEntrySummary(entry) || "-"}</span>
          </button>
        ) : (
          <div className={styles.rowSummaryText}>{conversationActivityEntrySummary(entry) || "-"}</div>
        )}
        {!direct ? <span className={styles.agentName}>{activityActorName(entry, usersById, t)}</span> : null}
        <span className={styles.sequence}>#{entry.index}</span>
        <time className={styles.time} dateTime={entry.createdAt}>
          {formatTime(entry.createdAt, locale)}
        </time>
      </div>
      {hasDetail && expanded ? (
        <div className={styles.detail}>
          {entry.source === "user" || entry.eventType === "message" ? (
            <MessageContent
              content={entry.message.content}
              message={entry.message}
              actionBusy=""
              actionFeedback={{ key: "", message: "" }}
              onAction={() => undefined}
              t={t}
            />
          ) : (
            details.map((detail) => (
              <section key={`${detail.kind}:${detail.value.slice(0, 32)}`} className={styles.detailSection}>
                <strong>{detailLabel(detail.kind, t)}</strong>
                <pre>{detail.value}</pre>
              </section>
            ))
          )}
        </div>
      ) : null}
    </article>
  );
}

function conversationActivityActorOptions(
  agents: readonly ConversationActivityAgent[],
  entries: readonly ConversationActivityEntry[],
  usersById: UsersById,
  t: TranslateFn,
): ActivityActorOption[] {
  const options: ActivityActorOption[] = agents.map((agent, index) => ({
    filterID: `agent:${agent.id}`,
    hueIndex: index,
    kind: "agent",
    name: agent.name,
  }));
  const known = new Set(options.map((option) => option.filterID));
  entries.forEach((entry) => {
    if (entry.source !== "user") {
      return;
    }
    const filterID = activityActorFilterID(entry, usersById);
    if (known.has(filterID)) {
      return;
    }
    known.add(filterID);
    options.push({
      filterID,
      hueIndex: options.length,
      kind: "human",
      name: activityActorName(entry, usersById, t),
    });
  });
  return options;
}

function activityActorFilterID(entry: ConversationActivityEntry, usersById: UsersById): string {
  if (entry.source !== "user") {
    return `agent:${entry.agentID}`;
  }
  const user = resolveUserByLocalIdentity(entry.message.sender_id, usersById);
  const identity = user?.id || userIDForLocalIdentity(entry.message.sender_id) || "unknown";
  return `user:${identity}`;
}

function activityActorName(entry: ConversationActivityEntry, usersById: UsersById, t: TranslateFn): string {
  if (entry.source !== "user") {
    return entry.agentName;
  }
  return userDisplayName(entry.message.sender_id, usersById) || t("localIdentityFallback");
}

function MetaChip({ children, icon }: { children: ReactNode; icon?: ReactNode }) {
  return (
    <span className={styles.metaChip}>
      {icon}
      <span>{children}</span>
    </span>
  );
}

function activityFilterOptions(entries: readonly ConversationActivityEntry[]): FilterOption[] {
  const options = new Map<string, FilterOption>();
  entries.forEach((entry) => {
    const existing = options.get(entry.eventType);
    if (existing) {
      existing.count += 1;
    } else {
      options.set(entry.eventType, { count: 1, id: entry.eventType, tone: entry.tone });
    }
  });
  return Array.from(options.values()).sort((left, right) => left.id.localeCompare(right.id));
}

function activitySummary(entries: readonly ConversationActivityEntry[], filteredCount: number) {
  const starts = entries.map((entry) => timestamp(entry.createdAt)).filter((value) => value > 0);
  const ends = entries.map((entry) => timestamp(entry.updatedAt)).filter((value) => value > 0);
  return {
    duration: starts.length && ends.length ? formatDuration(Math.min(...starts), Math.max(...ends)) : "",
    eventCount: entries.length,
    filteredCount,
    toolCount: entries.filter((entry) => entry.tone === "tool" || entry.tone === "error").length,
  };
}

function detailLabel(kind: string, t: TranslateFn): string {
  switch (kind) {
    case "command":
      return t("agentActivityCommand");
    case "input":
      return t("agentActivityInput");
    case "result":
      return t("agentActivityResult");
    default:
      return t("conversationActivityContent");
  }
}

function timestamp(value: string): number {
  const result = Date.parse(value);
  return Number.isFinite(result) ? result : 0;
}

function formatDuration(start: number, end: number): string {
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ${seconds % 60}s`;
  }
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

function formatTime(value: string, locale: LocaleCode): string {
  const valueTimestamp = timestamp(value);
  if (!valueTimestamp) {
    return "";
  }
  return new Intl.DateTimeFormat(locale.startsWith("zh") ? "zh-CN" : "en-US", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(valueTimestamp));
}

function agentHue(index: number): string {
  return String([216, 156, 270, 32, 188, 338][index % 6]);
}
