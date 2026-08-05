const semanticVersionPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export function normalizeDesktopReleaseVersion(rawVersion: string | undefined): string {
  const requested = (rawVersion || "").trim().replace(/^v/, "");
  const match = semanticVersionPattern.exec(requested);
  if (!match) {
    return "0.0.0-development";
  }

  // electron-winstaller removes dots from prerelease versions but leaves
  // additional hyphens intact; NuGet rejects versions such as
  // 0.3.15-179-gabcdef. Keep a single prerelease separator instead.
  const prerelease = match[4] ? `-${match[4].replace(/-/g, "")}` : "";
  return `${match[1]}.${match[2]}.${match[3]}${prerelease}`;
}

export function numericDesktopAppVersion(releaseVersion: string): string {
  const match = semanticVersionPattern.exec(releaseVersion);
  if (!match) {
    return "0.0.0";
  }
  return `${match[1]}.${match[2]}.${match[3]}`;
}
