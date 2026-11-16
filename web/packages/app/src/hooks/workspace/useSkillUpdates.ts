import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchSkillUpdates } from "@/api/skills";
import { workspaceQueryKeys } from "./workspaceQueries";
import type { AuthStatus } from "@/models/auth";

export const skillUpdatesQueryKey = ["workspace", "skill-updates"] as const;

export function useSkillUpdatesQuery() {
  const status = useQueryClient().getQueryData<AuthStatus>(workspaceQueryKeys.authStatus());
  return useQuery({
    queryKey: [
      ...skillUpdatesQueryKey,
      status?.opencsg_base_url ?? "",
      status?.user_id ?? "",
      status?.logged_in_at ?? "",
    ],
    queryFn: fetchSkillUpdates,
    staleTime: 5 * 60_000,
    refetchInterval: 5 * 60_000,
    retry: false,
  });
}

export function useSkillUpdatePrompt() {
  const [skillUpdateNames, setNames] = useState<string[]>([]);
  const pending = useRef<((updated: boolean) => void) | null>(null);
  const requestSkillUpdates = useCallback((names: string[]) => {
    if (pending.current || names.length === 0) return Promise.resolve(false);
    return new Promise<boolean>((resolve) => {
      pending.current = resolve;
      setNames([...new Set(names)].sort());
    });
  }, []);
  const finishSkillUpdates = useCallback((updated: boolean) => {
    pending.current?.(updated);
    pending.current = null;
    setNames([]);
  }, []);
  useEffect(
    () => () => {
      pending.current?.(false);
    },
    [],
  );
  return { skillUpdateNames, requestSkillUpdates, finishSkillUpdates };
}
