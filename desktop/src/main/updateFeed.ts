import type { DesktopUpdateChannel } from "../shared/desktopBridge.types";

export const DEFAULT_UPDATE_CHANNELS_BASE_URL =
  "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/channels";

export type DesktopDownloadsManifest = {
  channel: DesktopUpdateChannel;
  latest: string;
  artifacts: DesktopDownloadArtifact[];
};

export type DesktopDownloadArtifact = {
  platform: string;
  arch: string;
  url: string;
  sizeBytes: number;
  sha256: string;
};

export function desktopUpdateFeedOptions(
  url: string,
  platform: NodeJS.Platform,
): { url: string; serverType?: "json" } {
  return platform === "darwin" ? { url, serverType: "json" } : { url };
}

export function desktopUpdateFeedURL({
  channel,
  platform,
  arch,
  directURL = "",
  channelsBaseURL = DEFAULT_UPDATE_CHANNELS_BASE_URL,
}: {
  channel: DesktopUpdateChannel;
  platform: NodeJS.Platform;
  arch: string;
  directURL?: string;
  channelsBaseURL?: string;
}): string {
  const configured = normalizeHTTPSURL(directURL);
  if (configured) {
    return configured;
  }
  const baseURL = normalizeHTTPSURL(channelsBaseURL);
  if (!baseURL) {
    return "";
  }
  const feedURL = `${baseURL}/${channel}/updates/${platform}/${arch}`;
  return platform === "darwin" ? `${feedURL}/RELEASES.json` : feedURL;
}

export function desktopDownloadsManifestURL({
  channel,
  channelsBaseURL = DEFAULT_UPDATE_CHANNELS_BASE_URL,
}: {
  channel: DesktopUpdateChannel;
  channelsBaseURL?: string;
}): string {
  const baseURL = normalizeHTTPSURL(channelsBaseURL);
  return baseURL ? `${baseURL}/${channel}/downloads.json` : "";
}

export function parseDesktopDownloadsManifest(
  payload: unknown,
  expectedChannel: DesktopUpdateChannel,
): DesktopDownloadsManifest {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("Desktop downloads manifest must be a JSON object.");
  }
  const source = payload as Record<string, unknown>;
  if (source.schema_version !== 1) {
    throw new Error(
      `Unsupported desktop downloads manifest schema: ${String(source.schema_version)}`,
    );
  }
  if (source.channel !== expectedChannel) {
    throw new Error(
      `Desktop downloads manifest channel is ${String(source.channel)}, expected ${expectedChannel}.`,
    );
  }
  const rawLatest =
    typeof source.latest === "string" ? source.latest.trim() : "";
  const latest = rawLatest.replace(/^v/, "");
  if (!latest) {
    throw new Error("Desktop downloads manifest latest version is missing.");
  }
  const versions = source.versions;
  if (
    !versions ||
    typeof versions !== "object" ||
    Array.isArray(versions) ||
    (!(rawLatest in versions) && !(latest in versions))
  ) {
    throw new Error(
      `Desktop downloads manifest latest version ${latest} is missing from versions.`,
    );
  }
  const latestEntry =
    (versions as Record<string, unknown>)[rawLatest] ??
    (versions as Record<string, unknown>)[latest];
  const artifacts = parseDesktopDownloadArtifacts(latestEntry);
  return { channel: expectedChannel, latest, artifacts };
}

export function desktopInstallerArtifact(
  manifest: DesktopDownloadsManifest,
  platform: NodeJS.Platform,
  arch: string,
): DesktopDownloadArtifact | undefined {
  const manifestPlatform =
    platform === "darwin"
      ? "macos"
      : platform === "win32"
        ? "windows"
        : platform;
  const manifestArch = arch === "x64" ? "x86_64" : arch;
  return manifest.artifacts.find(
    (artifact) =>
      artifact.platform === manifestPlatform && artifact.arch === manifestArch,
  );
}

function parseDesktopDownloadArtifacts(
  latestEntry: unknown,
): DesktopDownloadArtifact[] {
  if (
    !latestEntry ||
    typeof latestEntry !== "object" ||
    Array.isArray(latestEntry)
  ) {
    return [];
  }
  const rawArtifacts = (latestEntry as Record<string, unknown>).artifacts;
  if (!Array.isArray(rawArtifacts)) {
    return [];
  }
  return rawArtifacts.map((rawArtifact, index) => {
    if (
      !rawArtifact ||
      typeof rawArtifact !== "object" ||
      Array.isArray(rawArtifact)
    ) {
      throw new Error(`Desktop download artifact ${index} is invalid.`);
    }
    const source = rawArtifact as Record<string, unknown>;
    const platform =
      typeof source.platform === "string" ? source.platform.trim() : "";
    const arch = typeof source.arch === "string" ? source.arch.trim() : "";
    const url =
      typeof source.url === "string" ? normalizeHTTPSURL(source.url) : "";
    const sizeBytes =
      typeof source.size_bytes === "number" &&
      Number.isSafeInteger(source.size_bytes) &&
      source.size_bytes > 0
        ? source.size_bytes
        : 0;
    const sha256 =
      typeof source.sha256 === "string" ? source.sha256.toLowerCase() : "";
    if (
      !platform ||
      !arch ||
      !url ||
      !sizeBytes ||
      !/^[0-9a-f]{64}$/.test(sha256)
    ) {
      throw new Error(`Desktop download artifact ${index} is invalid.`);
    }
    return { platform, arch, url, sizeBytes, sha256 };
  });
}

export function normalizeHTTPSURL(rawURL: string): string {
  const value = rawURL.trim();
  if (!value) {
    return "";
  }
  try {
    const parsed = new URL(value);
    if (
      parsed.protocol !== "https:" ||
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash
    ) {
      return "";
    }
    return parsed.toString().replace(/\/+$/, "");
  } catch {
    return "";
  }
}
