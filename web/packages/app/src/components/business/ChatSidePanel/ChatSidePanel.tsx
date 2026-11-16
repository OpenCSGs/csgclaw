import chatSidePanelStyles from "./ChatSidePanel.module.css";
import { createUUID } from "@/shared/lib/uuid";
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
  ListTodo,
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
import { AgentBrowserTab } from "@/components/business/AgentBrowserTab";
import { closeBrowserSession } from "@/api/browser";
import { HTMLBrowserTab } from "./HTMLBrowserTab";
import { useRoomBrowsers } from "./useRoomBrowsers";
import type { ChatFileScope, ReferenceFile } from "./types";
import type { ChatSidePanelController } from "./useChatSidePanel";

export type ChatSidePanelProps = {
  controller: ChatSidePanelController;
  fileScope: ChatFileScope;
  conversation?: IMConversation;
  toolKinds?: readonly ChatToolKind[];
  agents: AgentLike[];
  usersById: UsersById;
  locale: LocaleCode;
  t: TranslateFn;
  onReference: ReferenceFile;
  onPreview: (item: AttachmentPreviewItem) => void;
  onSources?: (selection: CitationSelection) => void;
  onCollapse: () => void;
  activeThreadRootID?: string;
  threadPanel?: ReactNode;
  roomTasksPanel?: ReactNode;
  suspended?: boolean;
  onThreadSelect?: (rootID: string) => void;
  onThreadClose?: () => void;
};

export type ChatToolKind = "files" | "browser" | "activity" | "tasks";

const DEFAULT_PANEL_WIDTH = 496;

function readWidth() {
  try {
    return Number(localStorage.getItem(CHAT_SIDE_PANEL_WIDTH_STORAGE_KEY)) || DEFAULT_PANEL_WIDTH;
  } catch {
    return DEFAULT_PANEL_WIDTH;
  }
}

export function ChatSidePanel({
  controller,
  fileScope,
  conversation,
  toolKinds,
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
  roomTasksPanel,
  suspended = false,
  onThreadSelect,
  onThreadClose,
}: ChatSidePanelProps) {
  const roomID = fileScope.roomID;
  const kinds = toolKinds ?? ["files", "browser", "activity", ...(roomTasksPanel ? ["tasks" as const] : [])];
  const panelRef = useRef<HTMLElement>(null);
  const [width, setWidth] = useState(readWidth);
  const [containerWidth, setContainerWidth] = useState(0);
  const [maximized, setMaximized] = useState(false);
  const [resizing, setResizing] = useState(false);
  const drag = useRef<{ pointerID: number; x: number; width: number } | null>(null);
  const { state, patch, openTab, closeTab, replaceTab } = controller;
  const [browserError, setBrowserError] = useState(false);
  const [closingBrowser, setClosingBrowser] = useState(false);
  const browsers = useRoomBrowsers(roomID);
  const knownBrowsers = useRef({ roomID, ids: new Set<string>() });
  useEffect(() => {
    if (knownBrowsers.current.roomID !== roomID) knownBrowsers.current = { roomID, ids: new Set() };
    if (!browsers.data) return;
    const sessions = browsers.data;
    const added = sessions.filter((session) => !knownBrowsers.current.ids.has(session.id));
    sessions.forEach((session) => knownBrowsers.current.ids.add(session.id));
    patch((current) => {
      let next = current;
      for (const tab of current.tabs) {
        if (tab.kind !== "browser" || !tab.browserID) continue;
        const session = sessions.find((session) => session.id === tab.browserID);
        if (!session) {
          const tabs = next.tabs.filter((item) => item.id !== tab.id);
          next = { ...next, tabs, activeID: next.activeID === tab.id ? (tabs.at(-1)?.id ?? "") : next.activeID };
        } else if (session.url !== tab.url) {
          next = { ...next, tabs: next.tabs.map((item) => (item.id === tab.id ? { ...tab, url: session.url } : item)) };
        }
      }
      for (const session of added) {
        const id = `agent-browser:${session.id}`;
        next = {
          ...next,
          tabs: [
            ...next.tabs.filter((tab) => tab.id !== id),
            { id, kind: "browser", url: session.url, browserID: session.id },
          ],
          activeID: id,
          open: true,
        };
      }
      return next;
    });
  }, [browsers.data, roomID, patch]);
  const visible = state.open && !suspended;
  const overlay = containerWidth < 680;
  useEffect(() => {
    setMaximized(false);
  }, [roomID]);
  useEffect(() => {
    if (!controller.state.open) setMaximized(false);
  }, [controller.state.open]);
  const actualWidth = clampChatSideWidth(width, containerWidth);
  const active = state.tabs.find((tab) => tab.id === state.activeID);
  useEffect(() => {
    if (visible)
      panelRef.current
        ?.querySelector('[role="tab"][aria-selected="true"]')
        ?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [visible, state.activeID, actualWidth, containerWidth, overlay, maximized]);
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
    const firstForRoom = previous?.roomID !== roomID;
    const changed = firstForRoom || previous.rootID !== rootID;
    threadActivation.current = { roomID: roomID, rootID };
    // Restore on activation only. A root selected by the main conversation wins
    // while its tab catches up, including a persisted tab on initial mounting.
    if (changed && rootID && activeThreadRootID !== rootID && (!firstForRoom || !activeThreadRootID))
      onThreadSelect?.(rootID);
  }, [state.open, threadRoot, onThreadSelect, activeThreadRootID, roomID]);

  useEffect(() => {
    if (!visible) return;
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
  }, [visible, maximized, overlay, onCollapse]);
  async function close(tab: ChatSideTab) {
    if (tab.kind === "browser" && tab.browserID) {
      setClosingBrowser(true);
      try {
        await closeBrowserSession(roomID, tab.browserID);
      } catch {
        setBrowserError(true);
        return;
      } finally {
        setClosingBrowser(false);
      }
    }
    closeTab(tab.id);
    if (tab.kind === "thread") onThreadClose?.();
    requestAnimationFrame(() =>
      panelRef.current
        ?.querySelector<HTMLButtonElement>('.chat-side-tabs [aria-selected="true"], .chat-side-welcome button')
        ?.focus({ preventScroll: true }),
    );
  }
  async function closeAll() {
    setClosingBrowser(true);
    try {
      await Promise.all(
        state.tabs
          .filter((tab) => tab.kind === "browser" && tab.browserID)
          .map((tab) => closeBrowserSession(roomID, (tab as Extract<ChatSideTab, { kind: "browser" }>).browserID!)),
      );
    } catch {
      setBrowserError(true);
      return;
    } finally {
      setClosingBrowser(false);
    }
    const hasThread = state.tabs.some((tab) => tab.kind === "thread");
    patch({ tabs: [], activeID: "" });
    if (hasThread) onThreadClose?.();
  }
  function persistWidth(next: number) {
    setWidth(next);
    try {
      localStorage.setItem(CHAT_SIDE_PANEL_WIDTH_STORAGE_KEY, String(next));
    } catch {
      /* Width remains usable without storage. */
    }
  }
  function newTab(kind: "files" | "activity" | "browser" | "tasks", previousID?: string) {
    const tab: ChatSideTab =
      kind === "browser" ? { id: previousID ?? createUUID(), kind, url: "" } : { id: kind, kind };
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
            : tab.kind === "tasks"
              ? ListTodo
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
          : tab.kind === "tasks"
            ? t("roomTasksOpen")
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
      hidden={!visible}
      className={`chat-side-panel${overlay ? " is-overlay" : ""}${maximized ? " is-maximized" : ""}${resizing ? " is-resizing" : ""}`}
      style={{ "--chat-side-width": `${actualWidth}px` } as CSSProperties}
      aria-label={t("chatSidePanel")}
    >
      {!overlay && !maximized ? (
        <div
          className={chatSidePanelStyles["chat-side-resizer"]}
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
                  event.key === "Home" ? DEFAULT_PANEL_WIDTH : actualWidth + (event.key === "ArrowLeft" ? 24 : -24),
                  containerWidth,
                ),
              );
            }
          }}
        />
      ) : null}
      <header className={chatSidePanelStyles["chat-side-header"]}>
        <div className={chatSidePanelStyles["chat-side-leading-actions"]}>
          <Tooltip content={t("chatSideNew")} contentProps={{ side: "bottom" }}>
            <Button
              className="chat-side-new"
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatSideNew")}
              onClick={() => openTab({ id: createUUID(), kind: "new" })}
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
              disabled={!state.tabs.length || closingBrowser}
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
              <div
                className={`${chatSidePanelStyles["chat-side-tab"]}${tab.id === state.activeID ? " is-active" : ""}`}
                key={tab.id}
              >
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
                      <span className={chatSidePanelStyles["chat-side-folder-icon"]} aria-hidden="true" />
                    ) : tab.kind === "file" ? (
                      <span className={chatSidePanelStyles["chat-side-file-icon"]} aria-hidden="true" />
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
                    className={chatSidePanelStyles["chat-side-tab-close"]}
                    aria-label={t("chatCloseTab", { name: labelFor(tab) })}
                    disabled={closingBrowser}
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
      <div className={chatSidePanelStyles["chat-side-content"]}>
        {browserError ? (
          <div role="alert" className={chatSidePanelStyles["chat-side-status"]}>
            <span>{t("chatAgentBrowserFailed")}</span>
            <Button
              size="sm"
              variant="secondaryGray"
              onClick={() => {
                setBrowserError(false);
                void browsers.refetch();
              }}
            >
              {t("chatBrowserRetry")}
            </Button>
          </div>
        ) : null}
        {!state.tabs.length ? <WelcomeTab t={t} onSelect={newTab} kinds={kinds} /> : null}
        {state.tabs.map((tab) => (
          <TabBody key={tab.id} tab={tab} active={tab.id === state.activeID && visible}>
            {tab.kind === "new" ? (
              <WelcomeTab t={t} onSelect={(kind) => newTab(kind, tab.id)} kinds={kinds} />
            ) : tab.kind === "tasks" ? (
              roomTasksPanel
            ) : tab.kind === "files" ? (
              <FilesTab
                key={fileScope.roomID || JSON.stringify([fileScope.defaultAgentID, fileScope.workspaceDir])}
                scope={fileScope}
                controller={controller}
                onPreview={onPreview}
                onReference={onReference}
                t={t}
              />
            ) : tab.kind === "browser" ? (
              tab.browserID ? (
                <AgentBrowserTab
                  roomID={roomID}
                  session={
                    browsers.data?.find((session) => session.id === tab.browserID) ?? {
                      id: tab.browserID,
                      room_id: roomID,
                      url: tab.url,
                      title: "",
                      revision: 0,
                      width: 1280,
                      height: 800,
                      can_go_back: false,
                      can_go_forward: false,
                    }
                  }
                  active={tab.id === state.activeID && visible}
                  t={t}
                />
              ) : (
                <BrowserTab
                  url={tab.url}
                  active={tab.id === state.activeID && visible}
                  onNavigate={(url) => replaceTab({ ...tab, url })}
                  onPopup={(url) => openTab({ id: createUUID(), kind: "browser", url })}
                  t={t}
                />
              )
            ) : tab.kind === "activity" ? (
              conversation ? (
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
              ) : null
            ) : tab.kind === "file" ? (
              <FileTab
                tab={tab}
                roomID={roomID}
                fileScope={fileScope}
                active={tab.id === state.activeID && visible}
                t={t}
                onClose={() => close(tab)}
                onReference={onReference}
                onPopup={(url) => openTab({ id: createUUID(), kind: "browser", url })}
              />
            ) : tab.kind === "thread" ? (
              (threadPanel ?? <div className={chatSidePanelStyles["chat-side-status"]}>{t("loading")}</div>)
            ) : (
              <CitationSources
                variant="embedded"
                activeID={tab.activeID}
                cited={
                  tab.cited ??
                  [
                    ...parseCitationDocument(
                      conversation?.messages.find((message) => message.id === tab.messageID)?.content ?? "",
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

function WelcomeTab({
  t,
  onSelect,
  kinds,
}: {
  t: TranslateFn;
  onSelect: (kind: "files" | "browser" | "activity" | "tasks") => void;
  kinds: readonly ChatToolKind[];
}) {
  return (
    <div className="chat-side-welcome">
      {kinds.map((kind) => {
        const Icon =
          kind === "files" ? FolderIcon : kind === "browser" ? Globe : kind === "tasks" ? ListTodo : Activity;
        return (
          <button key={kind} className={`chat-side-welcome-${kind}`} type="button" onClick={() => onSelect(kind)}>
            <span className={chatSidePanelStyles["chat-side-welcome-icon"]}>
              <Icon size={18} />
            </span>
            <span className={chatSidePanelStyles["chat-side-welcome-content"]}>
              <strong>
                {t(
                  kind === "files"
                    ? "chatWorkspaceFiles"
                    : kind === "browser"
                      ? "chatBrowser"
                      : kind === "tasks"
                        ? "roomTasksOpen"
                        : "conversationActivityOpen",
                )}
              </strong>
              <small>
                {t(
                  kind === "files"
                    ? "chatFilesHint"
                    : kind === "browser"
                      ? "chatBrowserEmpty"
                      : kind === "tasks"
                        ? "roomTasksPanelHint"
                        : "chatActivityHint",
                )}
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
      className={chatSidePanelStyles["chat-side-tab-body"]}
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
  onPopup,
}: {
  tab: Extract<ChatSideTab, { kind: "file" }>;
  roomID: string;
  fileScope: ChatFileScope;
  active: boolean;
  t: TranslateFn;
  onClose: () => void;
  onReference: ReferenceFile;
  onPopup: (url: string) => void;
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
      fileScope.workspaceDir,
    );
  }, [tab, roomID, needsCapability, resolved, fileScope.workspaceDir]);
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
  if (current && documentPreviewKind(current) === "html") {
    return (
      <HTMLBrowserTab
        item={current}
        sourceKey={JSON.stringify([roomID, fileScope.workspaceDir ?? "", current.id])}
        active={active}
        onPopup={onPopup}
        t={t}
      />
    );
  }
  return current ? (
    <DocumentPreviewPanel
      items={[current]}
      index={0}
      mode="embedded"
      active={active}
      t={t}
      onClose={onClose}
      toolbarAccessory={
        tab.reference ? (
          <WorkspacePathActions
            roomID={roomID}
            workspaceDir={fileScope.workspaceDir}
            reference={tab.reference}
            token={resolved ? attachmentCapability(resolved) : ""}
            active={active}
            t={t}
          />
        ) : undefined
      }
      onIndexChange={() => {}}
      onReference={tab.reference ? () => onReference(tab.reference!) : undefined}
    />
  ) : attachment.isLoading ? (
    <div className={chatSidePanelStyles["chat-side-status"]}>{t("loading")}</div>
  ) : (
    <div className={chatSidePanelStyles["chat-side-status"]} role="alert">
      {t("attachmentPreviewFailed")}
      <Button onClick={() => void attachment.refetch()}>{t("retry")}</Button>
    </div>
  );
}
