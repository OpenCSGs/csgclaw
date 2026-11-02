import { useEffect, useState } from "react";
import { fetchAgentArchiveImpact, type AgentArchiveImpact } from "@/api/agents";

export function useAgentArchiveImpact(id: string | null) {
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<{ id: string; data?: AgentArchiveImpact; failed?: boolean } | null>(null);
  useEffect(() => {
    setResult(null);
    if (!id) return;
    let cancelled = false;
    void fetchAgentArchiveImpact(id)
      .then((data) => {
        if (!cancelled) setResult({ id, data });
      })
      .catch(() => {
        if (!cancelled) setResult({ id, failed: true });
      });
    return () => {
      cancelled = true;
    };
  }, [id, attempt]);
  return {
    data: result?.id === id ? result.data : undefined,
    loading: Boolean(id && result?.id !== id),
    failed: Boolean(id && result?.id === id && result.failed),
    retry: () => setAttempt((value) => value + 1),
  };
}
