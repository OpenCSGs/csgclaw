import chatSidePanelStyles from "./ChatSidePanel.module.css";
import { WorkspacePathActions } from "./WorkspacePathActions";
import { useRoomFileReader } from "./useRoomFileReader";
import { useEffect, useRef, useState } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { AtSign, RefreshCw } from "lucide-react";
import { attachmentCapability, fetchWorkspaceDirectory, filePreviewItem } from "@/api/chatFiles";
import { errorMessage } from "@/api/client";
import { Breadcrumb, Button, Select, TextInput, Tooltip } from "@/components/ui";
import { FileTypeIcon } from "@/components/business/FileTypeIcon";
import { formatAttachmentSize, type AttachmentPreviewItem } from "@/models/attachments";
import { compareFiles, fileSortOptions, type FileSort } from "@/models/workspace";
import type { ComposerFileReference } from "@/models/fileReferences";
import type { TranslateFn } from "@/models/conversations";
import type { ChatFileScope, ReferenceFile } from "./types";
import type { ChatSidePanelController } from "./useChatSidePanel";

export function FilesTab({
  scope,
  controller,
  onPreview,
  onReference,
  t,
}: {
  scope: ChatFileScope;
  controller: ChatSidePanelController;
  onPreview: (item: AttachmentPreviewItem) => void;
  onReference: ReferenceFile;
  t: TranslateFn;
}) {
  const draftWorkspace = !scope.roomID;
  const source = draftWorkspace ? "workspace" : controller.state.fileSource;
  const setSource = (fileSource: "workspace" | "attachments") => controller.patch({ fileSource });
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<FileSort>("updated-desc");
  const [visibleCount, setVisibleCount] = useState(100);
  const queryClient = useQueryClient();
  const reader = useRoomFileReader(scope);
  const refreshTranscript = reader.refresh;
  const attachmentIDs = JSON.stringify(
    [...(scope.messages ?? []), ...(scope.retainedMessages ?? [])]
      .flatMap((message) => (message.attachments ?? []).map((attachment) => attachment.id))
      .sort(),
  );
  const observedAttachments = useRef({ roomID: scope.roomID, ids: attachmentIDs });
  useEffect(() => {
    const previous = observedAttachments.current;
    observedAttachments.current = { roomID: scope.roomID, ids: attachmentIDs };
    if (previous.roomID !== scope.roomID || previous.ids === attachmentIDs) return;
    void Promise.resolve(refreshTranscript()).then(() =>
      queryClient.invalidateQueries({ queryKey: ["chat-room-files", scope.roomID] }),
    );
  }, [attachmentIDs, scope.roomID, refreshTranscript, queryClient]);
  const agentID = scope.agents.some((agent) => agent.id === controller.state.agentID)
    ? controller.state.agentID
    : scope.defaultAgentID;
  const directory = controller.state.directory;
  const directorySegments = directory.split("/").filter(Boolean);
  const rootWorkspace = useQuery({
    queryKey: ["chat-workspace", scope.roomID, agentID, "", scope.workspaceDir],
    queryFn: ({ signal }) => fetchWorkspaceDirectory(agentID, "", signal, scope.roomID, scope.workspaceDir),
    enabled: Boolean(agentID) && !scope.automaticWorkspace,
  });
  const workspace = useQuery({
    queryKey: ["chat-workspace", scope.roomID, agentID, directory, scope.workspaceDir],
    queryFn: ({ signal }) => fetchWorkspaceDirectory(agentID, directory, signal, scope.roomID, scope.workspaceDir),
    enabled: Boolean(agentID) && !scope.automaticWorkspace,
  });
  const workspaceRootName =
    rootWorkspace.data?.absolute_path?.split(/[\\/]/).filter(Boolean).at(-1) || t("chatWorkspaceFiles");
  const attachments = useInfiniteQuery({
    queryKey: ["chat-room-files", scope.roomID, query, sort],
    enabled: Boolean(scope.roomID),
    queryFn: ({ pageParam, signal }) => reader.read(query, pageParam, signal, sort),
    initialPageParam: 0,
    getNextPageParam: (last) =>
      last.from + last.items.length < last.total ? last.from + last.items.length : undefined,
  });
  const isWorkspace = source === "workspace" && (draftWorkspace || Boolean(agentID));
  const entries = (workspace.data?.entries ?? [])
    .filter((entry) => (entry.name ?? entry.path).toLowerCase().includes(query.toLowerCase()))
    .sort(
      (a, b) =>
        Number(b.type === "dir") - Number(a.type === "dir") ||
        compareFiles(
          { name: a.name ?? a.path, date: a.modified_at },
          { name: b.name ?? b.path, date: b.modified_at },
          sort,
        ),
    );
  const files = attachments.data?.pages.flatMap((page) => page.items) ?? [];
  const loading = isWorkspace ? workspace.isLoading : attachments.isLoading;
  const error = isWorkspace ? workspace.error : attachments.error;
  function changeDirectory(path: string) {
    controller.patch({ directory: path });
    setQuery("");
    setVisibleCount(100);
  }
  function refresh() {
    void reader.refresh();
    void queryClient.invalidateQueries({
      queryKey: isWorkspace ? ["chat-workspace", scope.roomID, agentID] : ["chat-room-files", scope.roomID],
    });
  }
  function referenceButton(name: string, reference: ComposerFileReference) {
    return (
      <Tooltip content={t("chatFileReference")}>
        <Button
          className="chat-file-reference"
          size="sm"
          variant="tertiaryGray"
          iconOnly
          aria-label={`${t("chatFileReference")} ${name}`}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onReference(reference)}
        >
          <AtSign size={16} />
        </Button>
      </Tooltip>
    );
  }
  function fileRow(name: string, reference: ComposerFileReference, size: number, mediaType = "", token = "") {
    return (
      <div
        className={chatSidePanelStyles["chat-files-row"]}
        key={reference.source === "workspace" ? reference.path : reference.attachment_id}
      >
        <button
          type="button"
          className={chatSidePanelStyles["chat-files-open"]}
          onClick={() =>
            onPreview(filePreviewItem(scope.roomID, reference, size, mediaType, token, scope.workspaceDir))
          }
          title={reference.label}
        >
          <span className="chat-file-glyph">
            <FileTypeIcon name={name} mediaType={mediaType} />
          </span>
          <span className="chat-file-name">{name}</span>
          <small>{formatAttachmentSize(size)}</small>
        </button>
        {referenceButton(name, reference)}
      </div>
    );
  }
  return (
    <section className={chatSidePanelStyles["chat-files-tab"]} aria-label={t("chatFiles")}>
      <div className={chatSidePanelStyles["chat-files-toolbar"]}>
        {!draftWorkspace ? (
          <div className={chatSidePanelStyles["chat-files-sources"]} role="tablist" aria-label={t("chatFileSource")}>
            <Button
              role="tab"
              aria-selected={isWorkspace}
              active={isWorkspace}
              size="sm"
              variant="tertiaryGray"
              disabled={!scope.agents.length}
              onClick={() => {
                setSource("workspace");
                setQuery("");
              }}
            >
              {t("chatWorkspaceFiles")}
            </Button>
            <Button
              role="tab"
              aria-selected={!isWorkspace}
              active={!isWorkspace}
              size="sm"
              variant="tertiaryGray"
              onClick={() => {
                setSource("attachments");
                setQuery("");
              }}
            >
              {t("chatConversationFiles")}
            </Button>
          </div>
        ) : (
          <strong>{t("chatWorkspaceFiles")}</strong>
        )}
        <Tooltip content={t("chatRefresh")}>
          <Button
            iconOnly
            aria-label={t("chatRefresh")}
            disabled={scope.automaticWorkspace}
            size="sm"
            variant="tertiaryGray"
            onClick={refresh}
          >
            <RefreshCw size={16} />
          </Button>
        </Tooltip>
      </div>
      {isWorkspace && !scope.automaticWorkspace ? (
        <div className={chatSidePanelStyles["chat-files-location"]}>
          {scope.sharedWorkspace ? <span>{t("chatSharedWorkspace")}</span> : null}
          {!scope.direct && !scope.sharedWorkspace ? (
            <Select
              triggerProps={{ "aria-label": t("chatWorkspaceAgent") }}
              triggerClassName={chatSidePanelStyles["chat-files-agent"]}
              value={agentID}
              options={scope.agents.map((agent) => ({ value: agent.id, label: agent.name }))}
              onValueChange={(value) => {
                controller.patch({ agentID: value, directory: "" });
                setQuery("");
                setVisibleCount(100);
              }}
            />
          ) : null}
          <Breadcrumb
            className={chatSidePanelStyles["chat-files-path"]}
            wrap
            aria-label={t("chatWorkspaceFiles")}
            overflowLabel={t("chatBreadcrumbShowEarlier")}
            items={[
              { label: workspaceRootName, current: directorySegments.length === 0, onClick: () => changeDirectory("") },
              ...directorySegments.map((segment, index) => ({
                label: segment,
                current: index === directorySegments.length - 1,
                onClick: () => changeDirectory(directorySegments.slice(0, index + 1).join("/")),
              })),
            ]}
          />
          <WorkspacePathActions
            roomID={scope.roomID}
            workspaceDir={scope.workspaceDir}
            reference={{ source: "workspace", agent_id: agentID, path: directory, name: directory, label: directory }}
            location={
              workspace.data?.absolute_path
                ? {
                    absolute_path: workspace.data.absolute_path,
                    directory: true,
                    open_targets: workspace.data.open_targets ?? [],
                  }
                : undefined
            }
            t={t}
          />
        </div>
      ) : null}
      <div className={chatSidePanelStyles["chat-files-search-wrap"]}>
        <TextInput
          size="sm"
          type="search"
          disabled={scope.automaticWorkspace}
          placeholder={t("chatSearchFiles")}
          aria-label={t("chatSearchFiles")}
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setVisibleCount(100);
          }}
        />
        <Select
          size="sm"
          disabled={scope.automaticWorkspace}
          triggerProps={{ "aria-label": t("chatSortFiles") }}
          triggerClassName={chatSidePanelStyles["chat-files-sort"]}
          contentClassName={chatSidePanelStyles["chat-files-sort-menu"]}
          value={sort}
          options={fileSortOptions.map((value) => ({ value, label: t(`chatFileSort.${value}`) }))}
          onValueChange={(value) => {
            if (fileSortOptions.some((option) => option === value)) setSort(value as FileSort);
            setVisibleCount(100);
          }}
        />
      </div>
      <div className={chatSidePanelStyles["chat-files-list"]}>
        {loading ? (
          <div className={chatSidePanelStyles["chat-side-status"]} role="status">
            {t("loading")}
          </div>
        ) : error ? (
          <div className={chatSidePanelStyles["chat-side-status"]} role="alert">
            {errorMessage(error, t("attachmentPreviewFailed"))}
            <Button onClick={refresh} size="sm" variant="secondaryGray">
              {t("retrySend")}
            </Button>
          </div>
        ) : null}
        {!loading && !error && (isWorkspace ? entries.length === 0 : files.length === 0) ? (
          <div className={chatSidePanelStyles["chat-side-status"]}>
            {t(scope.automaticWorkspace ? "collaborationWorkdirAutomaticHint" : "chatFilesEmpty")}
          </div>
        ) : null}
        {isWorkspace
          ? entries.slice(0, visibleCount).map((entry) =>
              entry.type === "dir" ? (
                <div className={chatSidePanelStyles["chat-files-row"]} key={entry.path}>
                  <button
                    className={chatSidePanelStyles["chat-files-open"]}
                    type="button"
                    onClick={() => changeDirectory(entry.path)}
                  >
                    <span className="chat-file-glyph">
                      <FileTypeIcon isFolder />
                    </span>
                    <span className="chat-file-name">{entry.name}</span>
                  </button>
                  {referenceButton(entry.name ?? entry.path, {
                    source: "workspace",
                    agent_id: agentID,
                    path: entry.path,
                    label: entry.path + "/",
                    name: entry.name ?? entry.path,
                  })}
                </div>
              ) : (
                fileRow(
                  entry.name ?? entry.path,
                  {
                    source: "workspace",
                    agent_id: agentID,
                    path: entry.path,
                    label: entry.path,
                    name: entry.name ?? entry.path,
                  },
                  entry.size ?? 0,
                )
              ),
            )
          : files.map((file) =>
              fileRow(
                file.name,
                { source: "attachment", attachment_id: file.id, label: file.name, name: file.name },
                file.size_bytes,
                file.media_type,
                attachmentCapability(file),
              ),
            )}
        {isWorkspace && entries.length > visibleCount ? (
          <Button size="sm" variant="secondaryGray" onClick={() => setVisibleCount((value) => value + 100)}>
            {t("chatLoadMore")}
          </Button>
        ) : null}
        {!isWorkspace && attachments.hasNextPage ? (
          <Button
            size="sm"
            variant="secondaryGray"
            disabled={attachments.isFetchingNextPage}
            onClick={() => void attachments.fetchNextPage()}
          >
            {t("chatLoadMore")}
          </Button>
        ) : null}
      </div>
    </section>
  );
}
