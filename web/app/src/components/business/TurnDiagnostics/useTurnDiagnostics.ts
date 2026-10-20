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
  const focus = useRef({ source: initialSource, turn: initialTurn });
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
        let response = await fetchDiagnostics(
          room,
          { source_id: source, turn_id: turn || "", agent_id: agent, status, cursor },
          controller.signal,
        );
        if (controller.signal.aborted) return;
        const requestedTurn = source ? turn : focus.current.turn;
        const requestedSource = source || focus.current.source;
        let id = selectDiagnostic(
          response.items,
          selection.current,
          requestedTurn,
          response.source?.primary_agent_id,
          requestedSource,
        );
        // Resolve message focus independently of the conversation's current page.
        if (!source && requestedSource && !id) {
          const target = await fetchDiagnostics(
            room,
            { source_id: requestedSource, turn_id: requestedTurn || "", agent_id: agent, status },
            controller.signal,
          );
          if (controller.signal.aborted) return;
          id = selectDiagnostic(target.items, "", requestedTurn, target.source?.primary_agent_id, requestedSource);
          const item = target.items.find((item) => item.id === id);
          response = { ...response, items: item ? [...response.items, item] : response.items, source: target.source };
        }
        setPage(response);
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
    focus.current = { source: undefined, turn: undefined };
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
