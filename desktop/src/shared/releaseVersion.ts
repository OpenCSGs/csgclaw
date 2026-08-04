const semanticVersionPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export function normalizeDesktopReleaseVersion(rawVersion: string | undefined): string {
  const requested = (rawVersion || "").trim().replace(/^v/, "");
  const match = semanticVersionPattern.exec(requested);
  if (!match) {
    return "0.0.0-development";
  }

  const prerelease = match[4] ? `-${match[4]}` : "";
  return `${match[1]}.${match[2]}.${match[3]}${prerelease}`;
}

export function numericDesktopAppVersion(releaseVersion: string): string {
  const match = semanticVersionPattern.exec(releaseVersion);
  if (!match) {
    return "0.0.0";
  }
  return `${match[1]}.${match[2]}.${match[3]}`;
}
