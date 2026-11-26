import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  Archive,
  ChevronDown,
  Filter,
  Folder,
  Funnel,
  ListFilter,
  MoreHorizontal,
  Pin,
  PinOff,
  Search,
  Users,
  X,
} from "lucide-react";
import {
  Button,
  Tooltip,
  PopoverRoot,
  PopoverTrigger,
  PopoverContent,
  Checkbox,
  TextInput,
  DialogRoot,
  DialogHeader,
  DialogBody,
  DialogFooter,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { CollaborationModeBadge } from "@/components/business/CollaborationModeBadge";
import { RoomAvatar } from "@/components/business/RoomAvatar";
import { useTaskConversationPages } from "@/hooks/workspace/useTaskConversationPages";
import { useCloudAgentAvatarMetadata } from "@/hooks/workspace/useCloudAgentAvatarMetadata";
import { cloudAgentAvatar } from "@/models/cloudAgents";
import type { AuthStatus } from "@/models/auth";
import { errorMessage } from "@/api/client";
import { fetchTaskConversationFilters } from "@/api/taskConversations";
import { taskConversationPreview, uniqueTaskConversations } from "@/models/taskConversations";
import { resolveConversationUser } from "@/models/conversations";
import type { IMConversation, IMMessage, TranslateFn, UsersById } from "@/models/conversations";
import type { TaskEntryController } from "@/hooks/workspace/useTaskEntry";
import { useArchiveConfirmation } from "@/hooks/workspace/useArchiveConfirmation";
import { MANAGER_AGENT_ID, MANAGER_PARTICIPANT_ID } from "@/shared/constants/agents";
import { localizeAPIError } from "@/shared/i18n";
import {
  collaborationConversationOwner,
  conversationFilterOptions,
  emptyConversationFilters,
  type ConversationFilters,
} from "@/models/conversationAgentFilter";
import styles from "./TaskConversationList.module.css";

function FilterRow({
  label,
  icon,
  count,
  selected,
  onSelect,
  title,
}: {
  label: ReactNode;
  icon: ReactNode;
  count: number;
  selected: boolean;
  onSelect: () => void;
  title?: string;
}) {
  return (
    <button
      type="button"
      className={styles.filterOption}
      aria-pressed={selected}
      data-empty={count === 0}
      onClick={onSelect}
      title={title}
    >
      <span className={styles.filterOptionIcon} aria-hidden="true">
        {icon}
      </span>
      <span className={styles.filterOptionName}>{label}</span> <span className={styles.filterOptionCount}>{count}</span>
    </button>
  );
}

function highlightedSearchText(text: string, query: string) {
  const terms = Array.from(new Set(query.trim().split(/\s+/).filter(Boolean))).sort((a, b) => b.length - a.length);
  if (!terms.length) return text;
  const escaped = terms.map((term) => term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  const pattern = new RegExp(`(${escaped.join("|")})`, "gi");
  const matches = new Set(terms.map((term) => term.toLocaleLowerCase()));
  return text.split(pattern).map((part, index) =>
    matches.has(part.toLocaleLowerCase()) ? (
      <mark key={`${part}-${index}`} className={styles.searchHighlight}>
        {part}
      </mark>
    ) : (
      part
    ),
  );
}

function searchExcerpt(text: string, query: string, maxLength = 64, leadingContext = 4) {
  const terms = Array.from(new Set(query.trim().split(/\s+/).filter(Boolean)));
  const lowerText = text.toLocaleLowerCase();
  const ranges = terms
    .map((term) => {
      const start = lowerText.indexOf(term.toLocaleLowerCase());
      return start < 0 ? null : { start, end: start + term.length };
    })
    .filter((range): range is { start: number; end: number } => range !== null);
  if (!ranges.length) return text.length > maxLength ? `${text.slice(0, maxLength)}…` : text;

  const firstMatch = Math.min(...ranges.map((range) => range.start));
  const lastMatch = Math.max(...ranges.map((range) => range.end));
  const context = Array.from(text.slice(0, firstMatch)).slice(-leadingContext).join("");
  let start = firstMatch - context.length;
  if (lastMatch - start > maxLength) start = Math.max(0, lastMatch - maxLength);
  const end = Math.min(text.length, start + maxLength);
  return `${start > 0 ? "…" : ""}${text.slice(start, end)}${end < text.length ? "…" : ""}`;
}

type Props = {
  authStatus?: AuthStatus;
  archived?: boolean;
  compact?: boolean;
  collapsed?: boolean;
  onExpand?: () => void;
  filters?: ConversationFilters;
  onFiltersChange?: (filters: ConversationFilters) => void;
  directoriesByPeer?: ReadonlyMap<string, string>;
  entry: TaskEntryController;
  activeID?: string;
  currentUserID: string;
  usersById: UsersById;
  t: TranslateFn;
  onSelect: (id: string, match?: IMMessage | null) => void;
  onArchiveCurrent?: () => void;
  onNewWith?: (room: IMConversation) => void;
  onEditAgent?: (room: IMConversation) => void;
};

const noDirectories = new Map<string, string>();

export function TaskConversationList({
  authStatus,
  archived = false,
  compact = false,
  collapsed = false,
  onExpand,
  filters,
  onFiltersChange,
  directoriesByPeer = noDirectories,
  entry,
  activeID,
  currentUserID,
  usersById,
  t,
  onSelect,
  onArchiveCurrent,
  onNewWith,
  onEditAgent,
}: Props) {
  const [localSearch, setLocalSearch] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);
  if (collapsed && filterOpen) setFilterOpen(false);
  const { owner, directory, search } = filters ?? { ...emptyConversationFilters, search: localSearch };
  const filterCount = Number(Boolean(owner)) + Number(Boolean(directory));
  const filtering = filterCount > 0;
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState(search);
  function setSearch(value: string) {
    if (filters && onFiltersChange) onFiltersChange({ ...filters, search: value });
    else setLocalSearch(value);
  }
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<{ kind: "rename" | "archive" | "delete"; room: IMConversation } | null>(null);
  const [title, setTitle] = useState("");
  const [dontAsk, setDontAsk] = useState(false);
  const [skipArchive, setSkipArchive] = useArchiveConfirmation();
  useEffect(() => {
    const timer = window.setTimeout(() => setQuery(search), 250);
    return () => window.clearTimeout(timer);
  }, [search]);
  const legacyPeers = directory
    ? Array.from(directoriesByPeer, ([peer, path]) => (path === directory ? peer : ""))
        .filter(Boolean)
        .sort()
    : [];
  const list = useTaskConversationPages(
    archived,
    query,
    true,
    filtering ? { owner, directory, legacyPeers } : undefined,
  );
  const deletingID = dialog?.kind === "delete" && entry.busyID === dialog.room.id ? dialog.room.id : "";
  const rooms = uniqueTaskConversations(list.data?.pages ?? []).filter((room) => room.id !== deletingID);
  const cloudCatalog = useCloudAgentAvatarMetadata(authStatus);
  const filterGroups = useQuery({
    queryKey: ["task-conversations", archived, "", "filters"],
    enabled: filterOpen,
    queryFn: () => fetchTaskConversationFilters(archived),
  });
  const options = useMemo(() => {
    const groups = (filterGroups.data ?? []).map((group) => ({
      members: group.peer ? [group.peer] : [],
      is_direct: group.is_direct,
      cloud_agent: group.cloud_user !== undefined ? { user_id: group.cloud_user } : undefined,
      configuration: group.workspace_dir !== undefined ? { workspace_dir: group.workspace_dir } : undefined,
      count: group.count,
    }));
    return conversationFilterOptions(groups, { owner, directory }, currentUserID, usersById, directoriesByPeer, t);
  }, [filterGroups.data, owner, directory, currentUserID, usersById, directoriesByPeer, t]);
  const optionsPending = filterGroups.isPending;
  const optionsError = filterGroups.isError;
  async function run(action: () => Promise<unknown>) {
    setError("");
    try {
      await action();
      setDialog(null);
      return true;
    } catch (cause) {
      setError(localizeAPIError(cause, t, errorMessage(cause, t("taskEntryRequestFailed"))));
      return false;
    }
  }
  const activeIDRef = useRef(activeID);
  const archiveCurrentRef = useRef(onArchiveCurrent);
  activeIDRef.current = activeID;
  archiveCurrentRef.current = onArchiveCurrent;
  function requestArchive(room: IMConversation) {
    if (entry.isWorking(room.id)) {
      setError(t("taskEntryBusyRoom"));
      return;
    }
    if (skipArchive)
      void run(() => entry.patch(room.id, { archived: true })).then((completed) => {
        if (completed && room.id === activeIDRef.current) archiveCurrentRef.current?.();
      });
    else {
      setDontAsk(false);
      setDialog({ kind: "archive", room });
    }
  }
  async function confirm() {
    if (!dialog) return;
    if (dialog.kind !== "rename" && entry.isWorking(dialog.room.id)) {
      setError(t("taskEntryBusyRoom"));
      return;
    }
    const completed = await run(() =>
      dialog.kind === "delete"
        ? entry.remove(dialog.room.id)
        : entry.patch(dialog.room.id, dialog.kind === "rename" ? { title: title.trim() } : { archived: true }),
    );
    if (completed && dialog.kind === "archive" && dontAsk) setSkipArchive(true);
    if (completed && dialog.kind === "archive" && dialog.room.id === activeIDRef.current) archiveCurrentRef.current?.();
  }

  const searchField = (
    <div className={styles.searchField}>
      <Search size={15} aria-hidden="true" />
      <TextInput
        variant="plain"
        ref={searchInputRef}
        className={styles.search}
        aria-label={t("taskEntrySearchHistory")}
        placeholder={t(compact ? "iaSessionSearchPlaceholder" : "taskEntrySearchHistory")}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />
      {search && (
        <button
          type="button"
          className={styles.clearSearch}
          aria-label={t("taskEntryClearSearch")}
          title={t("taskEntryClearSearch")}
          onClick={() => {
            setSearch("");
            setQuery("");
            searchInputRef.current?.focus();
          }}
        >
          <X size={14} aria-hidden="true" />
        </button>
      )}
    </div>
  );
  return (
    <div
      className={`${styles.root} ${compact ? styles.compact : ""} ${archived ? styles.archived : ""} ${collapsed ? styles.collapsed : ""}`}
    >
      {collapsed && (filtering || search) ? (
        <div className={styles.railControls}>
          <Tooltip content={directory || search || t("iaSessionFilters")} contentProps={{ side: "right" }}>
            <button type="button" aria-label={t("iaSessionDirectoryFilter")} onClick={onExpand}>
              <span className={styles.railFilter}>
                <Filter size={13} aria-hidden="true" />
              </span>
            </button>
          </Tooltip>
        </div>
      ) : null}
      <div hidden={collapsed} className={styles.searchBar}>
        {!collapsed ? searchField : null}
        {filters && onFiltersChange ? (
          <PopoverRoot open={!collapsed && filterOpen} onOpenChange={setFilterOpen}>
            <Tooltip content={t("iaSessionFilters")}>
              <PopoverTrigger asChild>
                <Button
                  type="button"
                  variant="secondaryGray"
                  iconOnly
                  size="sm"
                  className={styles.filterTrigger}
                  data-active={filtering || (collapsed && Boolean(search))}
                  aria-label={t("iaSessionFilters")}
                >
                  <Funnel size={16} aria-hidden="true" />
                  {filterCount ? <span className={styles.filterBadge}>{filterCount}</span> : null}
                </Button>
              </PopoverTrigger>
            </Tooltip>
            <PopoverContent
              align="end"
              collisionPadding={8}
              className={styles.filterPanel}
              aria-label={t("iaSessionFilters")}
            >
              <div className={styles.filterHead}>
                <span>{t("iaSessionFilters")}</span>
                {filtering ? (
                  <button
                    type="button"
                    className={styles.filterReset}
                    onClick={() => onFiltersChange({ search, owner: "", directory: "" })}
                  >
                    {t("iaSessionClearFilters")}
                  </button>
                ) : null}
              </div>
              {!optionsPending && !optionsError ? (
                <>
                  <div className={styles.filterSection} role="group" aria-label={t("iaSessionOwnerFilter")}>
                    <p>{t("iaSessionOwnerFilter")}</p>
                    <FilterRow
                      label={t("iaSessionOwnerAll")}
                      icon={<ListFilter size={14} />}
                      count={options.ownerTotal}
                      selected={!owner}
                      onSelect={() => onFiltersChange({ ...filters, owner: "" })}
                    />
                    {options.owners.map((option) => (
                      <FilterRow
                        key={option.value}
                        title={option.label}
                        label={option.label}
                        icon={
                          option.value === collaborationConversationOwner ? (
                            <Users size={14} />
                          ) : (
                            <AgentAvatarContent avatar={option.avatar} fallback={option.label.slice(0, 1)} />
                          )
                        }
                        count={option.count}
                        selected={owner === option.value}
                        onSelect={() => onFiltersChange({ ...filters, owner: option.value })}
                      />
                    ))}
                  </div>
                  <div className={styles.filterSection} role="group" aria-label={t("iaSessionDirectoryFilter")}>
                    <p>{t("iaSessionDirectoryFilter")}</p>
                    <FilterRow
                      label={t("iaSessionDirectoryAll")}
                      icon={<Folder size={14} />}
                      count={options.directoryTotal}
                      selected={!directory}
                      onSelect={() => onFiltersChange({ ...filters, directory: "" })}
                    />
                    {options.directories.map(({ path, count }) => {
                      const separator = path.replace(/[\\/]+$/, "").search(/[^\\/]+$/);
                      const prefix = separator > 0 ? path.slice(0, separator) : "";
                      return (
                        <FilterRow
                          key={path}
                          title={path}
                          label={
                            <span className={styles.filterPath}>
                              <span>{prefix}</span>
                              {path.slice(prefix.length)}
                            </span>
                          }
                          icon={<Folder size={14} />}
                          count={count}
                          selected={directory === path}
                          onSelect={() => onFiltersChange({ ...filters, directory: path })}
                        />
                      );
                    })}
                  </div>
                </>
              ) : null}
              {optionsPending ? <span role="status">{t("loading")}</span> : null}
              {optionsError ? (
                <Button size="sm" variant="secondaryGray" onClick={() => void filterGroups.refetch()}>
                  {t("retry")}
                </Button>
              ) : null}
            </PopoverContent>
          </PopoverRoot>
        ) : null}
      </div>
      {error && !dialog ? (
        <p role="alert" className={styles.error}>
          {error}
        </p>
      ) : null}
      <div
        className={styles.list}
        onPointerOver={(event) => event.currentTarget.classList.add(styles.pointerOver)}
        onPointerOut={(event) => {
          if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget))
            event.currentTarget.classList.remove(styles.pointerOver);
        }}
        onScroll={(event) => {
          const node = event.currentTarget;
          if (
            node.scrollHeight - node.scrollTop - node.clientHeight < 80 &&
            list.hasNextPage &&
            !list.isFetchingNextPage
          )
            void list.fetchNextPage();
        }}
      >
        {rooms.map((room) => {
          const other = resolveConversationUser(room, currentUserID, usersById);
          const managerDM =
            !room.cloud_agent &&
            room.is_direct &&
            (other?.id === MANAGER_PARTICIPANT_ID || other?.id === MANAGER_AGENT_ID);
          const cloudAgent = room.cloud_agent
            ? cloudCatalog.find(
                (agent) =>
                  agent.id === room.cloud_agent?.agent_id && agent.connectionID === room.cloud_agent?.connection_id,
              )
            : undefined;
          const searchMatch = query ? room.search_match : undefined;
          const preview = searchMatch || taskConversationPreview(room, entry.liveRooms?.get(room.id));
          const fullPreviewText = preview?.content?.replace(/<[^>]+>/g, " ") || "";
          const previewText = searchMatch ? searchExcerpt(fullPreviewText, query) : fullPreviewText.slice(0, 64);
          const timestamp = preview?.created_at || room.messages.at(-1)?.created_at || room.created_at;
          const working = entry.isWorking(room.id);
          const disabled = entry.busyID === room.id;
          return (
            <div
              key={room.id}
              className={`${styles.row} ${managerDM && !collapsed ? styles.noAvatar : ""} ${activeID === room.id ? styles.active : ""}`}
            >
              <Tooltip
                content={room.title || t("taskEntryUntitled")}
                contentProps={{ side: "right", className: styles.titleTooltip }}
              >
                <button
                  type="button"
                  className={styles.open}
                  aria-current={activeID === room.id ? "page" : undefined}
                  aria-label={collapsed ? room.title || t("taskEntryUntitled") : undefined}
                  onClick={() => onSelect(room.id, searchMatch)}
                >
                  {!managerDM || collapsed ? (
                    <span className={styles.avatar} aria-hidden="true">
                      {room.cloud_agent ? (
                        <AgentAvatarContent
                          avatar={cloudAgentAvatar(
                            cloudAgent?.type,
                            cloudAgent?.avatar ||
                              usersById.get(room.cloud_agent.user_id)?.avatar ||
                              other?.avatar ||
                              undefined,
                          )}
                        />
                      ) : room.is_direct ? (
                        <AgentAvatarContent avatar={other?.avatar} fallback={other?.name?.slice(0, 1) || "A"} />
                      ) : (
                        <RoomAvatar
                          members={room.members.map((id) => usersById.get(id)).filter((user) => user !== undefined)}
                          size={26}
                          showCountBadge={false}
                        />
                      )}
                    </span>
                  ) : null}
                  <span className={styles.text}>
                    <span className={styles.titleLine}>
                      <strong>{room.title || t("taskEntryUntitled")}</strong>
                      <CollaborationModeBadge conversation={room} compact t={t} />
                    </span>
                    <small
                      className={searchMatch ? styles.searchMatch : undefined}
                      title={searchMatch ? fullPreviewText : undefined}
                    >
                      {searchMatch ? (
                        highlightedSearchText(previewText, query)
                      ) : (
                        <>
                          {timestamp ? (
                            <time dateTime={timestamp} title={new Date(timestamp).toLocaleString()}>
                              {new Date(timestamp).toLocaleDateString()} ·{" "}
                            </time>
                          ) : null}
                          {working ? t("taskEntryRunning") : previewText}
                        </>
                      )}
                    </small>
                  </span>
                  {compact && timestamp ? (
                    <time
                      className={styles.timestamp}
                      dateTime={timestamp}
                      title={new Date(timestamp).toLocaleString()}
                    >
                      {new Date(timestamp).toDateString() === new Date().toDateString()
                        ? new Date(timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
                        : new Date(timestamp).toLocaleDateString([], { month: "numeric", day: "numeric" })}
                    </time>
                  ) : null}
                  {working ? <span className={styles.workingDot} title={t("taskEntryRunning")} /> : null}
                  {room.pinned ? (
                    <Pin className={styles.pinnedIndicator} size={13} aria-label={t("taskEntryPinned")} />
                  ) : null}
                </button>
              </Tooltip>
              <div className={styles.actions} hidden={collapsed}>
                <DropdownMenuRoot>
                  <DropdownMenuTrigger asChild>
                    {archived ? (
                      <Button
                        type="button"
                        variant="tertiaryGray"
                        size="sm"
                        iconOnly
                        className={styles.archivedMore}
                        aria-label={`${t("taskEntryMore")} · ${room.title}`}
                      >
                        <MoreHorizontal size={17} aria-hidden="true" />
                      </Button>
                    ) : (
                      <button
                        type="button"
                        className={styles.more}
                        aria-label={`${t("taskEntryMore")} · ${room.title}`}
                      >
                        <MoreHorizontal size={17} />
                      </button>
                    )}
                  </DropdownMenuTrigger>
                  <DropdownMenuContent>
                    <DropdownMenuItem
                      disabled={disabled}
                      onSelect={() => void run(() => entry.patch(room.id, { pinned: !room.pinned }))}
                    >
                      {t(room.pinned ? "taskEntryUnpin" : "taskEntryPin")}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      disabled={disabled}
                      onSelect={() => {
                        setTitle(room.title || "");
                        setDialog({ kind: "rename", room });
                      }}
                    >
                      {t("taskEntryRename")}
                    </DropdownMenuItem>
                    {archived ? (
                      <DropdownMenuItem
                        disabled={disabled}
                        onSelect={() => void run(() => entry.patch(room.id, { archived: false }))}
                      >
                        {t("taskEntryRestore")}
                      </DropdownMenuItem>
                    ) : (
                      <DropdownMenuItem disabled={disabled || working} onSelect={() => requestArchive(room)}>
                        {t("taskEntryArchive")}
                      </DropdownMenuItem>
                    )}
                    {room.is_direct && onNewWith ? (
                      <>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onSelect={() => onNewWith(room)}>
                          {t("taskEntryNewWithAgent")}
                        </DropdownMenuItem>
                      </>
                    ) : null}
                    {room.is_direct && onEditAgent ? (
                      <DropdownMenuItem onSelect={() => onEditAgent(room)}>{t("taskEntryEditAgent")}</DropdownMenuItem>
                    ) : null}
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      danger
                      disabled={disabled || working}
                      onSelect={() => {
                        setError("");
                        setDialog({ kind: "delete", room });
                      }}
                    >
                      {t("taskEntryDelete")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenuRoot>
                {compact && !archived ? (
                  <>
                    <Tooltip
                      content={working ? t("taskEntryBusyRoom") : t("taskEntryArchive")}
                      contentProps={{ side: "bottom" }}
                    >
                      <button
                        type="button"
                        className={styles.more}
                        disabled={disabled || working}
                        aria-label={`${t("taskEntryArchive")} · ${room.title}`}
                        onClick={() => requestArchive(room)}
                      >
                        <Archive size={16} aria-hidden="true" />
                      </button>
                    </Tooltip>
                    <Tooltip
                      content={t(room.pinned ? "taskEntryUnpin" : "taskEntryPin")}
                      contentProps={{ side: "bottom" }}
                    >
                      <button
                        type="button"
                        className={styles.more}
                        disabled={disabled}
                        aria-label={`${t(room.pinned ? "taskEntryUnpin" : "taskEntryPin")} · ${room.title}`}
                        aria-pressed={Boolean(room.pinned)}
                        onClick={() => void run(() => entry.patch(room.id, { pinned: !room.pinned }))}
                      >
                        {room.pinned ? <PinOff size={16} aria-hidden="true" /> : <Pin size={16} aria-hidden="true" />}
                      </button>
                    </Tooltip>
                  </>
                ) : null}
              </div>
            </div>
          );
        })}
        {list.isPending ? <p role="status">{t("loading")}</p> : null}
        {list.isError ? (
          <p role="alert">
            {localizeAPIError(list.error, t, errorMessage(list.error, t("taskEntryRequestFailed")))}
            <Button variant="tertiaryGray" size="sm" onClick={() => void list.refetch()}>
              {t("retry")}
            </Button>
          </p>
        ) : null}
        {!list.isPending && !list.isError && !rooms.length ? (
          <p className={styles.empty}>
            {t(archived ? "archivedEmpty" : filtering ? "iaSessionNoFilterMatches" : "taskEntryNoHistory")}
          </p>
        ) : null}
        {list.hasNextPage && !collapsed ? (
          <Button
            variant="tertiaryGray"
            size="sm"
            disabled={list.isFetchingNextPage}
            aria-label={t("taskEntryLoadMore")}
            onClick={() => void list.fetchNextPage()}
          >
            {collapsed ? <ChevronDown size={16} aria-hidden="true" /> : t("taskEntryLoadMore")}
          </Button>
        ) : null}
      </div>
      <DialogRoot
        open={Boolean(dialog)}
        onOpenChange={(open) => {
          if (!open && !entry.busyID) setDialog(null);
        }}
      >
        <DialogContent className={styles.dialog}>
          <DialogHeader>
            <div>
              <DialogTitle>
                {t(
                  dialog?.kind === "rename"
                    ? "taskEntryRename"
                    : dialog?.kind === "delete"
                      ? "taskEntryDelete"
                      : "taskEntryArchive",
                )}
              </DialogTitle>
              <DialogDescription>{dialog?.room.title}</DialogDescription>
            </div>
          </DialogHeader>
          <DialogBody className={styles.dialogBody}>
            {error ? (
              <p role="alert" className={styles.error}>
                {error}
              </p>
            ) : null}
            {dialog?.kind === "rename" ? (
              <TextInput
                autoFocus
                className={styles.dialogInput}
                aria-label={t("taskEntryRename")}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
            ) : (
              <p className={styles.dialogMessage}>
                {t(dialog?.kind === "delete" ? "taskEntryDeleteWarning" : "taskEntryArchiveHint")}
              </p>
            )}
            {dialog?.kind === "archive" ? (
              <label className={styles.dialogCheckbox}>
                <Checkbox
                  checked={dontAsk}
                  aria-label={t("taskEntryDontAsk")}
                  onCheckedChange={(checked) => setDontAsk(checked === true)}
                />
                <span>{t("taskEntryDontAsk")}</span>
              </label>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <Button variant="secondaryGray" disabled={Boolean(entry.busyID)} onClick={() => setDialog(null)}>
              {t("cancel")}
            </Button>
            <Button
              loading={Boolean(deletingID)}
              variant={dialog?.kind === "delete" ? "danger" : "primary"}
              disabled={Boolean(entry.busyID) || (dialog?.kind === "rename" && !title.trim())}
              onClick={() => void confirm()}
            >
              {t("confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </div>
  );
}
