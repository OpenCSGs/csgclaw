export type BrowserBounds = {
  x: number;
  y: number;
  width: number;
  height: number;
};
export type DesktopBrowserRequest = { id: string } & (
  | { action: "sync"; url: string; visible: boolean; bounds: BrowserBounds }
  | { action: "navigate"; url: string }
  | { action: "reload" | "back" | "forward" | "close" }
);
export type DesktopBrowserState = {
  id: string;
  url: string;
  loading: boolean;
  canGoBack: boolean;
  canGoForward: boolean;
  error?: string;
  popupURL?: string;
  shortcut?: "escape";
};
