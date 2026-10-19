import { useEffect, useState } from "react";
import { Archive, MoreHorizontal, Pin, PinOff, Search } from "lucide-react";
import {
  Button,
  Tooltip,
  Select,
  DialogRoot,
  DialogContent,
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
import type { IMConversation, TranslateFn, UsersById } from "@/models/conversations";
import type { TaskEntryController } from "@/hooks/workspace/useTaskEntry";
import { useArchiveConfirmation } from "@/hooks/workspace/useArchiveConfirmation";
import { MANAGER_AGENT_ID, MANAGER_PARTICIPANT_ID } from "@/shared/constants/agents";
import { localizeAPIError } from "@/shared/i18n";
import styles from "./TaskConversationList.module.css";

type Props = {
  archived?: boolean;
  compact?: boolean;
  directories?: { path: string; agentIDs: string[] }[];
  entry: TaskEntryController;
  activeID?: string;
  currentUserID: string;
  usersById: UsersById;
  t: TranslateFn;
  onSelect: (id: string) => void;
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
  onNewWith,
  onEditAgent,
}: Props) {
  const [search, setSearch] = useState("");
  const [directory, setDirectory] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<{ kind: "rename" | "archive" | "delete"; room: IMConversation } | null>(null);
  const [title, setTitle] = useState("");
  const [dontAsk, setDontAsk] = useState(false);
  const [skipArchive, setSkipArchive] = useArchiveConfirmation();
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
  function requestArchive(room: IMConversation) {
    if (entry.isWorking(room.id)) {
      setError(t("taskEntryBusyRoom"));
      return;
    }
    if (skipArchive) void run(() => entry.patch(room.id, { archived: true }));
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
  }

  return (
    <div className={`${styles.root} ${compact ? styles.compact : ""}`}>
      <div className={styles.searchBar}>
        <div className={styles.searchField}>
          <Search size={15} aria-hidden="true" />
          <input
            className={styles.search}
            aria-label={t("taskEntrySearchHistory")}
            placeholder={t(compact ? "iaSessionSearchPlaceholder" : "taskEntrySearchHistory")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        {compact ? (
          <Select
            size="sm"
            triggerClassName={styles.directory}
            triggerProps={{
              "aria-label": t("iaSessionDirectoryFilter"),
              title: directory || t("iaSessionDirectoryAll"),
            }}
            contentProps={{ align: "end", collisionPadding: 8 }}
            value={directory}
            onValueChange={setDirectory}
            options={[
              { value: "", label: t("iaSessionDirectoryAll") },
              ...directoryPaths.map((path) => ({
                value: path,
                label:
                  path
                    .replace(/[\\/]+$/, "")
                    .split(/[\\/]/)
                    .at(-1) || path,
                description: path,
              })),
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
          const timestamp = room.messages.at(-1)?.created_at || room.created_at;
          const working = entry.isWorking(room.id);
          const disabled = entry.busyID === room.id;
          return (
            <div
              key={room.id}
              className={`${styles.row} ${managerDM ? styles.noAvatar : ""} ${activeID === room.id ? styles.active : ""}`}
            >
              <button type="button" className={styles.open} onClick={() => onSelect(room.id)}>
                {!managerDM ? (
                  <span className={styles.avatar}>
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
                  <small>
                    {timestamp ? (
                      <time dateTime={timestamp} title={new Date(timestamp).toLocaleString()}>
                        {new Date(timestamp).toLocaleDateString()} ·{" "}
                      </time>
                    ) : null}
                    {working
                      ? t("taskEntryRunning")
                      : room.messages
                          .at(-1)
                          ?.content?.replace(/<[^>]+>/g, " ")
                          .slice(0, 64)}
                  </small>
                </span>
                {compact && timestamp ? (
                  <time className={styles.timestamp} dateTime={timestamp} title={new Date(timestamp).toLocaleString()}>
                    {new Date(timestamp).toDateString() === new Date().toDateString()
                      ? new Date(timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
                      : new Date(timestamp).toLocaleDateString([], { month: "numeric", day: "numeric" })}
                  </time>
                ) : null}
                {working ? <span className={styles.workingDot} title={t("taskEntryRunning")} /> : null}
                {room.pinned ? <Pin size={13} aria-label={t("taskEntryPinned")} /> : null}
              </button>
              <div className={styles.actions}>
                <DropdownMenuRoot>
                  <DropdownMenuTrigger asChild>
                    <button type="button" className={styles.more} aria-label={`${t("taskEntryMore")} · ${room.title}`}>
                      <MoreHorizontal size={17} />
                    </button>
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
                        <Archive size={14} />
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
                    <Tooltip content={working ? t("taskEntryBusyRoom") : t("taskEntryArchive")}>
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
                    <Tooltip content={t(room.pinned ? "taskEntryUnpin" : "taskEntryPin")}>
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
        <DialogContent className={styles.dialog} aria-describedby={undefined}>
          <DialogTitle>
            {t(
              dialog?.kind === "rename"
                ? "taskEntryRename"
                : dialog?.kind === "delete"
                  ? "taskEntryDelete"
                  : "taskEntryArchive",
            )}
          </DialogTitle>
          <p>{dialog?.room.title}</p>
          {error ? (
            <p role="alert" className={styles.error}>
              {error}
            </p>
          ) : null}
          {dialog?.kind === "rename" ? (
            <input
              autoFocus
              aria-label={t("taskEntryRename")}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          ) : (
            <p>{t(dialog?.kind === "delete" ? "taskEntryDeleteWarning" : "taskEntryArchiveHint")}</p>
          )}
          {dialog?.kind === "archive" ? (
            <label>
              <input type="checkbox" checked={dontAsk} onChange={(event) => setDontAsk(event.target.checked)} />
              {t("taskEntryDontAsk")}
            </label>
          ) : null}
          <div className={styles.dialogActions}>
            <Button variant="secondaryGray" disabled={Boolean(entry.busyID)} onClick={() => setDialog(null)}>
              {t("cancel")}
            </Button>
            <Button
              disabled={Boolean(entry.busyID) || (dialog?.kind === "rename" && !title.trim())}
              onClick={() => void confirm()}
            >
              {t("confirm")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
    </div>
  );
}
