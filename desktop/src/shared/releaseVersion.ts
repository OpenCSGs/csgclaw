const semanticVersionPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const gitDescribePrereleasePattern = /^(.*?)(?:-)?(\d+)-g([0-9a-f]+)(-dirty)?$/i;

export function normalizeDesktopReleaseVersion(rawVersion: string | undefined): string {
  const requested = (rawVersion || "").trim().replace(/^v/, "");
  const match = semanticVersionPattern.exec(requested);
  if (!match) {
    return "0.0.0-development";
  }

  const prerelease = match[4];
  if (prerelease) {
    const gitDescribeMatch = gitDescribePrereleasePattern.exec(prerelease);
    if (gitDescribeMatch) {
      const [, label, distance, commit, dirty] = gitDescribeMatch;
      const normalizedLabel = label?.replace(/[^0-9A-Za-z]/g, "") || "";
      const prereleaseLabel = normalizedLabel ? `${normalizedLabel}dev` : "dev";
      const normalizedPrerelease = `${prereleaseLabel}${distance}g${commit}${dirty ? "dirty" : ""}`;
      return `${match[1]}.${match[2]}.${match[3]}-${normalizedPrerelease}`;
    }
    return `${match[1]}.${match[2]}.${match[3]}-${prerelease}`;
  }
  return `${match[1]}.${match[2]}.${match[3]}`;
}

export function numericDesktopAppVersion(releaseVersion: string): string {
  const match = semanticVersionPattern.exec(releaseVersion);
  if (!match) {
    return "0.0.0";
  }
  return `${match[1]}.${match[2]}.${match[3]}`;
}
