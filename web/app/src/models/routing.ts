// @ts-nocheck
import { WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY } from "@/shared/storage/keys";
import { WORKSPACE_TAB_AGENTS, WORKSPACE_TAB_HUB, WORKSPACE_TAB_MESSAGES } from "@/bootstrap/constants";
import { isDirectConversation } from "@/models/conversations";

export function paneFromLocation(pathname = window.location.pathname) {
  const parts = String(pathname || "/")
    .split("/")
    .filter(Boolean)
    .map(decodePathSegment);
  const section = parts[0] || "";
  const id = parts[1] || "";
  switch (section) {
    case "computer":
      return { type: "computer", id: "local" };
    case "agents":
    case "agent":
      return id ? { type: "agent", id } : { type: "computer", id: "local" };
    case "hub":
      return { type: "hub", id: "hub" };
    case "channels":
    case "channel":
    case "dms":
    case "dm":
    case "rooms":
    case "room":
    case "conversations":
    case "conversation":
      return id ? { type: "conversation", id } : { type: "conversation", id: "" };
    default:
      return { type: "conversation", id: "" };
  }
}

export function pathForPane(pane, rooms = []) {
  if (!pane || pane.type === "computer") {
    return "/computer";
  }
  if (pane.type === "agent" && pane.id) {
    return `/agents/${encodeURIComponent(pane.id)}`;
  }
  if (pane.type === "hub") {
    return "/hub";
  }
  if (pane.type === "conversation" && pane.id) {
    const room = rooms.find((item) => item.id === pane.id);
    const prefix = room && isDirectConversation(room) ? "/dms/" : "/rooms/";
    return `${prefix}${encodeURIComponent(pane.id)}`;
  }
  return "/";
}

export function syncBrowserPath(pane, rooms, mode = "push") {
  const nextPath = pathForPane(pane, rooms);
  if (!nextPath || window.location.pathname === nextPath) {
    return;
  }
  const state = { pane };
  if (mode === "replace") {
    window.history.replaceState(state, "", nextPath);
    return;
  }
  window.history.pushState(state, "", nextPath);
}

export function decodePathSegment(value) {
  try {
    return decodeURIComponent(value || "");
  } catch (_) {
    return value || "";
  }
}

export function workspaceTabForPane(pane) {
  if (pane?.type === "hub") {
    return WORKSPACE_TAB_HUB;
  }
  if (pane?.type === "agent" || pane?.type === "computer") {
    return WORKSPACE_TAB_AGENTS;
  }
  return WORKSPACE_TAB_MESSAGES;
}

export function readCollapsedWorkspaceGroups() {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY) || "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return {};
    }
    return parsed;
  } catch (_) {
    return {};
  }
}
