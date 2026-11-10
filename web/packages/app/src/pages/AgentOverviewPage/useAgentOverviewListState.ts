import { useEffect, useRef, type SetStateAction } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { paneFromLocation } from "@/models/routing";

type ListState = { query: string; filter: string };

export function useAgentOverviewListState() {
  const location = useLocation();
  const navigate = useNavigate();
  const params = new URLSearchParams(location.search);
  const tab = params.get("tab");
  const context =
    tab === "templates" || tab === "market"
      ? `templates:${params.get("template") || ""}`
      : tab === "plaza"
        ? "plaza"
        : "mine";
  const pane = paneFromLocation(location.pathname);
  const selectedFilter = params.get("cloud") ? "cloud" : pane.id ? (pane.type === "team" ? "teams" : "agents") : "";
  const previous = useRef<{ context: string; list: ListState } | null>(null);
  const historyState = location.state && typeof location.state === "object" ? location.state : {};
  const saved: unknown = historyState.agentOverviewList;
  const hasSavedState =
    saved !== null &&
    typeof saved === "object" &&
    "query" in saved &&
    typeof saved.query === "string" &&
    "filter" in saved &&
    typeof saved.filter === "string" &&
    ["all", "agents", "cloud", "teams"].includes(saved.filter);
  let list: ListState;
  if (hasSavedState) {
    list = saved as ListState;
  } else {
    const prior = previous.current;
    list =
      prior?.context === context
        ? prior.list
        : { query: "", filter: selectedFilter && selectedFilter !== "cloud" ? selectedFilter : "all" };
    if (context === "mine" && selectedFilter && list.filter !== "all" && list.filter !== selectedFilter) {
      list = { query: "", filter: selectedFilter };
    }
  }
  const { query, filter } = list;

  useEffect(() => {
    previous.current = { context, list: { query, filter } };
    if (!hasSavedState) {
      // Replace the current entry so Back/Forward can restore it without adding history steps.
      void navigate(location, {
        replace: true,
        state: { ...location.state, agentOverviewList: { query, filter } },
      });
    }
  }, [context, filter, hasSavedState, location, navigate, query]);

  function setListState(update: SetStateAction<ListState>) {
    const next = typeof update === "function" ? update(list) : update;
    void navigate(location, {
      replace: true,
      state: { ...historyState, agentOverviewList: next },
    });
  }

  return { query, filter, setListState };
}
