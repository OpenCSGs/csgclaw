import { request, resolveRequestPath } from "./client";

export type BrowserSession = {
  id: string;
  room_id: string;
  url: string;
  title: string;
  revision: number;
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

export function browserCommand(roomID: string, id: string, command: BrowserCommand) {
  return request<BrowserSession>(browserPath(roomID, id), { method: "POST", json: command });
}

export function browserScreenshotURL(roomID: string, id: string) {
  return resolveRequestPath(browserPath(roomID, id));
}

export async function fetchBrowserFrame(roomID: string, id: string, signal: AbortSignal) {
  const response = await fetch(browserScreenshotURL(roomID, id), { signal, cache: "no-store" });
  if (!response.ok) throw new Error(`Browser frame: ${response.status}`);
  return response.blob();
}
