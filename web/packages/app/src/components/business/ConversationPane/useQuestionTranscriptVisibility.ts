import { useCallback, useState } from "react";
import { isHiddenQuestionTranscript } from "@/models/agentActivity";
import type { IMMessage } from "@/models/conversations";

export function useQuestionTranscriptVisibility() {
  const [settled, setSettled] = useState<ReadonlySet<string>>(() => new Set());
  const hideQuestionTranscript = useCallback(
    (message: IMMessage) => isHiddenQuestionTranscript(message, settled),
    [settled],
  );
  const onQuestionSettled = useCallback((id: string) => {
    setSettled((current) => (current.has(id) ? current : new Set(current).add(id)));
  }, []);
  return { hideQuestionTranscript, onQuestionSettled };
}
