import { request, resolveRequestPath } from "./client";

export type BrowserSession = {
  id: string;
  room_id: string;
  url: string;
  title: string;
  revision: number;
  user_controlled?: boolean;
  width: number;
  height: number;
  can_go_back: boolean;
  can_go_forward: boolean;
};

export type BrowserCommand = {
  action: "navigate" | "back" | "forward" | "reload" | "pointer" | "type" | "key" | "scroll" | "resize";
  url?: string;
  text?: string;
  key?: string;
  x?: number;
  y?: number;
  width?: number;
  height?: number;
};

function browserPath(roomID: string, id?: string) {
  return `/api/v1/rooms/${encodeURIComponent(roomID)}/browsers${id ? `/${encodeURIComponent(id)}` : ""}`;
}

export function fetchBrowserSessions(roomID: string, signal?: AbortSignal) {
  return request<BrowserSession[]>(browserPath(roomID), { signal });
}

export function closeBrowserSession(roomID: string, id: string) {
  return request<void>(browserPath(roomID, id), { method: "DELETE" });
}

const controlHeader = "X-CSGClaw-Browser-Control";
const frameHeader = "X-CSGClaw-Browser-Frame";
export type BrowserControl = { session: BrowserSession; token: string };
export class BrowserControlExpired extends Error {}

export function takeBrowserControl(roomID: string, id: string) {
  return request<BrowserControl>(browserPath(roomID, id), { method: "POST", json: { action: "takeover" } });
}
export function releaseBrowserControl(roomID: string, id: string, token: string) {
  return request<BrowserSession>(browserPath(roomID, id), {
    method: "POST",
    json: { action: "release" },
    headers: { [controlHeader]: token },
  });
}
export function browserCommand(roomID: string, id: string, command: BrowserCommand, token = "", revision = "") {
  return request<BrowserSession>(browserPath(roomID, id), {
    method: "POST",
    json: command,
    headers: { [controlHeader]: token, [frameHeader]: revision },
  });
}

export function browserScreenshotURL(roomID: string, id: string) {
  return resolveRequestPath(browserPath(roomID, id));
}

export async function fetchBrowserFrame(roomID: string, id: string, signal: AbortSignal, token = "") {
  const response = await fetch(browserScreenshotURL(roomID, id), {
    signal,
    cache: "no-store",
    headers: token ? { [controlHeader]: token } : undefined,
  });
  if (response.status === 409) throw new BrowserControlExpired("Browser control expired");
  if (!response.ok) throw new Error(`Browser frame: ${response.status}`);
  return { blob: await response.blob(), revision: response.headers.get(frameHeader) || "" };
}
