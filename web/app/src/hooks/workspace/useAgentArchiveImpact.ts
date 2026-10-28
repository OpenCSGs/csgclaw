import { useEffect, useState } from "react";
import { fetchAgentArchiveImpact, type AgentArchiveImpact } from "@/api/agents";

export function useAgentArchiveImpact(id: string | null) {
  const [result, setResult] = useState<{ id: string; data: AgentArchiveImpact } | null>(null);
  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    void fetchAgentArchiveImpact(id)
      .then((data) => {
        if (!cancelled) setResult({ id, data });
      })
      .catch(() => {
        /* The confirmation still explains every effect when counts are unavailable. */
      });
    return () => {
      cancelled = true;
    };
  }, [id]);
  return { data: result?.id === id ? result?.data : undefined };
}
