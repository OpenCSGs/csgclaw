import type { TranslateFn } from "@/models/conversations";
import type { RenderedCitation } from "./markdown";

export type CitationSelection = {
  activeID: string;
  anchor: HTMLButtonElement;
  cited: RenderedCitation[];
};

export type CitationSelectHandler = (selection: CitationSelection) => void;

export type MessageLike = {
  id?: string;
  [key: string]: unknown;
};

export type MessageAction = {
  confirm?: string;
  id: string;
  label: string;
  style?: "danger" | "default" | string;
};

export type MessageActionFeedback = {
  key?: string;
  message?: string;
  tone?: "error" | "info" | "success";
};

export type StructuredMessagePayload = {
  badge?: string;
  code?: string;
  codeSummary?: string;
  link?: string;
  meta?: Array<{ label: string; value: string }>;
  payload?: string;
  payloadSummary?: string;
  subtitle?: string;
  summary?: string;
  title: string;
};

export type ActionCardPayload = StructuredMessagePayload & {
  actions: MessageAction[];
  fallback?: string;
  kind: "action_card";
};

export type ParsedStructuredMessage = StructuredMessagePayload | ActionCardPayload;

export type MessageContentProps = {
  actionBusy?: string;
  actionFeedback?: MessageActionFeedback | null;
  content?: string | null;
  enableLongMessageCollapse?: boolean;
  longMessageExpanded?: boolean;
  message?: MessageLike | null;
  onLongMessageExpandedChange?: (expanded: boolean) => void;
  onQuestionSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
  onCitationSelect?: CitationSelectHandler;
  onAction?: (action: MessageAction, message?: MessageLike | null) => void;
  t?: TranslateFn;
};
