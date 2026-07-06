import type { TranslateFn } from "@/models/conversations";

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

export type MessageActionError = {
  key?: string;
  message?: string;
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
  actionError?: MessageActionError | null;
  content?: string | null;
  enableLongMessageCollapse?: boolean;
  longMessageExpanded?: boolean;
  message?: MessageLike | null;
  onLongMessageExpandedChange?: (expanded: boolean) => void;
  onAction?: (action: MessageAction, message?: MessageLike | null) => void;
  t?: TranslateFn;
};
