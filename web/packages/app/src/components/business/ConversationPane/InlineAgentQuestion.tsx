import { useMemo, type ReactNode } from "react";
import { parseAgentActivity } from "@/models/agentActivity";
import type { IMMessage, TranslateFn } from "@/models/conversations";
import { AgentQuestionComposer } from "./AgentQuestionComposer";
import { useQuestionAnswerMode } from "./useQuestionAnswerMode";

export function InlineAgentQuestion({
  message,
  t,
  enabled = true,
  onSettled,
  children,
}: {
  message: IMMessage;
  t: TranslateFn;
  enabled?: boolean;
  onSettled: (id: string) => void;
  children: ReactNode;
}) {
  const question = enabled ? parseAgentActivity(message)?.content.question : undefined;
  if (question?.status !== "pending" || !question.id || !question.questions.length) return children;
  return <PendingQuestion key={question.id} message={message} t={t} onSettled={onSettled} />;
}

function PendingQuestion({
  message,
  t,
  onSettled,
}: {
  message: IMMessage;
  t: TranslateFn;
  onSettled: (id: string) => void;
}) {
  const messages = useMemo(() => [message], [message]);
  const mode = useQuestionAnswerMode({ messages, onSettled, t });
  return <AgentQuestionComposer mode={mode} t={t} />;
}
