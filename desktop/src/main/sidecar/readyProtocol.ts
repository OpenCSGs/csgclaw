import { DESKTOP_PROTOCOL_VERSION, DesktopMessageType, type DesktopReadyMessage } from "./contract";

export function parseReadyMessage(
  line: string,
  expectedInstanceID: string,
  expectedPID: number,
): DesktopReadyMessage {
  let source: unknown;
  try {
    source = JSON.parse(line);
  } catch {
    throw new Error("Go sidecar emitted an invalid ready message.");
  }
  if (!source || typeof source !== "object" || Array.isArray(source)) {
    throw new Error("Go sidecar ready message must be an object.");
  }
  const value = source as Record<string, unknown>;
  if (value.type !== DesktopMessageType.ready || value.protocol_version !== DESKTOP_PROTOCOL_VERSION) {
    throw new Error("Go sidecar desktop protocol is incompatible.");
  }
  if (value.instance_id !== expectedInstanceID || value.pid !== expectedPID) {
    throw new Error("Go sidecar ready identity does not match the spawned process.");
  }
  if (value.distribution !== "electron" || typeof value.version !== "string" || !value.version.trim()) {
    throw new Error("Go sidecar distribution or version is invalid.");
  }
  if (typeof value.base_url !== "string") {
    throw new Error("Go sidecar base URL is missing.");
  }

  const baseURL = new URL(value.base_url);
  if (
    baseURL.protocol !== "http:" ||
    baseURL.hostname !== "127.0.0.1" ||
    !baseURL.port ||
    baseURL.username ||
    baseURL.password ||
    baseURL.pathname !== "/" ||
    baseURL.search ||
    baseURL.hash
  ) {
    throw new Error("Go sidecar returned an unsafe base URL.");
  }

  return {
    type: DesktopMessageType.ready,
    protocol_version: DESKTOP_PROTOCOL_VERSION,
    instance_id: expectedInstanceID,
    pid: expectedPID,
    base_url: baseURL.origin,
    version: value.version,
    distribution: "electron",
  };
}

export function assertCompatibleVersions(appVersion: string, backendVersion: string, packaged: boolean): void {
  if (!packaged || isDevelopmentVersion(appVersion) || isDevelopmentVersion(backendVersion)) {
    return;
  }
  if (normalizeVersion(appVersion) !== normalizeVersion(backendVersion)) {
    throw new Error(`Desktop ${appVersion} cannot start bundled backend ${backendVersion}.`);
  }
}

function normalizeVersion(version: string): string {
  return version.trim().replace(/^v/, "").split(/[+-]/, 1)[0] || "";
}

function isDevelopmentVersion(version: string): boolean {
  const value = version.trim().toLowerCase();
  return !value || value === "dev" || value.includes("development") || value.includes("local");
}
