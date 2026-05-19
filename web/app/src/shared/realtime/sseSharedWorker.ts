type SharedWorkerGlobal = typeof globalThis & {
  onconnect: ((event: MessageEvent) => void) | null;
};

type WorkerControlMessage = {
  endpoint?: string;
  type?: "close" | "subscribe" | string;
};

let es: EventSource | null = null;
let retryTimer: ReturnType<typeof setTimeout> | null = null;
let endpoint = "/api/v1/events";
const ports = new Set<MessagePort>();
let reconnectDelayMs = 3000;

function broadcast(data: Record<string, unknown>) {
  for (const port of ports) {
    port.postMessage(data);
  }
}

function cleanup(): void {
  if (es) {
    es.close();
    es = null;
  }
}

function clearReconnectTimer(): void {
  if (retryTimer) {
    clearTimeout(retryTimer);
    retryTimer = null;
  }
}

function scheduleReconnect(): void {
  if (ports.size === 0 || retryTimer) {
    return;
  }
  retryTimer = setTimeout(() => {
    retryTimer = null;
    connect();
  }, reconnectDelayMs);
  reconnectDelayMs = Math.min(reconnectDelayMs * 2, 30000);
}

function connect(): void {
  if (es || ports.size === 0) {
    return;
  }

  clearReconnectTimer();
  es = new EventSource(endpoint);

  es.onopen = () => {
    reconnectDelayMs = 3000;
    broadcast({ type: "open" });
  };

  es.onmessage = (event) => {
    broadcast({ type: "message", data: event.data });
  };

  es.onerror = () => {
    broadcast({ type: "error", readyState: es ? es.readyState : EventSource.CLOSED });
    if (!es || es.readyState === EventSource.CONNECTING) {
      return;
    }
    cleanup();
    scheduleReconnect();
  };
}

(self as unknown as SharedWorkerGlobal).onconnect = (event: MessageEvent) => {
  const port = event.ports[0];
  ports.add(port);
  port.start();

  port.onmessage = ({ data }: MessageEvent<WorkerControlMessage>) => {
    if (data?.type === "subscribe") {
      if (typeof data.endpoint === "string" && data.endpoint.length > 0) {
        endpoint = data.endpoint;
      }
      connect();
      return;
    }
    if (data?.type === "close") {
      ports.delete(port);
      if (ports.size === 0) {
        clearReconnectTimer();
        cleanup();
      }
    }
  };

  connect();
};
