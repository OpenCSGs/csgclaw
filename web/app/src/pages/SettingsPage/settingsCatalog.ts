import {
  Archive,
  Bell,
  BellRing,
  BookOpen,
  Boxes,
  Cpu,
  Download,
  MessageSquare,
  Palette,
  PlugZap,
  Sparkles,
  UserRound,
  Wrench,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { WorkspaceRouteSegments } from "@/models/routing";

/** Valid section IDs shared by navigation and URL parsing. */
export const SETTINGS_SECTION_IDS = [
  "account",
  "runtime",
  "server",
  "models",
  "connectors",
  "skills",
  "knowledge",
  "external",
  "appearance",
  "notifications",
  "archived",
  "version",
  "feedback",
] as const;

export type SettingsSectionID = (typeof SETTINGS_SECTION_IDS)[number];

export type SettingsCard = {
  id: SettingsSectionID;
  icon: LucideIcon;
  titleKey: string;
  hintKey: string;
};

export type SettingsGroup = {
  id: string;
  labelKey: string;
  cards: SettingsCard[];
};

export const SETTINGS_GROUPS: SettingsGroup[] = [
  {
    id: "account",
    labelKey: "iaSettingsGroupAccount",
    cards: [
      {
        id: "account",
        icon: UserRound,
        titleKey: "iaSettingsCardAccount",
        hintKey: "iaSettingsCardAccountHint",
      },
    ],
  },
  {
    /* Global resources are shared; agent details configures their per-agent bindings. */
    id: "resources",
    labelKey: "iaSettingsGroupResources",
    cards: [
      {
        id: "models",
        icon: Sparkles,
        titleKey: "iaSettingsCardModels",
        hintKey: "iaSettingsCardModelsHint",
      },
      {
        id: "connectors",
        icon: Boxes,
        titleKey: "iaSettingsCardConnectors",
        hintKey: "iaSettingsCardConnectorsHint",
      },
      {
        id: "skills",
        icon: Wrench,
        titleKey: "iaSettingsCardSkills",
        hintKey: "iaSettingsCardSkillsHint",
      },
      {
        id: "knowledge",
        icon: BookOpen,
        titleKey: "iaSettingsCardKnowledge",
        hintKey: "iaSettingsCardKnowledgeHint",
      },
      {
        id: "external",
        icon: BellRing,
        titleKey: "iaSettingsCardExternal",
        hintKey: "iaSettingsCardExternalHint",
      },
    ],
  },
  {
    id: "machine",
    labelKey: "iaSettingsGroupMachine",
    cards: [
      {
        id: "runtime",
        icon: Cpu,
        titleKey: "iaSettingsCardRuntime",
        hintKey: "iaSettingsCardRuntimeHint",
      },
      {
        id: "server",
        icon: PlugZap,
        titleKey: "iaSettingsCardServer",
        hintKey: "iaSettingsCardServerHint",
      },
    ],
  },
  {
    id: "preference",
    labelKey: "iaSettingsGroupPreference",
    cards: [
      {
        id: "appearance",
        icon: Palette,
        titleKey: "iaSettingsCardAppearance",
        hintKey: "iaSettingsCardAppearanceHint",
      },
      {
        id: "notifications",
        icon: Bell,
        titleKey: "iaSettingsCardNotifications",
        hintKey: "iaSettingsCardNotificationsHint",
      },
    ],
  },
  {
    id: "data",
    labelKey: "iaSettingsGroupData",
    cards: [
      {
        id: "archived",
        icon: Archive,
        titleKey: "iaSettingsCardArchived",
        hintKey: "iaSettingsCardArchivedHint",
      },
    ],
  },
  {
    id: "app",
    labelKey: "iaSettingsGroupApp",
    cards: [
      {
        id: "version",
        icon: Download,
        titleKey: "iaSettingsCardVersion",
        hintKey: "iaSettingsCardVersionHint",
      },
      {
        id: "feedback",
        icon: MessageSquare,
        titleKey: "iaSettingsCardFeedback",
        hintKey: "iaSettingsCardFeedbackHint",
      },
    ],
  },
];

export const DEFAULT_SETTINGS_SECTION: SettingsSectionID = "account";

// Preserve deep links from the former MCP section.
const SETTINGS_SECTION_ALIASES: Record<string, SettingsSectionID> = {
  mcp: "connectors",
};

export function settingsSectionFromSearch(search: string): SettingsSectionID {
  const value = new URLSearchParams(search).get("section") || "";
  if (SETTINGS_SECTION_ALIASES[value]) {
    return SETTINGS_SECTION_ALIASES[value];
  }
  return (SETTINGS_SECTION_IDS as readonly string[]).includes(value)
    ? (value as SettingsSectionID)
    : DEFAULT_SETTINGS_SECTION;
}

export function settingsPath(section: SettingsSectionID = DEFAULT_SETTINGS_SECTION): string {
  return `/${WorkspaceRouteSegments.settings}?section=${section}`;
}

export function settingsSectionTitleKey(id: SettingsSectionID): string {
  for (const group of SETTINGS_GROUPS) {
    const card = group.cards.find((item) => item.id === id);
    if (card) {
      return card.titleKey;
    }
  }
  return "settings";
}
