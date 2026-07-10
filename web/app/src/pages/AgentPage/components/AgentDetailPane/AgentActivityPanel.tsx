import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDownNarrowWide, ArrowUpNarrowWide, ChevronRight, Clock, Filter, RefreshCw } from "lucide-react";
import { fetchMessagesRequest } from "@/api/im";
import { errorMessage } from "@/api/client";
import { MessageContent } from "@/components/business/MessageContent";
import { resolveAgentChannelUserID, type AgentLike } from "@/models/agents";
import {
  localIdentitiesMatch,
  type IMConversation,
  type IMMessage,
  type LocaleCode,
  type TranslateFn,
} from "@/models/conversations";
import {
  agentActivityMessageToolMergeKey,
  agentActivityToolMergeKeys,
  type AgentActivityCommand,
  type AgentActivityKind,
  parseAgentActivity,
  parseMessageActivityCommand,
  parsePlainAgentCommand,
  statusLabel,
  type AgentActivityPayload,
  type AgentActivityTool,
} from "@/models/agentActivity";
import { AgentActivityKinds, AgentActivityMsgTypes } from "@/shared/constants/messages";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import {
  Button,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";

type AgentActivityPanelProps = {
  item: AgentLike;
  locale: LocaleCode;
  rooms?: IMConversation[];
  t: TranslateFn;
};

export type AgentActivityRoomMessages = {
  messages: IMMessage[];
  room: IMConversation;
};

export type AgentActivityEntry = {
  activity: AgentActivityPayload | null;
  command: AgentActivityCommand | null;
  createdAt: string;
  id: string;
  index: number;
  kind: AgentActivityKind;
  message: IMMessage;
  roomID: string;
  roomTitle: string;
  type: "activity" | "message";
};

type AgentActivitySortMode = "chronological" | "newest_first";

type AgentActivityFilterOption = {
  count: number;
  id: string;
  label: string;
  tone: AgentActivityKind;
};

const duplicateCommandWindowMs = 10_000;

export function AgentActivityPanel({ item, locale, rooms = [], t }: AgentActivityPanelProps) {
  const queryClient = useQueryClient();
  const [sortMode, setSortMode] = useState<AgentActivitySortMode>("newest_first");
  const [selectedFilters, setSelectedFilters] = useState<Set<string>>(() => new Set());
  const [selectedEntryID, setSelectedEntryID] = useState<string | null>(null);
  const rowRefs = useRef<Map<string, HTMLElement>>(new Map());
  const latestVisibleEntryIDRef = useRef<string | null>(null);
  const liveMessagesRef = useRef<Map<string, IMMessage[]>>(new Map());
  const agentIdentity = useMemo(() => agentIdentityValues(item), [item]);
  const activityRooms = useMemo(
    () => rooms.filter((room) => room.members.some((memberID) => identityMatches(memberID, agentIdentity))),
    [agentIdentity, rooms],
  );
  const activityRoomIDSignature = useMemo(() => stableRoomIDs(activityRooms).join("\n"), [activityRooms]);
  const activityRoomIDs = useMemo(
    () => (activityRoomIDSignature ? activityRoomIDSignature.split("\n") : []),
    [activityRoomIDSignature],
  );
  const activityRoomIDSet = useMemo(() => new Set(activityRoomIDs), [activityRoomIDs]);
  const activityRoomsByID = useMemo(() => new Map(activityRooms.map((room) => [room.id, room])), [activityRooms]);
  const activityQueryKey = useMemo(
    () => ["agent-activity", item.id || "", activityRoomIDs] as const,
    [activityRoomIDs, item.id],
  );
  const query = useQuery({
    queryKey: activityQueryKey,
    queryFn: async (): Promise<AgentActivityRoomMessages[]> => {
      const pairs = await Promise.all(
        activityRooms.map(async (room) => ({
          room,
          messages: await fetchMessagesRequest(room.id, { includeThreadReplies: true }),
        })),
      );
      return pairs;
    },
    enabled: activityRooms.length > 0,
    staleTime: 2_000,
  });

  const mergeMessageIntoActivityCache = useCallback(
    (roomID: string | null | undefined, message: IMMessage | null | undefined, options?: { remember?: boolean }) => {
      const normalizedRoomID = String(roomID || "").trim();
      if (!normalizedRoomID || !message || !activityRoomIDSet.has(normalizedRoomID)) {
        return false;
      }
      const room = activityRoomsByID.get(normalizedRoomID);
      if (!room || !identityMatches(message.sender_id, agentIdentity)) {
        return false;
      }
      if (options?.remember) {
        rememberLiveActivityMessage(liveMessagesRef.current, normalizedRoomID, message);
      }
      queryClient.setQueryData<AgentActivityRoomMessages[]>(activityQueryKey, (current) =>
        current ? mergeActivityMessageIntoCache(current, room, message) : current,
      );
      return true;
    },
    [activityQueryKey, activityRoomIDSet, activityRoomsByID, agentIdentity, queryClient],
  );

  useEffect(() => {
    if (!activityRoomIDs.length) {
      return undefined;
    }
    return subscribeIMEvents((payload) => {
      if (payload.type !== "message.created" || !payload.message) {
        return;
      }
      mergeMessageIntoActivityCache(payload.room_id, payload.message, { remember: true });
    });
  }, [activityRoomIDs.length, mergeMessageIntoActivityCache]);

  useEffect(() => {
    if (!query.data) {
      return;
    }
    queryClient.setQueryData<AgentActivityRoomMessages[]>(activityQueryKey, (current) =>
      current
        ? mergeActivityMessagesIntoCache(current, activityRooms, agentIdentity, liveMessagesRef.current)
        : current,
    );
  }, [activityQueryKey, activityRooms, agentIdentity, query.data, queryClient]);

  const entries = useMemo(() => activityEntriesFromRooms(query.data ?? [], agentIdentity), [agentIdentity, query.data]);
  const filterOptions = useMemo(() => activityFilterOptions(entries, t), [entries, t]);
  const filteredEntries = useMemo(() => {
    if (selectedFilters.size === 0) {
      return entries;
    }
    return entries.filter((entry) => selectedFilters.has(activityFilterID(entry)));
  }, [entries, selectedFilters]);
  const displayEntries = useMemo(
    () => (sortMode === "newest_first" ? [...filteredEntries].reverse() : filteredEntries),
    [filteredEntries, sortMode],
  );
  const selectedFilterOptions = useMemo(
    () => filterOptions.filter((option) => selectedFilters.has(option.id)),
    [filterOptions, selectedFilters],
  );
  const summary = useMemo(() => activitySummary(entries, filteredEntries.length), [entries, filteredEntries.length]);
  const latestVisibleEntry = filteredEntries.at(-1) ?? null;
  const empty = !query.isFetching && entries.length === 0;
  const filteredEmpty = !query.isFetching && entries.length > 0 && displayEntries.length === 0;

  const scrollToEntry = useCallback((entryID: string) => {
    setSelectedEntryID(entryID);
    const scroll = () => {
      const node = rowRefs.current.get(entryID);
      if (node && "scrollIntoView" in node) {
        node.scrollIntoView({ behavior: "smooth", block: "center" });
      }
    };
    if (typeof window !== "undefined" && typeof window.requestAnimationFrame === "function") {
      window.requestAnimationFrame(scroll);
      return;
    }
    scroll();
  }, []);

  useEffect(() => {
    const latestID = latestVisibleEntry?.id ?? null;
    const previousID = latestVisibleEntryIDRef.current;
    latestVisibleEntryIDRef.current = latestID;
    if (!latestID || previousID === null || previousID === latestID) {
      return;
    }
    scrollToEntry(latestID);
  }, [latestVisibleEntry?.id, scrollToEntry]);

  const handleSortModeChange = useCallback(
    (nextMode: AgentActivitySortMode) => {
      if (nextMode === sortMode) {
        return;
      }
      setSortMode(nextMode);
      if (latestVisibleEntry) {
        scrollToEntry(latestVisibleEntry.id);
      }
    },
    [latestVisibleEntry, scrollToEntry, sortMode],
  );

  const toggleFilter = useCallback((filterID: string) => {
    setSelectedFilters((current) => {
      const next = new Set(current);
      if (next.has(filterID)) {
        next.delete(filterID);
      } else {
        next.add(filterID);
      }
      return next;
    });
  }, []);

  const clearFilters = useCallback(() => {
    setSelectedFilters(new Set());
  }, []);

  return (
    <section
      id="agent-profile-activity"
      className="profile-section agent-activity-section"
      aria-labelledby="agent-activity-title"
    >
      <h2 id="agent-activity-title" className="sr-only">
        {t("agentActivityTitle")}
      </h2>
      <div className="agent-section-form">
        <div className="agent-page-form-content agent-activity-content">
          {query.error ? (
            <div className="form-error">{errorMessage(query.error, t("agentActivityLoadFailed"))}</div>
          ) : null}
          {activityRooms.length === 0 ? <div className="agent-activity-empty">{t("agentActivityNoRooms")}</div> : null}
          {query.isFetching && entries.length === 0 ? (
            <div className="agent-activity-empty">{t("agentActivityLoading")}</div>
          ) : null}
          {empty && activityRooms.length > 0 ? (
            <div className="agent-activity-empty">{t("agentActivityEmpty")}</div>
          ) : null}
          {entries.length ? (
            <AgentActivityToolbar
              filterOptions={filterOptions}
              onClearFilters={clearFilters}
              onFilterToggle={toggleFilter}
              onRefresh={() => void query.refetch()}
              onSortModeChange={handleSortModeChange}
              refreshDisabled={query.isFetching || activityRooms.length === 0}
              selectedFilterOptions={selectedFilterOptions}
              selectedFilters={selectedFilters}
              sortMode={sortMode}
              summary={summary}
              t={t}
            />
          ) : null}
          {displayEntries.length ? (
            <AgentActivityTimeline
              entries={displayEntries}
              locale={locale}
              onEntrySelect={scrollToEntry}
              selectedEntryID={selectedEntryID}
              t={t}
            />
          ) : null}
          {filteredEmpty ? <div className="agent-activity-empty">{t("agentActivityNoFilteredResults")}</div> : null}
          {displayEntries.length ? (
            <div className="agent-activity-list" role="list">
              {displayEntries.map((entry) => (
                <AgentActivityRow
                  key={entry.id}
                  entry={entry}
                  locale={locale}
                  rowRef={(node) => {
                    if (node) {
                      rowRefs.current.set(entry.id, node);
                    } else {
                      rowRefs.current.delete(entry.id);
                    }
                  }}
                  selected={selectedEntryID === entry.id}
                  t={t}
                />
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </section>
  );
}

function stableRoomIDs(rooms: readonly IMConversation[]): string[] {
  return Array.from(new Set(rooms.map((room) => String(room.id || "").trim()).filter(Boolean))).sort();
}

function mergeActivityMessagesIntoCache(
  current: readonly AgentActivityRoomMessages[],
  rooms: readonly IMConversation[],
  agentIdentity: readonly string[],
  liveMessages: ReadonlyMap<string, readonly IMMessage[]>,
): AgentActivityRoomMessages[] {
  let next = current;
  for (const room of rooms) {
    for (const message of room.messages) {
      if (identityMatches(message.sender_id, agentIdentity)) {
        next = mergeActivityMessageIntoCache(next, room, message);
      }
    }
    for (const message of liveMessages.get(room.id) ?? []) {
      if (identityMatches(message.sender_id, agentIdentity)) {
        next = mergeActivityMessageIntoCache(next, room, message);
      }
    }
  }
  return next as AgentActivityRoomMessages[];
}

function mergeActivityMessageIntoCache(
  current: readonly AgentActivityRoomMessages[],
  room: IMConversation,
  message: IMMessage,
): AgentActivityRoomMessages[] {
  const roomIndex = current.findIndex((item) => item.room.id === room.id);
  if (roomIndex < 0) {
    return [{ room, messages: [message] }, ...current];
  }
  const roomMessages = current[roomIndex]?.messages ?? [];
  const mergedMessages = upsertActivityMessage(roomMessages, message);
  if (mergedMessages === roomMessages) {
    return current as AgentActivityRoomMessages[];
  }
  return current.map((item, index) =>
    index === roomIndex ? { room: { ...item.room, ...room }, messages: mergedMessages } : item,
  );
}

function upsertActivityMessage(messages: readonly IMMessage[], message: IMMessage): IMMessage[] {
  const nextKey = activityMessageMergeKey(message);
  const existingIndex = messages.findIndex((item) => activityMessageMergeKey(item) === nextKey);
  if (existingIndex >= 0) {
    if (activityMessagesEquivalent(messages[existingIndex], message)) {
      return messages as IMMessage[];
    }
    const next = messages.map((item, index) => (index === existingIndex ? message : item));
    return sortActivityMessages(next);
  }
  return sortActivityMessages([...messages, message]);
}

function activityMessagesEquivalent(left: IMMessage | undefined, right: IMMessage): boolean {
  if (!left) {
    return false;
  }
  if (left === right) {
    return true;
  }
  return (
    activityMessageMergeKey(left) === activityMessageMergeKey(right) &&
    String(left.content || "") === String(right.content || "") &&
    String(left.created_at || "") === String(right.created_at || "") &&
    String(left.sender_id || "") === String(right.sender_id || "") &&
    String(left.relates_to?.rel_type || "") === String(right.relates_to?.rel_type || "") &&
    String(left.relates_to?.event_id || "") === String(right.relates_to?.event_id || "") &&
    JSON.stringify(left.metadata ?? null) === JSON.stringify(right.metadata ?? null)
  );
}

function sortActivityMessages(messages: readonly IMMessage[]): IMMessage[] {
  return messages
    .map((message, index) => ({ index, message }))
    .sort((left, right) => {
      const timeDelta = activityTime(left.message.created_at || "") - activityTime(right.message.created_at || "");
      if (timeDelta !== 0) {
        return timeDelta;
      }
      return left.index - right.index;
    })
    .map((item) => item.message);
}

function activityMessageMergeKey(message: IMMessage): string {
  const id = String(message.id || "").trim();
  if (id) {
    return `id:${id}`;
  }
  return [
    "fallback",
    String(message.sender_id || ""),
    String(message.created_at || ""),
    String(message.content || ""),
    String(message.relates_to?.rel_type || ""),
    String(message.relates_to?.event_id || ""),
  ].join(":");
}

function rememberLiveActivityMessage(liveMessages: Map<string, IMMessage[]>, roomID: string, message: IMMessage): void {
  const messages = liveMessages.get(roomID) ?? [];
  const merged = upsertActivityMessage(messages, message);
  if (merged !== messages) {
    liveMessages.set(roomID, merged);
  }
}

function agentIdentityValues(item: AgentLike): string[] {
  const out: string[] = [];
  const push = (value: unknown) => {
    const id = String(value ?? "").trim();
    if (id && !out.includes(id)) {
      out.push(id);
    }
  };
  push(item.id);
  push(item.user_id);
  push(resolveAgentChannelUserID(item));
  item.participants?.forEach((participant) => {
    push(participant?.id);
    push(participant?.user_id);
    push(participant?.agent_id);
    push(participant?.channel_user_ref);
  });
  return out;
}

// eslint-disable-next-line react-refresh/only-export-components -- exported for focused activity-merge regression tests.
export function activityEntriesFromRooms(
  rooms: readonly AgentActivityRoomMessages[],
  agentIdentity: readonly string[],
): AgentActivityEntry[] {
  const entries: AgentActivityEntry[] = [];
  rooms.forEach(({ room, messages }) => {
    const pendingCommands = new Map<string, AgentActivityEntry[]>();
    const semanticCommandEntries = new Map<string, AgentActivityEntry>();
    const toolEntries = new Map<string, AgentActivityEntry>();
    messages.forEach((message) => {
      if (!identityMatches(message.sender_id, agentIdentity)) {
        return;
      }
      const activity = parseAgentActivity(message.content);
      const plainCommand = activity ? null : parseActivityCommand(message);
      const body = String(message.content || "")
        .replace(/\u200b/g, "")
        .trim();
      if (!activity && !plainCommand && !body) {
        return;
      }
      if (activity?.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
        const toolKeys = activityToolMergeKeys(activity.content.tool, message);
        const semanticCommandKey = activityToolSemanticCommandKey(activity.content.tool, message);
        const stableEntries = mappedActivityEntries(toolEntries, toolKeys);
        const semanticEntry =
          toolKeys.length === 0 && semanticCommandKey ? semanticCommandEntries.get(semanticCommandKey) : undefined;
        const existing = coalesceActivityEntries(
          entries,
          stableEntries.length ? stableEntries : semanticEntry ? [semanticEntry] : [],
          toolEntries,
          semanticCommandEntries,
        );
        if (existing) {
          mergeToolActivityIntoEntry(existing, activity, message);
          registerActivityEntry(toolEntries, toolKeys, existing);
          if (semanticCommandKey) {
            semanticCommandEntries.set(semanticCommandKey, existing);
          }
          return;
        }
        const entry: AgentActivityEntry = {
          activity,
          command: null,
          createdAt: message.created_at || "",
          id: `${room.id}:${message.id || entries.length}`,
          index: entries.length,
          kind: activityKind(activity),
          message,
          roomID: room.id,
          roomTitle: room.title || "",
          type: "activity",
        };
        entries.push(entry);
        registerActivityEntry(toolEntries, toolKeys, entry);
        if (semanticCommandKey) {
          semanticCommandEntries.set(semanticCommandKey, entry);
        }
        return;
      }
      if (plainCommand) {
        const stableToolKey = scopedActivityToolMergeKey(message, agentActivityMessageToolMergeKey(message));
        const commandKey = activityCommandMergeKey(plainCommand);
        const semanticCommandKey = openClawRequestCommandMergeKey(message, plainCommand.command);
        const stableEntry = stableToolKey ? toolEntries.get(stableToolKey) : undefined;
        const semanticEntry =
          !stableToolKey && semanticCommandKey ? semanticCommandEntries.get(semanticCommandKey) : undefined;
        const existing = stableEntry ?? semanticEntry;
        if (existing) {
          mergePlainCommandIntoEntry(existing, plainCommand, message);
          if (stableToolKey) {
            toolEntries.set(stableToolKey, existing);
          }
          return;
        }
        if (!stableToolKey && plainCommand.output) {
          const pending = pendingCommands.get(commandKey);
          const pendingIndex = pending ? pendingCommandMatchIndex(pending, message) : -1;
          const startEntry = pendingIndex >= 0 ? pending?.splice(pendingIndex, 1)[0] : undefined;
          if (startEntry?.command) {
            mergePlainCommandIntoEntry(startEntry, plainCommand, message);
            if (semanticCommandKey) {
              semanticCommandEntries.set(semanticCommandKey, startEntry);
            }
            if (pending && pending.length === 0) {
              pendingCommands.delete(commandKey);
            }
            return;
          }
        }
        const pending = pendingCommands.get(commandKey) ?? [];
        if (!plainCommand.output && pending.some((entry) => isDuplicateCommandStart(entry, plainCommand, message))) {
          return;
        }
        const entry: AgentActivityEntry = {
          activity: null,
          command: plainCommand,
          createdAt: message.created_at || "",
          id: `${room.id}:${message.id || entries.length}`,
          index: entries.length,
          kind: plainCommand.kind,
          message,
          roomID: room.id,
          roomTitle: room.title || "",
          type: "message",
        };
        entries.push(entry);
        if (stableToolKey) {
          toolEntries.set(stableToolKey, entry);
        }
        if (semanticCommandKey) {
          semanticCommandEntries.set(semanticCommandKey, entry);
        }
        if (!stableToolKey && !plainCommand.output) {
          pending.push(entry);
          pendingCommands.set(commandKey, pending);
        }
        return;
      }
      entries.push({
        activity,
        command: null,
        createdAt: message.created_at || "",
        id: `${room.id}:${message.id || entries.length}`,
        index: entries.length,
        kind: activityKind(activity),
        message,
        roomID: room.id,
        roomTitle: room.title || "",
        type: activity ? "activity" : "message",
      });
    });
  });
  return entries
    .sort((left, right) => {
      const timeDelta = activityTime(left.createdAt) - activityTime(right.createdAt);
      if (timeDelta !== 0) {
        return timeDelta;
      }
      return left.index - right.index;
    })
    .map((entry, index) => ({ ...entry, index: index + 1 }));
}

function mergeToolActivity(base: AgentActivityPayload, next: AgentActivityPayload): AgentActivityPayload {
  const baseTool = base.content.tool;
  const nextTool = next.content.tool;
  if (!baseTool || !nextTool) {
    return next;
  }
  const baseHasOutput = hasActivityToolExplicitOutput(baseTool);
  const nextHasOutput = hasActivityToolExplicitOutput(nextTool);
  const baseTerminal = isTerminalActivityTool(baseTool);
  const nextTerminal = isTerminalActivityTool(nextTool);
  return {
    ...base,
    content: {
      ...base.content,
      body: firstNonEmpty(next.content.body, base.content.body),
      tool: {
        ...baseTool,
        ...nextTool,
        command: firstNonEmpty(nextTool.command, baseTool.command),
        cwd: firstNonEmpty(nextTool.cwd, baseTool.cwd),
        duration_ms: nextTool.duration_ms ?? baseTool.duration_ms,
        exit_code: nextTool.exit_code ?? baseTool.exit_code,
        input: nextTool.input ?? baseTool.input,
        input_summary: firstNonEmpty(nextTool.input_summary, baseTool.input_summary),
        item_id: firstNonEmpty(nextTool.item_id, baseTool.item_id),
        output: nextTool.output ?? baseTool.output,
        output_summary: nextHasOutput
          ? firstNonEmpty(nextTool.output_summary, baseTool.output_summary)
          : baseHasOutput
            ? firstNonEmpty(baseTool.output_summary, nextTool.output_summary)
            : firstNonEmpty(nextTool.output_summary, baseTool.output_summary),
        phase: baseTerminal && !nextTerminal ? baseTool.phase : firstNonEmpty(nextTool.phase, baseTool.phase),
        status: baseTerminal && !nextTerminal ? baseTool.status : firstNonEmpty(nextTool.status, baseTool.status),
        tool_call_id: firstNonEmpty(nextTool.tool_call_id, baseTool.tool_call_id),
      },
    },
  };
}

function mergeToolActivityIntoEntry(
  entry: AgentActivityEntry,
  activity: AgentActivityPayload,
  message: IMMessage,
): void {
  const previousCommand = entry.command;
  const previousMessage = entry.message;
  entry.message = message;
  if (entry.activity) {
    entry.activity = mergeToolActivity(entry.activity, activity);
  } else {
    entry.activity = activity;
    entry.command = null;
    entry.kind = activityKind(activity);
    entry.type = "activity";
  }
  if (previousCommand) {
    mergePlainCommandIntoEntry(entry, previousCommand, previousMessage);
  }
}

function mappedActivityEntries(
  entries: ReadonlyMap<string, AgentActivityEntry>,
  keys: readonly string[],
): AgentActivityEntry[] {
  const matches: AgentActivityEntry[] = [];
  for (const key of keys) {
    const entry = entries.get(key);
    if (entry && !matches.includes(entry)) {
      matches.push(entry);
    }
  }
  return matches;
}

function activityToolMergeKeys(tool: AgentActivityTool, message: IMMessage): string[] {
  return agentActivityToolMergeKeys(tool).map((key) => scopedActivityToolMergeKey(message, key));
}

function scopedActivityToolMergeKey(message: IMMessage, key: string): string {
  const requestID = openClawRequestID(message);
  return requestID && key ? `openclaw-request:${requestID}:${key}` : key;
}

function registerActivityEntry(
  entries: Map<string, AgentActivityEntry>,
  keys: readonly string[],
  entry: AgentActivityEntry,
): void {
  keys.forEach((key) => entries.set(key, entry));
}

function coalesceActivityEntries(
  entries: AgentActivityEntry[],
  matches: readonly AgentActivityEntry[],
  toolEntries: Map<string, AgentActivityEntry>,
  semanticCommandEntries: Map<string, AgentActivityEntry>,
): AgentActivityEntry | undefined {
  if (matches.length === 0) {
    return undefined;
  }
  const ordered = [...matches].sort((left, right) => entries.indexOf(left) - entries.indexOf(right));
  const target = ordered[0];
  if (!target) {
    return undefined;
  }
  for (const duplicate of ordered.slice(1)) {
    mergeActivityEntryIntoEntry(target, duplicate);
    const duplicateIndex = entries.indexOf(duplicate);
    if (duplicateIndex >= 0) {
      entries.splice(duplicateIndex, 1);
    }
    replaceMappedActivityEntry(toolEntries, duplicate, target);
    replaceMappedActivityEntry(semanticCommandEntries, duplicate, target);
  }
  return target;
}

function mergeActivityEntryIntoEntry(target: AgentActivityEntry, source: AgentActivityEntry): void {
  if (source.activity) {
    mergeToolActivityIntoEntry(target, source.activity, source.message);
  }
  if (source.command) {
    mergePlainCommandIntoEntry(target, source.command, source.message);
  }
}

function replaceMappedActivityEntry(
  entries: Map<string, AgentActivityEntry>,
  previous: AgentActivityEntry,
  next: AgentActivityEntry,
): void {
  entries.forEach((entry, key) => {
    if (entry === previous) {
      entries.set(key, next);
    }
  });
}

function hasActivityToolExplicitOutput(tool: AgentActivityTool): boolean {
  return tool.output !== undefined;
}

function isTerminalActivityTool(tool: AgentActivityTool): boolean {
  const phase = String(tool.phase || "")
    .trim()
    .toLowerCase();
  const status = String(tool.status || "")
    .trim()
    .toLowerCase();
  return (
    phase === "end" ||
    ["completed", "done", "failed", "blocked", "canceled", "cancelled", "success", "succeeded"].includes(status)
  );
}

function mergePlainCommandIntoEntry(
  entry: AgentActivityEntry,
  command: AgentActivityCommand,
  message: IMMessage,
): void {
  const normalizedCommand = normalizeCommandText(command.command);
  const output = firstNonEmpty(command.output);
  if (entry.activity?.content.msgtype === AgentActivityMsgTypes.tool && entry.activity.content.tool) {
    const tool = entry.activity.content.tool;
    entry.activity = {
      ...entry.activity,
      content: {
        ...entry.activity.content,
        body: firstNonEmpty(output, entry.activity.content.body),
        tool: {
          ...tool,
          command: firstNonEmpty(tool.command, normalizedCommand),
          output: output || tool.output,
          output_summary: firstNonEmpty(output, tool.output_summary),
          phase: output ? "end" : tool.phase,
          status: output ? "completed" : tool.status,
        },
      },
    };
    if (output) {
      entry.message = message;
    }
    return;
  }
  if (entry.command) {
    entry.command = {
      ...entry.command,
      command: firstNonEmpty(entry.command.command, normalizedCommand),
      output: firstNonEmpty(output, entry.command.output) || undefined,
    };
    if (output) {
      entry.message = message;
    }
  }
}

function activityToolSemanticCommandKey(tool: AgentActivityTool, message: IMMessage): string {
  return openClawRequestCommandMergeKey(message, activityToolCommandForMerge(tool));
}

function activityToolCommandForMerge(tool: AgentActivityTool): string {
  const kind = normalizeCommandSignature(tool.kind || "");
  return normalizeCommandText(
    firstNonEmpty(
      summaryValue(tool.input_summary, "command", "cmd"),
      summaryValue(tool.input, "command", "cmd"),
      summaryText(tool.input_summary),
      summaryText(tool.input),
      tool.command,
      toolTitleCommandForMerge(tool.title, kind),
    ),
  );
}

function toolTitleCommandForMerge(title: string, kind: string): string {
  const text = title.trim();
  if (!text) {
    return "";
  }
  if ((kind === "command" || kind === "exec" || kind === "bash") && text.toLowerCase().startsWith(`${kind} `)) {
    return text.slice(kind.length + 1).trim();
  }
  return /^command\s+/i.test(text) ? text : "";
}

function parseActivityCommand(message: IMMessage): AgentActivityCommand | null {
  const command = parseMessageActivityCommand(message) ?? parsePlainOpenClawProgressCommand(message.content);
  return command ? normalizeActivityCommand(command) : null;
}

function parsePlainOpenClawProgressCommand(content: unknown): AgentActivityCommand | null {
  if (typeof content !== "string" || !isOpenClawProgressMarker(content)) {
    return null;
  }
  return parsePlainAgentCommand(content);
}

function isOpenClawProgressMarker(content: string): boolean {
  return /^(?:🛠️?|🔎|📄|📖|🧠|✍️?|🩹|📩)\s*/u.test(content.replace(/\u200b/g, "").trim());
}

function normalizeActivityCommand(command: AgentActivityCommand): AgentActivityCommand {
  const normalizedCommand = normalizeCommandText(command.command);
  if (normalizedCommand === command.command) {
    return command;
  }
  return {
    ...command,
    command: normalizedCommand,
    signature: command.signature.startsWith("openclaw-tool:")
      ? command.signature
      : normalizeCommandSignature(normalizedCommand),
    title: commandTitleFromText(normalizedCommand),
  };
}

function isDuplicateCommandStart(
  entry: AgentActivityEntry,
  command: AgentActivityCommand,
  message: IMMessage,
): boolean {
  if (!entry.command || entry.command.output || command.output) {
    return false;
  }
  if (activityCommandMergeKey(entry.command) !== activityCommandMergeKey(command)) {
    return false;
  }
  const entryRequestID = openClawRequestID(entry.message);
  const messageRequestID = openClawRequestID(message);
  if (entryRequestID && messageRequestID) {
    return entryRequestID === messageRequestID;
  }
  return Math.abs(activityTime(entry.createdAt) - activityTime(message.created_at || "")) <= duplicateCommandWindowMs;
}

function activityCommandMergeKey(command: AgentActivityCommand): string {
  return command.signature.startsWith("openclaw-tool:")
    ? normalizeCommandSignature(command.command)
    : normalizeCommandSignature(command.signature || command.command);
}

function normalizeCommandText(command: string): string {
  return command.replace(/^command\s+(.+)$/i, "$1").trim();
}

function normalizeCommandSignature(command: string): string {
  return normalizeCommandText(command).replace(/\s+/g, " ").trim().toLowerCase();
}

function pendingCommandMatchIndex(pending: readonly AgentActivityEntry[], message: IMMessage): number {
  if (pending.length === 0) {
    return -1;
  }
  const messageRequestID = openClawRequestID(message);
  if (messageRequestID) {
    const requestIndex = pending.findIndex((entry) => openClawRequestID(entry.message) === messageRequestID);
    if (requestIndex >= 0) {
      return requestIndex;
    }
  }
  return 0;
}

function openClawRequestCommandMergeKey(message: IMMessage, command: string): string {
  const requestID = openClawRequestID(message);
  const commandKey = normalizeCommandSignature(command);
  return requestID && commandKey ? `openclaw-request:${requestID}:command:${commandKey}` : "";
}

function openClawRequestID(message: IMMessage | null | undefined): string {
  const metadata = isRecord(message?.metadata) ? message.metadata : undefined;
  const openclaw = isRecord(metadata?.openclaw) ? metadata.openclaw : undefined;
  const deliveryInfo = isRecord(openclaw?.delivery_info)
    ? openclaw.delivery_info
    : isRecord(openclaw?.deliveryInfo)
      ? openclaw.deliveryInfo
      : undefined;
  return firstNonEmpty(
    openclaw?.request_id,
    openclaw?.requestId,
    openclaw?.source_message_id,
    openclaw?.sourceMessageId,
    deliveryInfo?.request_id,
    deliveryInfo?.requestId,
  );
}

function commandTitleFromText(command: string): string {
  const colon = command.indexOf(":");
  if (colon > 0 && colon <= 32) {
    return command.slice(0, colon).trim();
  }
  const words = command.split(/\s+/).slice(0, 2).join(" ").trim();
  return words || AgentActivityKinds.execCommand;
}

function activityKind(activity: AgentActivityPayload | null): AgentActivityKind {
  if (activity?.content.msgtype === AgentActivityMsgTypes.tool) {
    return AgentActivityKinds.execCommand;
  }
  if (activity) {
    return AgentActivityKinds.other;
  }
  return AgentActivityKinds.message;
}

function identityMatches(value: string | null | undefined, candidates: readonly string[]): boolean {
  const id = String(value || "").trim();
  return Boolean(id && candidates.some((candidate) => candidate === id || localIdentitiesMatch(candidate, id)));
}

function AgentActivityToolbar({
  filterOptions,
  onClearFilters,
  onFilterToggle,
  onRefresh,
  onSortModeChange,
  refreshDisabled,
  selectedFilterOptions,
  selectedFilters,
  sortMode,
  summary,
  t,
}: {
  filterOptions: readonly AgentActivityFilterOption[];
  onClearFilters: () => void;
  onFilterToggle: (filterID: string) => void;
  onRefresh: () => void;
  onSortModeChange: (mode: AgentActivitySortMode) => void;
  refreshDisabled: boolean;
  selectedFilterOptions: readonly AgentActivityFilterOption[];
  selectedFilters: ReadonlySet<string>;
  sortMode: AgentActivitySortMode;
  summary: AgentActivitySummary;
  t: TranslateFn;
}) {
  return (
    <div className="agent-activity-toolbar">
      <div className="agent-activity-meta-row" aria-label={t("agentActivitySummaryLabel")}>
        {summary.duration ? (
          <ActivityMetaChip icon={<Clock aria-hidden="true" size={14} strokeWidth={2} />}>
            {summary.duration}
          </ActivityMetaChip>
        ) : null}
        <ActivityMetaChip>{t("agentActivityToolCallsCount", { count: summary.toolCount })}</ActivityMetaChip>
        <ActivityMetaChip>
          {selectedFilters.size
            ? t("agentActivityFilteredEventsCount", { shown: summary.filteredCount, total: summary.eventCount })
            : t("agentActivityEventsCount", { count: summary.eventCount })}
        </ActivityMetaChip>
      </div>
      <div className="agent-activity-controls">
        <div className="agent-activity-sort" role="group" aria-label={t("agentActivitySortLabel")}>
          <button
            type="button"
            className={classNames("agent-activity-sort-button", sortMode === "chronological" && "active")}
            aria-pressed={sortMode === "chronological"}
            title={t("agentActivityChronological")}
            onClick={() => onSortModeChange("chronological")}
          >
            <ArrowDownNarrowWide aria-hidden="true" size={15} strokeWidth={2} />
            <span>{t("agentActivityChronological")}</span>
          </button>
          <button
            type="button"
            className={classNames("agent-activity-sort-button", sortMode === "newest_first" && "active")}
            aria-pressed={sortMode === "newest_first"}
            title={t("agentActivityNewestFirst")}
            onClick={() => onSortModeChange("newest_first")}
          >
            <ArrowUpNarrowWide aria-hidden="true" size={15} strokeWidth={2} />
            <span>{t("agentActivityNewestFirst")}</span>
          </button>
        </div>
        <DropdownMenuRoot>
          <DropdownMenuTrigger asChild>
            <Button
              className={classNames("agent-activity-filter-trigger", selectedFilters.size > 0 && "active")}
              variant="secondaryGray"
              size="sm"
            >
              <Filter aria-hidden="true" size={15} strokeWidth={2} />
              <span>{t("agentActivityFilter")}</span>
              {selectedFilters.size ? (
                <span
                  className="agent-activity-filter-count"
                  aria-label={t("agentActivityFilterActiveCount", { count: selectedFilters.size })}
                >
                  {selectedFilters.size}
                </span>
              ) : null}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="agent-activity-filter-menu" aria-label={t("agentActivityFilter")}>
            {filterOptions.map((option) => (
              <DropdownMenuCheckboxItem
                key={option.id}
                aria-label={`${option.label} (${option.count})`}
                checked={selectedFilters.has(option.id)}
                onCheckedChange={() => onFilterToggle(option.id)}
              >
                <span className={classNames("agent-activity-filter-dot", option.tone)} aria-hidden="true" />
                <span className="agent-activity-filter-label">{option.label}</span>
                <span className="agent-activity-filter-option-count">{option.count}</span>
              </DropdownMenuCheckboxItem>
            ))}
            {selectedFilters.size > 0 ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={onClearFilters}>{t("agentActivityClearFilters")}</DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenuRoot>
        <Button
          className="agent-activity-toolbar-refresh"
          variant="secondaryGray"
          size="sm"
          disabled={refreshDisabled}
          aria-label={t("agentActivityRefresh")}
          title={t("agentActivityRefresh")}
          onClick={onRefresh}
        >
          <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
        </Button>
      </div>
      {selectedFilterOptions.length ? (
        <div className="agent-activity-selected-filters" aria-label={t("agentActivitySelectedFiltersLabel")}>
          {selectedFilterOptions.map((option) => (
            <button
              key={option.id}
              type="button"
              className={classNames("agent-activity-filter-chip", option.tone)}
              aria-label={t("agentActivityRemoveFilter", { label: option.label })}
              onClick={() => onFilterToggle(option.id)}
            >
              <span>{option.label}</span>
              <span aria-hidden="true">×</span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ActivityMetaChip({ children, icon }: { children: ReactNode; icon?: ReactNode }) {
  return (
    <span className="agent-activity-meta-chip">
      {icon}
      <span>{children}</span>
    </span>
  );
}

function AgentActivityTimeline({
  entries,
  locale,
  onEntrySelect,
  selectedEntryID,
  t,
}: {
  entries: readonly AgentActivityEntry[];
  locale: LocaleCode;
  onEntrySelect: (entryID: string) => void;
  selectedEntryID: string | null;
  t: TranslateFn;
}) {
  return (
    <div className="agent-activity-timeline-wrap">
      <div className="agent-activity-timeline" role="navigation" aria-label={t("agentActivityTimelineLabel")}>
        {entries.map((entry) => (
          <button
            key={entry.id}
            type="button"
            className={classNames(
              "agent-activity-timeline-segment",
              entry.kind,
              selectedEntryID === entry.id && "selected",
            )}
            aria-label={t("agentActivityTimelineSegment", {
              index: entry.index,
              label: activityEntryLabel(entry, t),
              time: formatActivityTime(entry.createdAt, locale),
            })}
            title={`${activityEntryLabel(entry, t)} #${entry.index} ${formatActivityTime(entry.createdAt, locale)}`}
            onClick={() => onEntrySelect(entry.id)}
          />
        ))}
      </div>
    </div>
  );
}

type AgentActivitySummary = {
  duration: string;
  eventCount: number;
  filteredCount: number;
  toolCount: number;
};

function activitySummary(entries: readonly AgentActivityEntry[], filteredCount: number): AgentActivitySummary {
  const first = entries.find((entry) => activityTime(entry.createdAt) > 0);
  const last = [...entries].reverse().find((entry) => activityTime(entry.createdAt) > 0);
  const duration =
    first && last ? formatActivityDuration(activityTime(first.createdAt), activityTime(last.createdAt)) : "";
  return {
    duration,
    eventCount: entries.length,
    filteredCount,
    toolCount: entries.filter((entry) => entry.kind === AgentActivityKinds.execCommand).length,
  };
}

function activityFilterOptions(entries: readonly AgentActivityEntry[], t: TranslateFn): AgentActivityFilterOption[] {
  const options = new Map<string, AgentActivityFilterOption>();
  for (const entry of entries) {
    const id = activityFilterID(entry);
    const existing = options.get(id);
    if (existing) {
      existing.count += 1;
      continue;
    }
    options.set(id, {
      count: 1,
      id,
      label: activityFilterLabel(entry, t),
      tone: entry.kind,
    });
  }
  return Array.from(options.values()).sort((left, right) => {
    const rankDelta = activityFilterRank(left) - activityFilterRank(right);
    if (rankDelta !== 0) {
      return rankDelta;
    }
    return left.label.localeCompare(right.label);
  });
}

function activityFilterRank(option: AgentActivityFilterOption): number {
  if (option.id === AgentActivityKinds.message) {
    return 0;
  }
  if (option.id.startsWith(`${AgentActivityKinds.execCommand}:`)) {
    return 1;
  }
  return 2;
}

function activityFilterID(entry: AgentActivityEntry): string {
  if (entry.kind === AgentActivityKinds.message) {
    return AgentActivityKinds.message;
  }
  if (entry.kind === AgentActivityKinds.execCommand) {
    return `${AgentActivityKinds.execCommand}:${normalizeFilterValue(activityToolLabel(entry))}`;
  }
  return AgentActivityKinds.other;
}

function activityFilterLabel(entry: AgentActivityEntry, t: TranslateFn): string {
  if (entry.kind === AgentActivityKinds.message) {
    return t("agentActivityMessageFilter");
  }
  if (entry.kind === AgentActivityKinds.execCommand) {
    return `${t("agentActivityTool")}:${activityToolLabel(entry)}`;
  }
  return t("agentActivityOtherFilter");
}

function activityEntryLabel(entry: AgentActivityEntry, t: TranslateFn): string {
  if (entry.kind === AgentActivityKinds.message) {
    return t("agentActivityMessageFilter");
  }
  if (entry.kind === AgentActivityKinds.execCommand) {
    return `${t("agentActivityTool")}:${activityToolLabel(entry)}`;
  }
  if (entry.activity?.content.msgtype === AgentActivityMsgTypes.action) {
    return t("agentActivityAction");
  }
  return t("agentActivityOtherFilter");
}

function activityToolLabel(entry: AgentActivityEntry): string {
  if (entry.command) {
    return entry.command.title || entry.command.kind;
  }
  const tool = entry.activity?.content.tool;
  return firstNonEmpty(tool?.kind, tool?.title, tool?.id, AgentActivityKinds.execCommand);
}

function normalizeFilterValue(value: string): string {
  return value.replace(/\s+/g, " ").trim().toLowerCase();
}

function activityTime(value: string): number {
  const ts = Date.parse(value);
  return Number.isFinite(ts) ? ts : 0;
}

function formatActivityDuration(start: number, end: number): string {
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) {
    return `${minutes}m ${remainingSeconds}s`;
  }
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `${hours}h ${remainingMinutes}m`;
}

function formatActivityTime(value: string, locale: LocaleCode): string {
  const ts = activityTime(value);
  if (!ts) {
    return "";
  }
  return new Intl.DateTimeFormat(locale.startsWith("zh") ? "zh-CN" : "en-US", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(ts));
}

function AgentActivityRow({
  entry,
  locale,
  rowRef,
  selected,
  t,
}: {
  entry: AgentActivityEntry;
  locale: LocaleCode;
  rowRef?: (node: HTMLElement | null) => void;
  selected: boolean;
  t: TranslateFn;
}) {
  const row = activityRowView(entry, t);
  const [expanded, setExpanded] = useState(row.defaultExpanded);
  const hasDetail = Boolean(row.detail);

  return (
    <article
      ref={rowRef}
      className={classNames("agent-activity-row", row.tone, selected && "selected")}
      role="listitem"
    >
      <div className="agent-activity-row-main">
        <span className={`agent-activity-type-badge ${row.tone}`}>{row.label}</span>
        {hasDetail ? (
          <button
            type="button"
            className="agent-activity-row-summary expandable"
            aria-expanded={expanded}
            onClick={() => setExpanded((value) => !value)}
          >
            <ChevronRight className={`agent-activity-row-chevron ${expanded ? "expanded" : ""}`} size={14} />
            <span className="agent-activity-row-text">{row.summary || "-"}</span>
          </button>
        ) : (
          <div className="agent-activity-row-summary">
            <span className="agent-activity-row-text">{row.summary || "-"}</span>
          </div>
        )}
        <span className="agent-activity-row-room" title={entry.roomTitle || entry.roomID}>
          {entry.roomTitle || entry.roomID}
        </span>
        <span className="agent-activity-row-seq">#{entry.index}</span>
        <time className="agent-activity-row-time" dateTime={entry.createdAt}>
          {formatActivityTime(entry.createdAt, locale)}
        </time>
      </div>
      {hasDetail && expanded ? <div className="agent-activity-row-detail">{row.detail}</div> : null}
    </article>
  );
}

function activityRowView(entry: AgentActivityEntry, t: TranslateFn) {
  if (entry.command) {
    return commandRowView(entry.command, t);
  }

  const activity = entry.activity;
  if (activity?.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
    return toolRowView(activity.content.tool, activity.content.body, t);
  }

  if (activity?.content.msgtype === AgentActivityMsgTypes.action) {
    return {
      defaultExpanded: false,
      detail: <ActivityMessageDetail message={entry.message} t={t} />,
      label: AgentActivityKinds.other,
      summary: activity.content.action
        ? `${activity.content.action.title} · ${statusLabel(activity.content.action.status)}`
        : activity.content.body,
      tone: AgentActivityKinds.other,
    };
  }

  if (activity) {
    return {
      defaultExpanded: false,
      detail: activity.content.body ? <PlainDetail value={activity.content.body} /> : null,
      label: AgentActivityKinds.other,
      summary: truncateText(activity.content.body, 180),
      tone: AgentActivityKinds.other,
    };
  }

  const messageText = String(entry.message.content || "")
    .replace(/\u200b/g, "")
    .trim();

  return {
    defaultExpanded: false,
    detail: messageText ? <ActivityMessageDetail message={entry.message} t={t} /> : null,
    label: AgentActivityKinds.message,
    summary: summarizeReply(entry.message.content),
    tone: AgentActivityKinds.message,
  };
}

function commandRowView(command: AgentActivityCommand, t: TranslateFn) {
  const detailSections = [
    { label: t("agentActivityCommand"), value: command.command },
    command.output ? { label: t("agentActivityResult"), value: command.output } : null,
  ].filter(isDetailSection);
  return {
    defaultExpanded: false,
    detail: detailSections.length ? <ToolDetail sections={detailSections} /> : null,
    label: command.kind,
    summary: command.command,
    tone: command.kind,
  };
}

function toolRowView(tool: AgentActivityTool, fallbackBody: string, t: TranslateFn) {
  const command = toolCommandSummary(tool);
  const output = toolOutputSummary(tool);
  const input = toolInputSummary(tool);
  const detailSections = [
    command ? { label: t("agentActivityCommand"), value: command } : null,
    !command && input ? { label: t("agentActivityInput"), value: input } : null,
    output ? { label: t("agentActivityResult"), value: output } : null,
  ].filter(isDetailSection);
  return {
    defaultExpanded: false,
    detail: detailSections.length ? <ToolDetail sections={detailSections} /> : null,
    label: AgentActivityKinds.execCommand,
    summary: command || input || output || tool.title || fallbackBody,
    tone: AgentActivityKinds.execCommand,
  };
}

type DetailSection = {
  label: string;
  value: string;
};

function isDetailSection(value: DetailSection | null): value is DetailSection {
  return value !== null && value.value.trim() !== "";
}

function ToolDetail({ sections }: { sections: readonly DetailSection[] }) {
  return (
    <div className="agent-activity-tool-detail">
      {sections.map((section) => (
        <div key={section.label} className="agent-activity-tool-section">
          <div className="agent-activity-tool-section-label">{section.label}</div>
          <pre>{section.value}</pre>
        </div>
      ))}
    </div>
  );
}

function PlainDetail({ value }: { value: string }) {
  return <pre className="agent-activity-plain-detail">{value}</pre>;
}

function ActivityMessageDetail({ message, t }: { message: IMMessage; t: TranslateFn }) {
  return (
    <MessageContent
      content={message.content}
      message={message}
      actionBusy=""
      actionError={{ key: "", message: "" }}
      onAction={() => undefined}
      t={t}
    />
  );
}

function toolCommandSummary(tool: AgentActivityTool): string {
  return normalizeCommandText(
    firstNonEmpty(
      summaryValue(tool.input_summary, "command", "cmd"),
      summaryValue(tool.input, "command", "cmd"),
      summaryText(tool.input_summary),
      summaryText(tool.input),
      tool.command,
      toolTitleCommandForMerge(tool.title, normalizeCommandSignature(tool.kind || "")),
    ),
  );
}

function toolInputSummary(tool: AgentActivityTool): string {
  return firstNonEmpty(
    summaryValue(
      tool.input_summary,
      "input",
      "query",
      "path",
      "file",
      "filename",
      "pattern",
      "description",
      "prompt",
      "arguments",
      "args",
      "params",
    ),
    summaryValue(
      tool.input,
      "input",
      "query",
      "path",
      "file",
      "filename",
      "pattern",
      "description",
      "prompt",
      "arguments",
      "args",
      "params",
    ),
    summaryText(tool.input_summary),
    summaryText(tool.input),
  );
}

function toolOutputSummary(tool: AgentActivityTool): string {
  const output = firstNonEmpty(
    summaryValue(tool.output_summary, "output", "result", "stdout", "stderr", "error"),
    summaryValue(tool.output, "output", "result", "stdout", "stderr", "error"),
    summaryText(tool.output_summary),
    summaryText(tool.output),
  );
  if (output) {
    return output;
  }
  const details: string[] = [];
  if (tool.status && tool.status !== "running") {
    details.push(`status=${tool.status}`);
  }
  if (tool.exit_code !== undefined) {
    details.push(`exitCode=${tool.exit_code === null ? "null" : tool.exit_code}`);
  }
  if (tool.duration_ms !== undefined) {
    details.push(`durationMs=${tool.duration_ms}`);
  }
  if (tool.cwd) {
    details.push(`cwd=${tool.cwd}`);
  }
  return details.join("\n");
}

function summaryValue(value: unknown, ...keys: string[]): string {
  const decoded = decodeSummary(value);
  if (isRecord(decoded)) {
    for (const key of keys) {
      const text = summaryText(decoded[key]);
      if (text) {
        return text;
      }
    }
    return "";
  }
  return "";
}

function summaryText(value: unknown): string {
  const decoded = decodeSummary(value);
  if (typeof decoded === "string") {
    return decoded.trim();
  }
  if (typeof decoded === "number" || typeof decoded === "boolean") {
    return String(decoded);
  }
  if (decoded === null || decoded === undefined) {
    return "";
  }
  try {
    return JSON.stringify(decoded, null, 2);
  } catch {
    return "";
  }
}

function decodeSummary(value: unknown): unknown {
  if (typeof value !== "string") {
    return value;
  }
  const text = value.trim();
  if (!text) {
    return "";
  }
  if (!text.startsWith("{") && !text.startsWith("[")) {
    return text;
  }
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function summarizeReply(content: unknown): string {
  const text = plainText(content);
  return truncateText(text, 180);
}

function plainText(content: unknown): string {
  return String(content || "")
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/!\[[^\]]*]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)]\([^)]*\)/g, "$1")
    .replace(/[#>*_\-|]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function truncateText(value: string, maxLength: number): string {
  const text = value.trim();
  if (text.length <= maxLength) {
    return text;
  }
  return `${text.slice(0, Math.max(0, maxLength - 1)).trimEnd()}...`;
}

function firstNonEmpty(...values: unknown[]): string {
  for (const value of values) {
    const text = typeof value === "string" ? value.trim() : summaryText(value);
    if (text) {
      return text;
    }
  }
  return "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
