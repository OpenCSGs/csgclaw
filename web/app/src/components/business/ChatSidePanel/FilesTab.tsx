import { useRoomFileReader } from "./useRoomFileReader";
import { useState } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUp, AtSign, File, Folder, RefreshCw } from "lucide-react";
import { attachmentCapability, fetchWorkspaceDirectory, filePreviewItem } from "@/api/chatFiles";
import { errorMessage } from "@/api/client";
import { Button, Select, Tooltip } from "@/components/ui";
import { formatAttachmentSize, type AttachmentPreviewItem } from "@/models/attachments";
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
  const source = controller.state.fileSource;
  const setSource = (fileSource: "workspace" | "attachments") => controller.patch({ fileSource });
  const [query, setQuery] = useState("");
  const [visibleCount, setVisibleCount] = useState(100);
  const queryClient = useQueryClient();
  const reader = useRoomFileReader(scope);
  const agentID = scope.agents.some((agent) => agent.id === controller.state.agentID)
    ? controller.state.agentID
    : scope.defaultAgentID;
  const directory = controller.state.directory;
  const workspace = useQuery({
    queryKey: ["chat-workspace", scope.roomID, agentID, directory],
    queryFn: ({ signal }) => fetchWorkspaceDirectory(agentID, directory, signal, scope.roomID),
    enabled: Boolean(agentID),
  });
  const attachments = useInfiniteQuery({
    queryKey: ["chat-room-files", scope.roomID, query],
    queryFn: ({ pageParam, signal }) => reader.read(query, pageParam, signal),
    initialPageParam: 0,
    getNextPageParam: (last) =>
      last.from + last.items.length < last.total ? last.from + last.items.length : undefined,
  });
  const isWorkspace = source === "workspace" && Boolean(agentID);
  const entries = (workspace.data?.entries ?? [])
    .filter((entry) => (entry.name ?? entry.path).toLowerCase().includes(query.toLowerCase()))
    .sort(
      (a, b) =>
        Number(b.type === "dir") - Number(a.type === "dir") || (a.name ?? a.path).localeCompare(b.name ?? b.path),
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
      <div className="chat-files-row" key={reference.source === "workspace" ? reference.path : reference.attachment_id}>
        <button
          type="button"
          className="chat-files-open"
          onClick={() => onPreview(filePreviewItem(scope.roomID, reference, size, mediaType, token))}
          title={reference.label}
        >
          <span className="chat-file-glyph">
            <File size={18} />
          </span>
          <span className="chat-file-name">{name}</span>
          <small>{formatAttachmentSize(size)}</small>
        </button>
        {referenceButton(name, reference)}
      </div>
    );
  }
  return (
    <section className="chat-files-tab" aria-label={t("chatFiles")}>
      <div className="chat-files-toolbar">
        <div className="chat-files-sources" role="tablist" aria-label={t("chatFileSource")}>
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
        <Tooltip content={t("chatRefresh")}>
          <Button iconOnly aria-label={t("chatRefresh")} size="sm" variant="tertiaryGray" onClick={refresh}>
            <RefreshCw size={16} />
          </Button>
        </Tooltip>
      </div>
      {isWorkspace ? (
        <div className="chat-files-location">
          <Select
            aria-label={t("chatWorkspaceAgent")}
            value={agentID}
            options={scope.agents.map((agent) => ({ value: agent.id, label: agent.name }))}
            onValueChange={(value) => {
              controller.patch({ agentID: value, directory: "" });
              setQuery("");
              setVisibleCount(100);
            }}
          />
          <div className="chat-files-path">
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatParentDirectory")}
              disabled={!directory}
              onClick={() => changeDirectory(directory.split("/").slice(0, -1).join("/"))}
            >
              <ArrowUp size={16} />
            </Button>
            <span title={directory || "/"}>{directory || "/"}</span>
          </div>
        </div>
      ) : null}
      <input
        className="chat-files-search"
        type="search"
        placeholder={t("chatSearchFiles")}
        aria-label={t("chatSearchFiles")}
        value={query}
        onChange={(event) => {
          setQuery(event.target.value);
          setVisibleCount(100);
        }}
      />
      <div className="chat-files-list">
        {loading ? (
          <div className="chat-side-status" role="status">
            {t("loading")}
          </div>
        ) : error ? (
          <div className="chat-side-status" role="alert">
            {errorMessage(error, t("attachmentPreviewFailed"))}
            <Button onClick={refresh} size="sm" variant="secondaryGray">
              {t("retrySend")}
            </Button>
          </div>
        ) : null}
        {!loading && !error && (isWorkspace ? entries.length === 0 : files.length === 0) ? (
          <div className="chat-side-status">{t("chatFilesEmpty")}</div>
        ) : null}
        {isWorkspace
          ? entries.slice(0, visibleCount).map((entry) =>
              entry.type === "dir" ? (
                <div className="chat-files-row" key={entry.path}>
                  <button className="chat-files-open" type="button" onClick={() => changeDirectory(entry.path)}>
                    <span className="chat-file-glyph is-folder">
                      <Folder size={18} />
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
