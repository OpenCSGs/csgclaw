import type { Session } from "electron";

const allowedPermissions = new Set(["notifications", "clipboard-sanitized-write"]);

export function installPermissionPolicy(session: Session, allowedOrigin: string): void {
  session.setPermissionRequestHandler((webContents, permission, callback, details) => {
    const origin = safeOrigin(details.requestingUrl || webContents.getURL());
    callback(origin === allowedOrigin && allowedPermissions.has(permission));
  });
  session.setPermissionCheckHandler((_webContents, permission, requestingOrigin) => {
    const origin = safeOrigin(requestingOrigin);
    return origin === allowedOrigin && allowedPermissions.has(permission);
  });
}

function safeOrigin(rawURL: string): string {
  try {
    return new URL(rawURL).origin;
  } catch {
    return "";
  }
}
