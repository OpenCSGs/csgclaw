import type {
  BrowserBounds,
  DesktopBrowserRequest,
} from "../shared/desktopBrowser.types";

export function browserURL(value: unknown, appOrigin: string): string {
  if (typeof value !== "string") throw new Error("Browser URL is invalid.");
  const url = new URL(value);
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.origin === appOrigin
  ) {
    throw new Error("Browser URL must be an independent HTTP or HTTPS page.");
  }
  return url.href;
}

export function parseBrowserRequest(value: unknown): DesktopBrowserRequest {
  if (!value || typeof value !== "object")
    throw new Error("Browser request is invalid.");
  const request = value as DesktopBrowserRequest;
  if (typeof request.id !== "string" || !request.id || request.id.length > 256)
    throw new Error("Browser identity is invalid.");
  if (request.action === "navigate") {
    if (typeof request.url !== "string")
      throw new Error("Browser navigation is invalid.");
  } else if (request.action === "sync") {
    if (
      typeof request.url !== "string" ||
      typeof request.visible !== "boolean" ||
      !request.bounds ||
      ![
        request.bounds.x,
        request.bounds.y,
        request.bounds.width,
        request.bounds.height,
      ].every(Number.isFinite) ||
      request.bounds.width < 0 ||
      request.bounds.height < 0
    )
      throw new Error("Browser layout is invalid.");
  } else if (!["reload", "back", "forward", "close"].includes(request.action))
    throw new Error("Browser action is invalid.");
  return request;
}

export function clipBrowserBounds(
  bounds: BrowserBounds,
  width: number,
  height: number,
): BrowserBounds {
  const x = Math.max(0, Math.min(width, Math.round(bounds.x)));
  const y = Math.max(0, Math.min(height, Math.round(bounds.y)));
  return {
    x,
    y,
    width: Math.max(
      0,
      Math.min(width - x, Math.round(bounds.x + bounds.width) - x),
    ),
    height: Math.max(
      0,
      Math.min(height - y, Math.round(bounds.y + bounds.height) - y),
    ),
  };
}
