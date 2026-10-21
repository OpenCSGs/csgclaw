import { useEffect, useRef, useState } from "react";
import { Archive, MoreHorizontal, Pin, PinOff, Search, X } from "lucide-react";
import {
  Button,
  Tooltip,
  Select,
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
import { RoomAvatar } from "@/components/business/RoomAvatar";
import { useTaskConversationPages } from "@/hooks/workspace/useTaskConversationPages";
import { errorMessage } from "@/api/client";
import { uniqueTaskConversations } from "@/models/taskConversations";
import { resolveConversationUser } from "@/models/conversations";
import type { IMConversation, IMMessage, TranslateFn, UsersById } from "@/models/conversations";
import type { TaskEntryController } from "@/hooks/workspace/useTaskEntry";
import { useArchiveConfirmation } from "@/hooks/workspace/useArchiveConfirmation";
import { MANAGER_AGENT_ID, MANAGER_PARTICIPANT_ID } from "@/shared/constants/agents";
import { localizeAPIError } from "@/shared/i18n";
import styles from "./TaskConversationList.module.css";

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
  archived?: boolean;
  compact?: boolean;
  directories?: { path: string; agentIDs: string[] }[];
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

export function TaskConversationList({
  archived = false,
  compact = false,
  directories = [],
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
  const [search, setSearch] = useState("");
  const searchInputRef = useRef<HTMLInputElement>(null);
  const searchBarRef = useRef<HTMLDivElement>(null);
  const [searchBarWidth, setSearchBarWidth] = useState<number>();
  const [directory, setDirectory] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<{ kind: "rename" | "archive" | "delete"; room: IMConversation } | null>(null);
  const [title, setTitle] = useState("");
  const [dontAsk, setDontAsk] = useState(false);
  const [skipArchive, setSkipArchive] = useArchiveConfirmation();
  useEffect(() => {
    const element = searchBarRef.current;
    if (!element) return;
    const updateWidth = () => setSearchBarWidth(element.getBoundingClientRect().width);
    updateWidth();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(updateWidth);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    const timer = window.setTimeout(() => setQuery(search), 250);
    return () => window.clearTimeout(timer);
  }, [search]);
  const list = useTaskConversationPages(archived, query);
  const allRooms = uniqueTaskConversations(list.data?.pages ?? []);
  const directoryPaths = Array.from(
    new Set([
      ...directories.map((item) => item.path),
      ...allRooms.map((room) => room.configuration?.workspace_dir || ""),
    ]),
  ).filter(Boolean);
  const directoryItem = directories.find((item) => item.path === directory);
  const rooms = allRooms.filter((room) => {
    if (!directory) return true;
    if (room.configuration) return room.configuration.workspace_dir === directory;
    return room.members.some((id) => directoryItem?.agentIDs.includes(id));
  });
  const fetchNextPage = list.fetchNextPage;
  useEffect(() => {
    if (directory && rooms.length < 20 && list.hasNextPage && !list.isFetchingNextPage && !list.isFetchNextPageError)
      void fetchNextPage();
  }, [directory, rooms.length, list.hasNextPage, list.isFetchingNextPage, list.isFetchNextPageError, fetchNextPage]);
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

  return (
    <div className={`${styles.root} ${compact ? styles.compact : ""} ${archived ? styles.archived : ""}`}>
      <div ref={searchBarRef} className={styles.searchBar}>
        <div className={styles.searchField}>
          <Search size={15} aria-hidden="true" />
          <input
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
        {compact ? (
          <Select
            size="sm"
            triggerClassName={styles.directory}
            contentClassName={styles.directoryMenu}
            triggerProps={{
              "aria-label": t("iaSessionDirectoryFilter"),
              title: directory || t("iaSessionDirectoryAll"),
            }}
            contentProps={{
              align: "end",
              collisionPadding: 8,
              style: searchBarWidth ? { width: searchBarWidth } : undefined,
            }}
            value={directory}
            onValueChange={setDirectory}
            options={[
              { value: "", label: t("iaSessionDirectoryAll") },
              ...directoryPaths.map((path) => {
                const trimmed = path.replace(/[\\/]+$/, "");
                const separator = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
                const prefix = separator >= 0 ? trimmed.slice(0, separator + 1) : "";
                const current = separator >= 0 ? trimmed.slice(separator + 1) : trimmed;
                return {
                  value: path,
                  label: current || path,
                  description: (
                    <span className={styles.directoryPath} title={path}>
                      <span className={styles.directoryPrefix}>{prefix}</span>
                      <span className={styles.directoryCurrent}>{current}</span>
                    </span>
                  ),
                };
              }),
            ]}
          />
        ) : null}
      </div>
      {error && !dialog ? (
        <p role="alert" className={styles.error}>
          {error}
        </p>
      ) : null}
      <div
        className={styles.list}
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
          const managerDM = room.is_direct && (other?.id === MANAGER_PARTICIPANT_ID || other?.id === MANAGER_AGENT_ID);
          const searchMatch = query ? room.search_match : undefined;
          const preview = searchMatch || room.messages.at(-1);
          const fullPreviewText = preview?.content?.replace(/<[^>]+>/g, " ") || "";
          const previewText = searchMatch ? searchExcerpt(fullPreviewText, query) : fullPreviewText.slice(0, 64);
          const timestamp = preview?.created_at || room.messages.at(-1)?.created_at || room.created_at;
          const working = entry.isWorking(room.id);
          const disabled = entry.busyID === room.id;
          return (
            <div
              key={room.id}
              className={`${styles.row} ${managerDM ? styles.noAvatar : ""} ${activeID === room.id ? styles.active : ""}`}
            >
              <Tooltip
                content={room.title || t("taskEntryUntitled")}
                contentProps={{ side: "right", className: styles.titleTooltip }}
              >
                <button
                  type="button"
                  className={styles.open}
                  aria-current={activeID === room.id ? "page" : undefined}
                  onClick={() => onSelect(room.id, searchMatch)}
                >
                  {!managerDM ? (
                    <span className={styles.avatar} aria-hidden="true">
                      {room.is_direct ? (
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
                    <strong>{room.title || t("taskEntryUntitled")}</strong>
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
              <div className={styles.actions}>
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
                    {archived ? (
                      <>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          danger
                          disabled={disabled || working}
                          onSelect={() => setDialog({ kind: "delete", room })}
                        >
                          {t("taskEntryDelete")}
                        </DropdownMenuItem>
                      </>
                    ) : null}
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
          <p className={styles.empty}>{t(archived ? "archivedEmpty" : "taskEntryNoHistory")}</p>
        ) : null}
        {list.hasNextPage ? (
          <Button
            variant="tertiaryGray"
            size="sm"
            disabled={list.isFetchingNextPage}
            onClick={() => void list.fetchNextPage()}
          >
            {t("taskEntryLoadMore")}
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
