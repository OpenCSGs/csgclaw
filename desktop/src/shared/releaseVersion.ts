const semanticVersionPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const gitDescribePrereleasePattern =
  /^(.*?)(?:-)?(\d+)-g([0-9a-f]+)(-dirty)?$/i;

export function normalizeDesktopReleaseVersion(
  rawVersion: string | undefined,
): string {
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

export function compareDesktopReleaseVersions(
  leftVersion: string,
  rightVersion: string,
): number {
  const left = parseDesktopReleaseVersion(leftVersion);
  const right = parseDesktopReleaseVersion(rightVersion);
  if (!left || !right) {
    throw new Error(
      `Cannot compare invalid desktop versions: ${leftVersion} and ${rightVersion}`,
    );
  }

  for (const key of ["major", "minor", "patch"] as const) {
    if (left[key] !== right[key]) {
      return left[key] < right[key] ? -1 : 1;
    }
  }
  return comparePrerelease(left.prerelease, right.prerelease);
}

function parseDesktopReleaseVersion(version: string): {
  major: number;
  minor: number;
  patch: number;
  prerelease: string[];
} | null {
  const match = semanticVersionPattern.exec(version.trim().replace(/^v/, ""));
  if (!match) {
    return null;
  }
  return {
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
    prerelease: match[4]?.split(".") ?? [],
  };
}

function comparePrerelease(left: string[], right: string[]): number {
  if (left.length === 0 || right.length === 0) {
    if (left.length === right.length) {
      return 0;
    }
    return left.length === 0 ? 1 : -1;
  }

  for (let index = 0; index < Math.min(left.length, right.length); index += 1) {
    const comparison = comparePrereleaseIdentifier(left[index]!, right[index]!);
    if (comparison !== 0) {
      return comparison;
    }
  }
  return left.length === right.length ? 0 : left.length < right.length ? -1 : 1;
}

function comparePrereleaseIdentifier(left: string, right: string): number {
  if (left === right) {
    return 0;
  }
  const leftNumeric = /^\d+$/.test(left);
  const rightNumeric = /^\d+$/.test(right);
  if (leftNumeric && rightNumeric) {
    return Number(left) < Number(right) ? -1 : 1;
  }
  if (leftNumeric !== rightNumeric) {
    return leftNumeric ? -1 : 1;
  }
  return left < right ? -1 : 1;
}
