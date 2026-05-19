// @ts-nocheck
let es = null;
let retryTimer = null;
let endpoint = "/api/v1/events";
const ports = new Set();
let reconnectDelayMs = 3000;

function broadcast(data) {
  for (const port of ports) {
    port.postMessage(data);
  }
}

function cleanup() {
  if (es) {
    es.close();
    es = null;
  }
}

function clearReconnectTimer() {
  if (retryTimer) {
    clearTimeout(retryTimer);
    retryTimer = null;
  }
}

function scheduleReconnect() {
  if (ports.size === 0 || retryTimer) {
    return;
  }
  retryTimer = setTimeout(() => {
    retryTimer = null;
    connect();
  }, reconnectDelayMs);
  reconnectDelayMs = Math.min(reconnectDelayMs * 2, 30000);
}

function connect() {
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

self.onconnect = (event) => {
  const port = event.ports[0];
  ports.add(port);
  port.start();

  port.onmessage = ({ data }) => {
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
