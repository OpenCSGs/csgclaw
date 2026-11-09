import { apiErrorFromText, readResponseText, request, requestText, resolveRequestPath } from "./client";

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

export async function fetchDiagnosticBundle(room: string, id: string, signal?: AbortSignal) {
  const query = new URLSearchParams({ timezone: Intl.DateTimeFormat().resolvedOptions().timeZone });
  const text = await requestText(`${path(room)}/${encodeURIComponent(id)}/bundle?${query}`, {
    signal,
    headers: { Accept: "application/json" },
    cache: "no-store",
  });
  const bundle = JSON.parse(text) as { schema_version: number; turn_id: string; collection: { complete: boolean } };
  return { text, turnId: bundle.turn_id, complete: bundle.collection.complete };
}

function diagnosticFilename(turnId: string) {
  return `csgclaw-turn-${turnId.replace(/^turn-csgclaw-/, "").replace(/[^a-zA-Z0-9._-]/g, "_")}-diagnostics`;
}

export async function fetchDiagnosticArchive(room: string, id: string, signal?: AbortSignal) {
  const query = new URLSearchParams({ timezone: Intl.DateTimeFormat().resolvedOptions().timeZone, download: "1" });
  const response = await fetch(resolveRequestPath(`${path(room)}/${encodeURIComponent(id)}/bundle?${query}`), {
    signal,
    headers: { Accept: "application/zip" },
    cache: "no-store",
  });
  if (!response.ok) {
    throw apiErrorFromText(response.status, (await readResponseText(response)) || response.statusText);
  }
  const blob = await response.blob();
  const { default: JSZip } = await import("jszip");
  // Read only the ZIP directory and small collection-status comment. Do not
  // inflate or JSON.parse the full diagnostic entry just to download it.
  const archive = await JSZip.loadAsync(await blob.arrayBuffer());
  if (!("comment" in archive) || typeof archive.comment !== "string") {
    throw new Error("Diagnostic archive metadata missing");
  }
  const metadata = JSON.parse(archive.comment) as {
    schema_version?: unknown;
    turn_id?: unknown;
    complete?: unknown;
  } | null;
  if (
    metadata?.schema_version !== 1 ||
    typeof metadata.turn_id !== "string" ||
    !metadata.turn_id ||
    typeof metadata.complete !== "boolean"
  ) {
    throw new Error("Invalid diagnostic archive metadata");
  }
  const filename = diagnosticFilename(metadata.turn_id);
  if (Object.keys(archive.files).length !== 1 || !archive.file(`${filename}.json`)) {
    throw new Error("Diagnostic archive entry missing");
  }
  if (signal?.aborted) throw new DOMException("Diagnostic export cancelled", "AbortError");
  return { blob, filename: `${filename}.zip`, turnId: metadata.turn_id, complete: metadata.complete };
}
