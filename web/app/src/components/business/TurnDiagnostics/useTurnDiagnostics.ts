import { useEffect, useRef, useState } from "react";
import { fetchDiagnostic, fetchDiagnostics, type DiagnosticList, type TurnDiagnostic } from "@/api/diagnostics";
import { selectDiagnostic } from "@/models/diagnostics";
import type { TranslateFn } from "@/models/conversations";

type Options = {
  room: string;
  source: string;
  turn?: string;
  initialSource?: string;
  initialTurn?: string;
  agent: string;
  status: string;
  cursor: string;
  revision: number;
  enabled: boolean;
  t: TranslateFn;
};

export function useTurnDiagnostics({
  room,
  source,
  turn,
  initialSource,
  initialTurn,
  agent,
  status,
  cursor,
  revision,
  enabled,
  t,
}: Options) {
  const [page, setPage] = useState<DiagnosticList>({ items: [], next_cursor: "", agents: [], total: 0 });
  const [selected, setSelected] = useState("");
  const selection = useRef("");
  const focusedTurn = useRef(initialTurn);
  const [detail, setDetail] = useState<TurnDiagnostic | null>(null);
  const [listError, setListError] = useState("");
  const [detailError, setDetailError] = useState("");
  const loadedDetail = useRef("");
  const [loading, setLoading] = useState(true);
  const summary = JSON.stringify(page.items.find((item) => item.id === selected));

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    setLoading(true);
    const refresh = async () => {
      try {
        const response = await fetchDiagnostics(
          room,
          { source_id: source, turn_id: turn || "", agent_id: agent, status, cursor },
          controller.signal,
        );
        if (controller.signal.aborted) return;
        setPage(response);
        const requestedTurn = source ? turn : focusedTurn.current;
        const requestedSource = source || initialSource;
        const id = selectDiagnostic(
          response.items,
          selection.current,
          requestedTurn,
          response.source?.primary_agent_id,
          requestedSource,
        );
        if (id && !source) focusedTurn.current = undefined;
        selection.current = id;
        setSelected(id);
        setListError("");
      } catch {
        if (!controller.signal.aborted) setListError(t("diagLoadFailed"));
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          timer = setTimeout(refresh, 2000);
        }
      }
    };
    void refresh();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [room, source, turn, initialSource, agent, status, cursor, revision, enabled, t]);

  useEffect(() => {
    if (!enabled) return;
    if (!selected || !summary) {
      setDetailError("");
      return;
    }
    const key = `${room}:${selected}:${revision}:${summary}`;
    if (loadedDetail.current === key) return;
    const controller = new AbortController();
    void fetchDiagnostic(room, selected, controller.signal)
      .then((record) => {
        if (!controller.signal.aborted) {
          setDetail(record);
          loadedDetail.current = key;
          setDetailError("");
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setDetailError(t("diagLoadFailed"));
      });
    return () => controller.abort();
  }, [room, selected, summary, page, revision, enabled, t]);

  const select = (id: string) => {
    focusedTurn.current = undefined;
    selection.current = id;
    setSelected(id);
  };
  return {
    page,
    selected,
    select,
    detail: detail?.id === selected ? detail : null,
    error: listError || detailError,
    loading,
  };
}
