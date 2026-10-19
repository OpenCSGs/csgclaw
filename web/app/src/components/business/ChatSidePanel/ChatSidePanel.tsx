import { WorkspacePathActions } from "./WorkspacePathActions";
import { useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  BookOpen,
  FileText,
  Folder,
  Globe,
  LayoutGrid,
  Maximize2,
  MessageSquare,
  Minimize2,
  PanelRightClose,
  Plus,
  X,
} from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import { FolderIcon } from "@/components/business/FileTypeIcon";
import { DocumentPreviewPanel, documentPreviewKind } from "@/components/business/DocumentPreviewPanel";
import { ConversationActivityPanel } from "@/components/business/ConversationActivityPanel";
import { CitationSources } from "@/components/business/MessageContent";
import { attachmentCapability, filePreviewItem, transcriptRoomFiles } from "@/api/chatFiles";
import { useRoomFileReader } from "./useRoomFileReader";
import { clampChatSideWidth, type ChatSideTab } from "@/models/chatSidePanel";
import { parseCitationDocument } from "@/models/citations";
import type { AttachmentPreviewItem } from "@/models/attachments";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, LocaleCode, TranslateFn, UsersById } from "@/models/conversations";
import type { CitationSelection } from "@/components/business/MessageContent";
import { CHAT_SIDE_PANEL_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";
import { FilesTab } from "./FilesTab";
import { BrowserTab } from "./BrowserTab";
import type { ChatFileScope, ReferenceFile } from "./types";
import type { ChatSidePanelController } from "./useChatSidePanel";

export type ChatSidePanelProps = {
  controller: ChatSidePanelController;
  fileScope: ChatFileScope;
  conversation: IMConversation;
  agents: AgentLike[];
  usersById: UsersById;
  locale: LocaleCode;
  t: TranslateFn;
  onReference: ReferenceFile;
  onPreview: (item: AttachmentPreviewItem) => void;
  onSources: (selection: CitationSelection) => void;
  onCollapse: () => void;
  activeThreadRootID?: string;
  threadPanel?: ReactNode;
  onThreadSelect: (rootID: string) => void;
  onThreadClose: () => void;
};

function readWidth() {
  try {
    return Number(localStorage.getItem(CHAT_SIDE_PANEL_WIDTH_STORAGE_KEY)) || 560;
  } catch {
    return 560;
  }
}

export function ChatSidePanel({
  controller,
  fileScope,
  conversation,
  agents,
  usersById,
  locale,
  t,
  onReference,
  onPreview,
  onSources,
  onCollapse,
  activeThreadRootID,
  threadPanel,
  onThreadSelect,
  onThreadClose,
}: ChatSidePanelProps) {
  const panelRef = useRef<HTMLElement>(null);
  const [width, setWidth] = useState(readWidth);
  const [containerWidth, setContainerWidth] = useState(0);
  const [maximized, setMaximized] = useState(false);
  const [resizing, setResizing] = useState(false);
  const drag = useRef<{ pointerID: number; x: number; width: number } | null>(null);
  const { state, patch, openTab, closeTab, replaceTab } = controller;
  const overlay = containerWidth < 680;
  useEffect(() => {
    setMaximized(false);
  }, [conversation.id]);
  useEffect(() => {
    if (!controller.state.open) setMaximized(false);
  }, [controller.state.open]);
  const actualWidth = clampChatSideWidth(width, containerWidth);
  const active = state.tabs.find((tab) => tab.id === state.activeID);
  useEffect(() => {
    if (state.open)
      panelRef.current
        ?.querySelector('[role="tab"][aria-selected="true"]')
        ?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [state.open, state.activeID, actualWidth, containerWidth, overlay, maximized]);
  useEffect(() => {
    const parent = panelRef.current?.parentElement;
    if (!parent) return;
    const measure = () => setContainerWidth(parent.getBoundingClientRect().width);
    measure();
    const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(measure) : null;
    observer?.observe(parent);
    window.addEventListener("resize", measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);
  const threadRoot = active?.kind === "thread" ? active.rootID : "";
  const threadActivation = useRef<{ roomID: string; rootID: string } | null>(null);
  useEffect(() => {
    const rootID = state.open ? threadRoot : "";
    const previous = threadActivation.current;
    const firstForRoom = previous?.roomID !== conversation.id;
    const changed = firstForRoom || previous.rootID !== rootID;
    threadActivation.current = { roomID: conversation.id, rootID };
    // Restore on activation only. A root selected by the main conversation wins
    // while its tab catches up, including a persisted tab on initial mounting.
    if (changed && rootID && activeThreadRootID !== rootID && (!firstForRoom || !activeThreadRootID))
      onThreadSelect(rootID);
  }, [state.open, threadRoot, onThreadSelect, activeThreadRootID, conversation.id]);

  useEffect(() => {
    if (!state.open) return;
    const parent = panelRef.current?.parentElement;
    const siblings = Array.from(parent?.children ?? []).filter(
      (node): node is HTMLElement => node instanceof HTMLElement && node !== panelRef.current,
    );
    if (maximized || overlay) {
      siblings.forEach((node) => (node.inert = true));
      if (!panelRef.current?.contains(document.activeElement))
        panelRef.current
          ?.querySelector<HTMLButtonElement>('[role="tab"][aria-selected="true"], .chat-side-layout-actions button')
          ?.focus({ preventScroll: true });
    }
    const handle = (event: KeyboardEvent) => {
      if (
        event.defaultPrevented ||
        event.key !== "Escape" ||
        document.querySelector('[data-state="open"][role="menu"], [role="dialog"]')
      )
        return;
      const focused = document.activeElement;
      if (focused instanceof Element && !parent?.contains(focused)) return;
      event.preventDefault();
      if (maximized) setMaximized(false);
      else onCollapse();
    };
    window.addEventListener("keydown", handle);
    return () => {
      window.removeEventListener("keydown", handle);
      siblings.forEach((node) => (node.inert = false));
    };
  }, [state.open, maximized, overlay, onCollapse]);
  function close(tab: ChatSideTab) {
    closeTab(tab.id);
    if (tab.kind === "thread") onThreadClose();
    requestAnimationFrame(() =>
      panelRef.current
        ?.querySelector<HTMLButtonElement>('.chat-side-tabs [aria-selected="true"], .chat-side-welcome button')
        ?.focus({ preventScroll: true }),
    );
  }
  function closeAll() {
    const hasThread = state.tabs.some((tab) => tab.kind === "thread");
    patch({ tabs: [], activeID: "" });
    if (hasThread) onThreadClose();
  }
  function persistWidth(next: number) {
    setWidth(next);
    try {
      localStorage.setItem(CHAT_SIDE_PANEL_WIDTH_STORAGE_KEY, String(next));
    } catch {
      /* Width remains usable without storage. */
    }
  }
  function newTab(kind: "files" | "activity" | "browser", previousID?: string) {
    const tab: ChatSideTab =
      kind === "browser" ? { id: previousID ?? crypto.randomUUID(), kind, url: "" } : { id: kind, kind };
    if (previousID) replaceTab(tab, previousID);
    else openTab(tab);
  }
  const iconFor = (tab: ChatSideTab) =>
    tab.kind === "new"
      ? LayoutGrid
      : tab.kind === "files"
        ? Folder
        : tab.kind === "browser"
          ? Globe
          : tab.kind === "activity"
            ? Activity
            : tab.kind === "thread"
              ? MessageSquare
              : tab.kind === "sources"
                ? BookOpen
                : documentPreviewKind({ name: tab.name, mediaType: tab.mediaType }) === "html"
                  ? Globe
                  : FileText;
  const labelFor = (tab: ChatSideTab) =>
    tab.kind === "new"
      ? t("chatSideNewTitle")
      : tab.kind === "files"
        ? t("chatFiles")
        : tab.kind === "activity"
          ? t("conversationActivityOpen")
          : tab.kind === "thread"
            ? t("threadPanelTitle")
            : tab.kind === "sources"
              ? t("citationDrawerTitle")
              : tab.kind === "file"
                ? tab.name
                : tab.url
                  ? new URL(tab.url).hostname
                  : t("chatBrowser");
  return (
    <aside
      ref={panelRef}
      hidden={!state.open}
      className={`chat-side-panel${overlay ? " is-overlay" : ""}${maximized ? " is-maximized" : ""}${resizing ? " is-resizing" : ""}`}
      style={{ "--chat-side-width": `${actualWidth}px` } as CSSProperties}
      aria-label={t("chatSidePanel")}
    >
      {!overlay && !maximized ? (
        <div
          className="chat-side-resizer"
          role="separator"
          tabIndex={0}
          aria-label={t("chatSideResize")}
          aria-orientation="vertical"
          aria-valuemin={320}
          aria-valuemax={Math.max(320, containerWidth - 360)}
          aria-valuenow={actualWidth}
          onPointerDown={(event) => {
            if (event.button !== 0) return;
            drag.current = { pointerID: event.pointerId, x: event.clientX, width: actualWidth };
            event.currentTarget.setPointerCapture(event.pointerId);
            setResizing(true);
          }}
          onPointerMove={(event) => {
            if (drag.current?.pointerID === event.pointerId)
              setWidth(clampChatSideWidth(drag.current.width + drag.current.x - event.clientX, containerWidth));
          }}
          onPointerUp={() => {
            drag.current = null;
            setResizing(false);
            persistWidth(actualWidth);
          }}
          onPointerCancel={() => {
            drag.current = null;
            setResizing(false);
          }}
          onKeyDown={(event) => {
            if (["ArrowLeft", "ArrowRight", "Home"].includes(event.key)) {
              event.preventDefault();
              persistWidth(
                clampChatSideWidth(
                  event.key === "Home" ? 560 : actualWidth + (event.key === "ArrowLeft" ? 24 : -24),
                  containerWidth,
                ),
              );
            }
          }}
        />
      ) : null}
      <header className="chat-side-header">
        <div className="chat-side-leading-actions">
          <Tooltip content={t("chatSideNew")} contentProps={{ side: "bottom" }}>
            <Button
              className="chat-side-new"
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatSideNew")}
              onClick={() => openTab({ id: crypto.randomUUID(), kind: "new" })}
            >
              <Plus size={18} />
            </Button>
          </Tooltip>
          <Tooltip content={t("chatSideCloseAll")} contentProps={{ side: "bottom" }}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatSideCloseAll")}
              disabled={!state.tabs.length}
              onClick={closeAll}
            >
              <X size={17} />
            </Button>
          </Tooltip>
        </div>
        <div
          className="chat-side-tabs"
          role="tablist"
          aria-label={t("chatSideTabs")}
          onKeyDown={(event) => {
            if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
            if (!state.tabs.length) return;
            event.preventDefault();
            const index = Math.max(
              0,
              state.tabs.findIndex((tab) => tab.id === state.activeID),
            );
            const next =
              event.key === "Home"
                ? 0
                : event.key === "End"
                  ? state.tabs.length - 1
                  : (index + (event.key === "ArrowRight" ? 1 : -1) + state.tabs.length) % state.tabs.length;
            const tab = state.tabs[next];
            if (tab) {
              patch({ activeID: tab.id });
              panelRef.current?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
            }
          }}
        >
          {state.tabs.map((tab) => {
            const Icon = iconFor(tab);
            return (
              <div className={`chat-side-tab${tab.id === state.activeID ? " is-active" : ""}`} key={tab.id}>
                <Tooltip content={labelFor(tab)} contentProps={{ side: "bottom" }}>
                  <button
                    type="button"
                    role="tab"
                    aria-selected={tab.id === state.activeID}
                    tabIndex={tab.id === state.activeID ? 0 : -1}
                    id={`chat-tab-label-${encodeURIComponent(tab.id)}`}
                    aria-controls={`chat-tab-${encodeURIComponent(tab.id)}`}
                    onClick={() => patch({ activeID: tab.id })}
                  >
                    {tab.kind === "files" ? (
                      <span className="chat-side-folder-icon" aria-hidden="true" />
                    ) : tab.kind === "file" ? (
                      <span className="chat-side-file-icon" aria-hidden="true" />
                    ) : (
                      <Icon size={15} aria-hidden="true" />
                    )}
                    <span>{labelFor(tab)}</span>
                  </button>
                </Tooltip>
                <Tooltip content={t("chatCloseTab", { name: labelFor(tab) })} contentProps={{ side: "bottom" }}>
                  <Button
                    iconOnly
                    size="sm"
                    variant="tertiaryGray"
                    className="chat-side-tab-close"
                    aria-label={t("chatCloseTab", { name: labelFor(tab) })}
                    onClick={() => close(tab)}
                  >
                    <X size={13} />
                  </Button>
                </Tooltip>
              </div>
            );
          })}
        </div>
        <div className="chat-side-layout-actions">
          <Tooltip content={t(maximized ? "chatSideRestore" : "chatSideMaximize")} contentProps={{ side: "bottom" }}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t(maximized ? "chatSideRestore" : "chatSideMaximize")}
              onClick={() => setMaximized((value) => !value)}
            >
              {maximized ? <Minimize2 size={17} /> : <Maximize2 size={17} />}
            </Button>
          </Tooltip>
          <Tooltip content={t("chatSideCollapse")} contentProps={{ side: "bottom" }}>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatSideCollapse")}
              onClick={() => {
                setMaximized(false);
                onCollapse();
              }}
            >
              <PanelRightClose size={17} />
            </Button>
          </Tooltip>
        </div>
      </header>
      <div className="chat-side-content">
        {!state.tabs.length ? <WelcomeTab t={t} onSelect={newTab} /> : null}
        {state.tabs.map((tab) => (
          <TabBody key={tab.id} tab={tab} active={tab.id === state.activeID && state.open}>
            {tab.kind === "new" ? (
              <WelcomeTab t={t} onSelect={(kind) => newTab(kind, tab.id)} />
            ) : tab.kind === "files" ? (
              <FilesTab
                scope={fileScope}
                controller={controller}
                onPreview={onPreview}
                onReference={onReference}
                t={t}
              />
            ) : tab.kind === "browser" ? (
              <BrowserTab
                url={tab.url}
                active={tab.id === state.activeID && state.open}
                onNavigate={(url) => replaceTab({ ...tab, url })}
                onPopup={(url) => openTab({ id: crypto.randomUUID(), kind: "browser", url })}
                t={t}
              />
            ) : tab.kind === "activity" ? (
              <ConversationActivityPanel
                embedded
                agents={agents}
                conversation={conversation}
                initialEntryID={tab.entryID}
                locale={locale}
                t={t}
                usersById={usersById}
                onClose={() => close(tab)}
                onCitationSelect={onSources}
              />
            ) : tab.kind === "file" ? (
              <FileTab
                tab={tab}
                roomID={conversation.id}
                fileScope={fileScope}
                active={tab.id === state.activeID && state.open}
                t={t}
                onClose={() => close(tab)}
                onReference={onReference}
              />
            ) : tab.kind === "thread" ? (
              (threadPanel ?? <div className="chat-side-status">{t("loading")}</div>)
            ) : (
              <CitationSources
                variant="embedded"
                activeID={tab.activeID}
                cited={
                  tab.cited ??
                  [
                    ...parseCitationDocument(
                      conversation.messages.find((message) => message.id === tab.messageID)?.content ?? "",
                    ).sources.values(),
                  ].map((source) => ({ ...source, number: Number(source.id) }))
                }
                t={t}
                onActiveChange={(id) => (id ? replaceTab({ ...tab, activeID: id }) : close(tab))}
              />
            )}
          </TabBody>
        ))}
      </div>
    </aside>
  );
}

function WelcomeTab({ t, onSelect }: { t: TranslateFn; onSelect: (kind: "files" | "browser" | "activity") => void }) {
  return (
    <div className="chat-side-welcome">
      {(["files", "browser", "activity"] as const).map((kind) => {
        const Icon = kind === "files" ? FolderIcon : kind === "browser" ? Globe : Activity;
        return (
          <button key={kind} className={`chat-side-welcome-${kind}`} type="button" onClick={() => onSelect(kind)}>
            <span className="chat-side-welcome-icon">
              <Icon size={18} />
            </span>
            <span className="chat-side-welcome-content">
              <strong>
                {t(
                  kind === "files"
                    ? "chatWorkspaceFiles"
                    : kind === "browser"
                      ? "chatBrowser"
                      : "conversationActivityOpen",
                )}
              </strong>
              <small>
                {t(kind === "files" ? "chatFilesHint" : kind === "browser" ? "chatBrowserEmpty" : "chatActivityHint")}
              </small>
            </span>
          </button>
        );
      })}
    </div>
  );
}

function TabBody({ tab, active, children }: { tab: ChatSideTab; active: boolean; children: ReactNode }) {
  const [visited, setVisited] = useState(active);
  useEffect(() => {
    if (active) setVisited(true);
  }, [active]);
  return (
    <div
      className="chat-side-tab-body"
      role="tabpanel"
      id={`chat-tab-${encodeURIComponent(tab.id)}`}
      aria-labelledby={`chat-tab-label-${encodeURIComponent(tab.id)}`}
      data-side-tab-id={tab.id}
      hidden={!active}
    >
      {visited ? children : null}
    </div>
  );
}

function FileTab({
  tab,
  roomID,
  fileScope,
  active,
  t,
  onClose,
  onReference,
}: {
  tab: Extract<ChatSideTab, { kind: "file" }>;
  roomID: string;
  fileScope: ChatFileScope;
  active: boolean;
  t: TranslateFn;
  onClose: () => void;
  onReference: ReferenceFile;
}) {
  const reader = useRoomFileReader(fileScope);
  const attachmentID = tab.reference?.source === "attachment" ? tab.reference.attachment_id : "";
  const needsCapability = Boolean(attachmentID && !reader.desktop && !tab.draft);
  const known = useMemo(
    () =>
      transcriptRoomFiles([...(fileScope.messages ?? []), ...(fileScope.retainedMessages ?? [])]).find(
        (file) => file.id === attachmentID,
      ),
    [fileScope.messages, fileScope.retainedMessages, attachmentID],
  );
  const attachment = useQuery({
    queryKey: ["chat-file-preview-source", roomID, attachmentID],
    queryFn: () => reader.resolve(attachmentID),
    enabled: needsCapability && !known,
    retry: false,
  });
  const resolved = known ?? attachment.data;
  const candidate = useMemo(() => {
    if (tab.draft) return tab.draft;
    if (!tab.reference) return undefined;
    if (needsCapability && (!resolved || !attachmentCapability(resolved))) return undefined;
    return filePreviewItem(
      roomID,
      tab.reference,
      tab.sizeBytes,
      tab.mediaType,
      resolved ? attachmentCapability(resolved) : "",
    );
  }, [tab, roomID, needsCapability, resolved]);
  const [current, setCurrent] = useState(candidate);
  useEffect(() => {
    setCurrent((previous) =>
      previous?.id === candidate?.id &&
      previous?.previewURL === candidate?.previewURL &&
      previous?.downloadURL === candidate?.downloadURL &&
      previous?.name === candidate?.name &&
      previous?.mediaType === candidate?.mediaType &&
      previous?.sizeBytes === candidate?.sizeBytes &&
      previous?.file === candidate?.file
        ? previous
        : candidate,
    );
  }, [candidate]);
  return current ? (
    <DocumentPreviewPanel
      items={[current]}
      index={0}
      mode="embedded"
      active={active}
      t={t}
      onClose={onClose}
      toolbarAccessory={
        tab.reference?.source === "workspace" ? (
          <WorkspacePathActions
            agentID={tab.reference.agent_id}
            roomID={roomID}
            path={tab.reference.path}
            active={active}
            t={t}
          />
        ) : undefined
      }
      onIndexChange={() => {}}
      onReference={tab.reference ? () => onReference(tab.reference!) : undefined}
    />
  ) : attachment.isLoading ? (
    <div className="chat-side-status">{t("loading")}</div>
  ) : (
    <div className="chat-side-status" role="alert">
      {t("attachmentPreviewFailed")}
      <Button onClick={() => void attachment.refetch()}>{t("retry")}</Button>
    </div>
  );
}
