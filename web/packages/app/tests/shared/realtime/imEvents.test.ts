import { afterEach, describe, expect, it, vi } from "vitest";

import { resolveIMEventsEndpoint } from "@/shared/realtime/imEvents";

type SharedWorkerTestGlobal = typeof self & {
  onconnect?: (event: MessageEvent) => void;
};

const workerGlobal = self as SharedWorkerTestGlobal;

describe("resolveIMEventsEndpoint", () => {
  it("resolves the SSE endpoint from the application root", () => {
    expect(resolveIMEventsEndpoint("http://127.0.0.1:5173/")).toBe("http://127.0.0.1:5173/api/v1/events");
  });

  it("preserves a production application subpath", () => {
    expect(resolveIMEventsEndpoint("https://example.com/v1/sandboxes/demo/")).toBe(
      "https://example.com/v1/sandboxes/demo/api/v1/events",
    );
  });
});

describe("SSE SharedWorker", () => {
  afterEach(() => {
    delete workerGlobal.onconnect;
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("waits for an absolute endpoint and reconnects after the last port closes", async () => {
    const sources: EventSourceMock[] = [];

    class EventSourceMock {
      static readonly CLOSED = 2;
      static readonly CONNECTING = 0;

      readonly close = vi.fn();
      readonly url: string;
      onerror: (() => void) | null = null;
      onmessage: ((event: MessageEvent<string>) => void) | null = null;
      onopen: (() => void) | null = null;
      readyState = 1;

      constructor(url: string | URL) {
        this.url = String(url);
        sources.push(this);
      }
    }

    vi.stubGlobal("EventSource", EventSourceMock);
    await import("@/shared/realtime/sseSharedWorker");

    const connect = workerGlobal.onconnect;
    expect(connect).toBeTypeOf("function");

    const firstPort = createPortMock();
    connect?.({ ports: [firstPort] } as unknown as MessageEvent);
    expect(sources).toHaveLength(0);

    firstPort.onmessage?.(controlMessage({ type: "subscribe", endpoint: "http://127.0.0.1:5173/api/v1/events" }));
    expect(sources.map((source) => source.url)).toEqual(["http://127.0.0.1:5173/api/v1/events"]);

    firstPort.onmessage?.(controlMessage({ type: "close" }));
    expect(sources[0]?.close).toHaveBeenCalledOnce();

    const secondPort = createPortMock();
    connect?.({ ports: [secondPort] } as unknown as MessageEvent);
    expect(sources).toHaveLength(1);

    secondPort.onmessage?.(controlMessage({ type: "subscribe", endpoint: "http://127.0.0.1:5173/api/v1/events" }));
    expect(sources.map((source) => source.url)).toEqual([
      "http://127.0.0.1:5173/api/v1/events",
      "http://127.0.0.1:5173/api/v1/events",
    ]);

    secondPort.onmessage?.(controlMessage({ type: "close" }));
  });
});

function createPortMock(): MessagePort {
  return {
    close: vi.fn(),
    onmessage: null,
    postMessage: vi.fn(),
    start: vi.fn(),
  } as unknown as MessagePort;
}

function controlMessage(data: { endpoint?: string; type: string }): MessageEvent {
  return { data } as MessageEvent;
}
