import type { TranslateFn } from "@/models/conversations";

type PrimarySidebarLabel = {
  badge?: boolean;
  group?: boolean;
  label: string;
};

const FULL_WIDTH_CHAR_PATTERN =
  /[\u1100-\u115f\u2329\u232a\u2e80-\ua4cf\uac00-\ud7a3\uf900-\ufaff\ufe10-\ufe19\ufe30-\ufe6f\uff00-\uff60\uffe0-\uffe6]/;

const SidebarChrome = {
  badge: 24,
  betweenItemsGap: 12,
  breathingRoom: 8,
  collapsedPrimary: 80,
  defaultExpanded: 600,
  groupLabelInlinePadding: 24,
  header: 24 + 106 + 16 + 32 + 20,
  icon: 24,
  maxExpanded: 720,
  minContext: 200,
  navInlinePadding: 32,
  primaryAutoMinimum: 240,
  primaryMax: 300,
  primaryMin: 200,
  rowInlinePadding: 24,
  step: 16,
} as const;

export const SidebarWidth = {
  collapsedPrimary: SidebarChrome.collapsedPrimary,
  default: SidebarChrome.defaultExpanded,
  max: SidebarChrome.maxExpanded,
  min: SidebarChrome.primaryAutoMinimum + SidebarChrome.minContext,
  primaryFallback: SidebarChrome.primaryAutoMinimum,
  primaryMax: SidebarChrome.primaryMax,
  primaryMin: SidebarChrome.primaryMin,
  step: SidebarChrome.step,
} as const;

export function workspacePrimarySidebarLabels(t: TranslateFn): PrimarySidebarLabel[] {
  return [
    { group: true, label: t("messagesTab") },
    { label: t("messagesTab") },
    { group: true, label: t("agentsTab") },
    { label: t("computerAgentsSection") },
    { label: t("humanSection") },
    { label: t("computersSection") },
    { label: t("notificationsSection") },
    { label: t("teamsSection") },
    { group: true, label: t("tasksTab") },
    { label: t("tasksTab") },
    { label: t("scheduledTasksTab") },
    { group: true, label: t("resourcesTab") },
    { label: t("resourcesTemplatesSection") },
    { badge: true, label: t("resourcesSkillsLabel") },
    { label: t("resourcesModelProvidersSection") },
  ];
}

export function workspacePrimarySidebarWidth(labels: readonly PrimarySidebarLabel[]): number {
  const widestItem = labels.reduce((widest, item) => Math.max(widest, primarySidebarItemWidth(item)), 0);
  const width = Math.ceil(Math.max(SidebarChrome.primaryAutoMinimum, SidebarChrome.header, widestItem));
  return clampPrimarySidebarWidth(width);
}

export function workspaceSidebarWidthBounds(primarySidebarWidth: number = SidebarChrome.primaryAutoMinimum) {
  return {
    default: SidebarChrome.defaultExpanded,
    max: SidebarChrome.maxExpanded,
    min: primarySidebarWidth + SidebarChrome.minContext,
  };
}

export function workspacePrimarySidebarWidthBounds() {
  return {
    default: SidebarChrome.primaryAutoMinimum,
    max: SidebarChrome.primaryMax,
    min: SidebarChrome.primaryMin,
  };
}

export function clampPrimarySidebarWidth(value: number): number {
  return Math.min(SidebarChrome.primaryMax, Math.max(SidebarChrome.primaryMin, Math.round(value)));
}

export function normalizeStoredPrimarySidebarWidth(value: string | null): number | null {
  if (value === null) {
    return null;
  }
  const width = Number(value);
  return Number.isFinite(width) ? clampPrimarySidebarWidth(width) : null;
}

function primarySidebarItemWidth(item: PrimarySidebarLabel): number {
  const textWidth = approximateLabelWidth(item.label);
  if (item.group) {
    return SidebarChrome.navInlinePadding + SidebarChrome.groupLabelInlinePadding + textWidth;
  }
  return (
    SidebarChrome.navInlinePadding +
    SidebarChrome.rowInlinePadding +
    SidebarChrome.icon +
    SidebarChrome.betweenItemsGap +
    textWidth +
    (item.badge ? SidebarChrome.betweenItemsGap + SidebarChrome.badge : 0) +
    SidebarChrome.breathingRoom
  );
}

function approximateLabelWidth(label: string): number {
  return Array.from(label).reduce((total, char) => {
    if (FULL_WIDTH_CHAR_PATTERN.test(char)) {
      return total + 16;
    }
    if (char === " ") {
      return total + 4.5;
    }
    if (/[A-Z]/.test(char)) {
      return total + 9.5;
    }
    if (/[a-z0-9]/.test(char)) {
      return total + 8;
    }
    return total + 7;
  }, 0);
}
