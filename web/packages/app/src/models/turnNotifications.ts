import { Lexer, type Token, type Tokens } from "marked";
import { formatMessagePreviewText, isDirectConversation, normalizeComparable } from "@/models/conversations";
import type { IMMessage, IMServerEvent, TranslateFn } from "@/models/conversations";
import { activityDeliveryKind } from "@/models/agentActivity";
import { parseTurnProgress } from "@/models/turnProgress";

export const TurnNotificationModes = {
  off: "off",
  always: "always",
  whenUnfocused: "when_unfocused",
} as const;

export type TurnNotificationMode = (typeof TurnNotificationModes)[keyof typeof TurnNotificationModes];

export const DEFAULT_TURN_NOTIFICATION_MODE: TurnNotificationMode = TurnNotificationModes.whenUnfocused;

const TURN_NOTIFICATION_TAG_PREFIX = "csgclaw-";
const NOTIFICATION_PREVIEW_MAX_LENGTH = 160;
const NOTIFICATION_PREVIEW_ELLIPSIS = "……";
const FNV1A_64_OFFSET_BASIS = 0xcbf29ce484222325n;
const FNV1A_64_PRIME = 0x100000001b3n;

export function normalizeTurnNotificationMode(value: unknown): TurnNotificationMode {
  return Object.values(TurnNotificationModes).includes(value as TurnNotificationMode)
    ? (value as TurnNotificationMode)
    : DEFAULT_TURN_NOTIFICATION_MODE;
}

export function shouldShowTurnNotification(
  mode: TurnNotificationMode,
  appState: { documentVisible: boolean; windowFocused: boolean },
): boolean {
  if (mode === TurnNotificationModes.off) {
    return false;
  }
  if (mode === TurnNotificationModes.always) {
    return true;
  }
  return !appState.documentVisible || !appState.windowFocused;
}

export function isCompletedAgentTurnEvent(event: IMServerEvent | null | undefined): boolean {
  return Boolean(
    event?.type === "participant.work.updated" &&
    event.work?.kind === "agent_turn" &&
    event.work.state === "idle" &&
    event.work.reason === "released",
  );
}

export function isCompletedAgentTurnMessage(message: IMMessage): boolean {
  const progress = parseTurnProgress(message);
  if (progress) {
    return progress.status === "succeeded";
  }
  const metadata = message.metadata as { csgclaw?: { delivery_kind?: unknown } } | undefined;
  return (metadata?.csgclaw?.delivery_kind ?? activityDeliveryKind(message)) === "final";
}

export function buildTurnNotificationTag(eventKey: string): string {
  // Chromium adds origin data to this value before passing it to the Windows
  // toast API, whose notification tag is limited to 64 characters.
  let hash = FNV1A_64_OFFSET_BASIS;
  for (let index = 0; index < eventKey.length; index += 1) {
    hash ^= BigInt(eventKey.charCodeAt(index));
    hash = BigInt.asUintN(64, hash * FNV1A_64_PRIME);
  }
  return `${TURN_NOTIFICATION_TAG_PREFIX}${hash.toString(16).padStart(16, "0")}`;
}

export function resolveTurnNotificationRoomLabel(
  agentName: string,
  roomTitle: string,
  room?: { is_direct?: boolean | null } | null,
): string {
  const title = roomTitle.trim();
  if (!title || isDirectConversation(room) || normalizeComparable(title) === normalizeComparable(agentName)) {
    return "";
  }
  return title;
}

export function formatTurnNotificationPreview(content: unknown): string {
  const source = formatMessagePreviewText(content, { preserveLineBreaks: true })
    .replace(/[\u200b-\u200d\ufeff]/g, "")
    .trim();
  const lineBreak = source.search(/[\r\n]/);
  const firstLine = lineBreak < 0 ? source : source.slice(0, lineBreak);
  const text = notificationTokensText(Lexer.lex(firstLine, { gfm: true }))
    .replace(/\s+/g, " ")
    .trim();
  if (!text) return "";

  const characters = Array.from(text);
  const hasMore = lineBreak >= 0 || characters.length > NOTIFICATION_PREVIEW_MAX_LENGTH;
  return hasMore
    ? `${characters
        .slice(0, NOTIFICATION_PREVIEW_MAX_LENGTH - NOTIFICATION_PREVIEW_ELLIPSIS.length)
        .join("")
        .trimEnd()}${NOTIFICATION_PREVIEW_ELLIPSIS}`
    : text;
}

function notificationTokensText(tokens: Token[]): string {
  return tokens
    .map((token): string => {
      switch (token.type) {
        case "html":
        case "hr":
        case "def":
        case "checkbox":
          return "";
        case "space":
        case "br":
          return " ";
        case "list":
          return (token as Tokens.List).items.map((item) => notificationTokensText(item.tokens)).join(" ");
        default:
          if ("tokens" in token && token.tokens) return notificationTokensText(token.tokens);
          return "text" in token ? token.text : "";
      }
    })
    .join("");
}

export function formatTurnNotificationBody(
  t: TranslateFn,
  input: {
    agentName: string;
    preview: string;
    room?: { is_direct?: boolean | null } | null;
    roomTitle: string;
  },
): string {
  const preview = input.preview.trim();
  const roomLabel = resolveTurnNotificationRoomLabel(input.agentName, input.roomTitle, input.room);
  if (preview) {
    return roomLabel ? t("turnNotificationBody", { message: preview, room: roomLabel }) : preview;
  }
  return roomLabel ? t("turnNotificationRoomBody", { room: roomLabel }) : t("turnNotificationDefaultBody");
}
