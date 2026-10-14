import type { AttachmentPreviewItem } from "./attachments";
import type { CitationSource } from "./citations";
import type { ComposerFileReference } from "./fileReferences";
import { normalizeBrowserURL } from "./fileReferences";

export type ChatSideTab =
  | { id: string; kind: "new" }
  | { id: string; kind: "files" }
  | { id: string; kind: "activity"; entryID?: string }
  | { id: string; kind: "browser"; url: string }
  | {
      id: string;
      kind: "file";
      name: string;
      sizeBytes: number;
      mediaType: string;
      reference?: ComposerFileReference;
      draft?: AttachmentPreviewItem;
    }
  | { id: string; kind: "thread"; rootID: string }
  | {
      id: string;
      kind: "sources";
      activeID: string;
      messageID: string;
      cited?: (CitationSource & { number: number })[];
    };

export type ChatSideState = {
  tabs: ChatSideTab[];
  activeID: string;
  open: boolean;
  agentID: string;
  directory: string;
  fileSource: "workspace" | "attachments";
};

export function initialChatSideState(): ChatSideState {
  return { tabs: [], activeID: "", open: false, agentID: "", directory: "", fileSource: "workspace" };
}

export function openChatSideTab(state: ChatSideState, tab: ChatSideTab): ChatSideState {
  const existing = state.tabs.some((item) => item.id === tab.id);
  return {
    ...state,
    open: true,
    activeID: tab.id,
    tabs: existing ? state.tabs.map((item) => (item.id === tab.id ? tab : item)) : [...state.tabs, tab],
  };
}

export function replaceChatSideTab(state: ChatSideState, previousID: string, tab: ChatSideTab): ChatSideState {
  const existing = state.tabs.find((item) => item.id === tab.id && item.id !== previousID);
  if (existing) return openChatSideTab(closeChatSideTab(state, previousID), existing);
  return {
    ...state,
    tabs: state.tabs.map((item) => (item.id === previousID ? tab : item)),
    activeID: state.activeID === previousID ? tab.id : state.activeID,
  };
}

export function closeChatSideTab(state: ChatSideState, id: string): ChatSideState {
  const index = state.tabs.findIndex((tab) => tab.id === id);
  const tabs = state.tabs.filter((tab) => tab.id !== id);
  return {
    ...state,
    tabs,
    activeID: state.activeID === id ? (tabs[Math.min(index, tabs.length - 1)]?.id ?? "") : state.activeID,
  };
}

export function serializeChatSideState(state: ChatSideState): string {
  const tabs = state.tabs
    .filter((tab) => tab.kind !== "file" || !tab.draft)
    .map((tab) => {
      if (tab.kind === "sources") return { ...tab, cited: undefined };
      return tab;
    });
  return JSON.stringify({
    ...state,
    tabs,
    activeID: tabs.some((tab) => tab.id === state.activeID) ? state.activeID : (tabs[0]?.id ?? ""),
  });
}

export function restoreChatSideState(value: string | null): ChatSideState {
  const empty = initialChatSideState();
  try {
    const parsed: unknown = JSON.parse(value ?? "null");
    if (!parsed || typeof parsed !== "object") return empty;
    const data = parsed as Record<string, unknown>;
    const tabs: ChatSideTab[] = [];
    for (const entry of Array.isArray(data.tabs) ? data.tabs : []) {
      if (
        !entry ||
        typeof entry !== "object" ||
        typeof entry.id !== "string" ||
        tabs.some((tab) => tab.id === entry.id)
      )
        continue;
      const tab = entry as ChatSideTab;
      if (tab.kind === "new" || tab.kind === "files" || tab.kind === "activity") tabs.push(tab);
      else if (tab.kind === "browser" && typeof tab.url === "string") {
        const url = tab.url ? normalizeBrowserURL(tab.url) : "";
        if (url !== null) tabs.push({ ...tab, url });
      } else if (tab.kind === "thread" && typeof tab.rootID === "string") tabs.push(tab);
      else if (tab.kind === "sources" && typeof tab.messageID === "string" && typeof tab.activeID === "string")
        tabs.push(tab);
      else if (
        tab.kind === "file" &&
        typeof tab.name === "string" &&
        typeof tab.mediaType === "string" &&
        typeof tab.sizeBytes === "number" &&
        Number.isFinite(tab.sizeBytes) &&
        tab.reference &&
        !tab.draft
      ) {
        const ref = tab.reference;
        if (
          (ref.source === "workspace" && typeof ref.agent_id === "string" && typeof ref.path === "string") ||
          (ref.source === "attachment" && typeof ref.attachment_id === "string")
        )
          if (typeof ref.name === "string" && typeof ref.label === "string")
            tabs.push({
              id: tab.id,
              kind: tab.kind,
              name: tab.name,
              sizeBytes: tab.sizeBytes,
              mediaType: tab.mediaType,
              reference: ref,
            });
      }
    }
    return {
      tabs,
      activeID: tabs.some((tab) => tab.id === data.activeID) ? String(data.activeID) : (tabs[0]?.id ?? ""),
      open: data.open === true,
      agentID: typeof data.agentID === "string" ? data.agentID : "",
      directory: typeof data.directory === "string" ? data.directory : "",
      fileSource: data.fileSource === "attachments" ? "attachments" : "workspace",
    };
  } catch {
    return empty;
  }
}

export function clampChatSideWidth(width: number, containerWidth: number): number {
  return Math.max(320, Math.min(width, Math.max(320, containerWidth - 360)));
}
