// @ts-nocheck
export function hubWorkspaceAncestorDirs(path) {
  const normalized = typeof path === "string" ? path.trim() : "";
  if (!normalized) {
    return [];
  }
  const segments = normalized.split("/").filter(Boolean);
  if (segments.length <= 1) {
    return [];
  }
  const ancestors = [];
  for (let index = 1; index < segments.length; index += 1) {
    ancestors.push(segments.slice(0, index).join("/"));
  }
  return ancestors;
}

export function buildVisibleHubWorkspaceEntries(entries, collapsedDirs) {
  const hiddenParents = [];
  return entries.filter((entry) => {
    while (hiddenParents.length && !entry.path.startsWith(`${hiddenParents[hiddenParents.length - 1]}/`)) {
      hiddenParents.pop();
    }
    const visible = hiddenParents.length === 0;
    if (entry.type === "dir" && collapsedDirs[entry.path]) {
      hiddenParents.push(entry.path);
    }
    return visible;
  });
}

export function buildInitialCollapsedHubWorkspaceDirs(entries) {
  return (entries || []).reduce((acc, entry) => {
    if (entry?.type === "dir" && entry.path) {
      acc[entry.path] = true;
    }
    return acc;
  }, {});
}

export function formatHubDate(value, locale) {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    timeZone: "UTC",
  }).format(new Date(value));
}

export function formatHubDateTime(value, locale) {
  if (!value) {
    return "-";
  }
  return `${new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    timeZone: "UTC",
  }).format(new Date(value))} (UTC)`;
}

export function formatHubTemplateCount(count, locale, t) {
  if (locale === "zh") {
    return `共 ${count} ${t("hubTemplateCountSuffix")}`;
  }
  return `${count} ${t("hubTemplateCountSuffix")}`;
}
