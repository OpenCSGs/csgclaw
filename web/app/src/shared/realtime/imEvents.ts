import { ApiEndpoints, IM_EVENTS_SHARED_WORKER_PATH } from "@/shared/constants/api";
import type { IMServerEvent } from "@/models/conversations";

const sharedWorkerURL = import.meta.env.DEV ? "/src/shared/realtime/sseSharedWorker.ts" : IM_EVENTS_SHARED_WORKER_PATH;

function createSharedWorker() {
  if (import.meta.env.DEV) {
    return new SharedWorker(sharedWorkerURL, { type: "module" });
  }
  return new SharedWorker(sharedWorkerURL);
}

type SharedWorkerEnvelope = {
  data?: string;
  type?: string;
};

export function safeParseEventData(raw: string): IMServerEvent | null {
  try {
    return JSON.parse(raw) as IMServerEvent;
  } catch (error) {
    console.warn("Failed to parse IM event payload", error);
    return null;
  }
}

export function subscribeIMEvents(onEvent: (payload: IMServerEvent) => void): () => void {
  if (typeof window.SharedWorker === "function") {
    try {
      const worker = createSharedWorker();
      const port = worker.port;
      const handleMessage = ({ data }: MessageEvent<SharedWorkerEnvelope>) => {
        if (!data || data.type !== "message") {
          return;
        }
        const payload = safeParseEventData(String(data.data ?? ""));
        if (payload) {
          onEvent(payload);
        }
      };

      port.addEventListener("message", handleMessage);
      port.start();
      port.postMessage({ type: "subscribe", endpoint: ApiEndpoints.imEvents });

      return () => {
        port.postMessage({ type: "close" });
        port.removeEventListener("message", handleMessage);
        port.close();
      };
    } catch (error) {
      console.warn("SharedWorker SSE unavailable, falling back to EventSource", error);
    }
  }

  const source = new EventSource(ApiEndpoints.imEvents);
  source.onmessage = (event) => {
    const payload = safeParseEventData(event.data);
    if (payload) {
      onEvent(payload);
    }
  };

  return () => source.close();
}
