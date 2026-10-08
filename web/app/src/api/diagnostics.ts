import { request } from "./client";

export type DiagnosticSpan = {
  id: string;
  name: string;
  owner: string;
  start_ms: number;
  end_ms?: number;
  status: string;
  details?: {
    source?: string;
    category?: string;
    trace_id?: string;
    parent_span_id?: string;
    label?: string;
    command?: string;
    directory?: string;
    arguments?: string;
    event_type?: string;
    http_status?: number;
    reported_duration_ms?: number;
    exit_code?: number;
    first_response_ms?: number;
  };
};
export type TurnDiagnostic = {
  id: string;
  room_id: string;
  source_id: string;
  thread_id?: string;
  agent_id: string;
  agent_name?: string;
  turn_id: string;
  runtime?: string;
  runtime_session_id?: string;
  runtime_turn_id?: string;
  runtime_request_id?: string;
  model?: string;
  started_at: string;
  dispatch_ms?: number;
  status: string;
  total_ms: number;
  runtime_start_ms?: number;
  runtime_end_ms?: number;
  first_output_ms?: number;
  spans?: DiagnosticSpan[];
  error?: { code: string; stage: string; message: string };
  incomplete?: boolean;
  browser?: { first_ms?: number; complete_ms?: number; first_text_ms?: number; first_text_at?: string };
};
export type DiagnosticSource = {
  id: string;
  sender_name: string;
  content: string;
  created_at: string;
  state: "recorded" | "waiting" | "no_execution" | "unavailable";
  primary_agent_id?: string;
};
export type DiagnosticList = {
  items: TurnDiagnostic[];
  next_cursor: string;
  total: number;
  agents: { id: string; name: string }[];
  source?: DiagnosticSource | null;
};
const path = (room: string) => `/api/v1/rooms/${encodeURIComponent(room)}/diagnostics`;
export function fetchDiagnostics(room: string, filters: Record<string, string>, signal?: AbortSignal) {
  return request<DiagnosticList>(`${path(room)}?${new URLSearchParams(filters)}`, { signal });
}
export function fetchDiagnostic(room: string, id: string, signal?: AbortSignal) {
  return request<TurnDiagnostic>(`${path(room)}/${encodeURIComponent(id)}`, { signal });
}
export function reportDiagnosticTiming(
  room: string,
  source: string,
  turn: string,
  first: number,
  complete?: number,
  firstText?: { ms: number; at: string },
) {
  return request<void>(`${path(room)}/timings`, {
    method: "POST",
    json: {
      source_id: source,
      turn_id: turn,
      first_ms: first,
      complete_ms: complete,
      first_text_ms: firstText?.ms,
      first_text_at: firstText?.at,
    },
  });
}
