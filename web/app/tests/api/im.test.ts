import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { clearRoomMessagesRequest, removeRoomUserRequest, sendMessageRequest } from "@/api/im";

function mockFetch(): Mock<typeof fetch> {
  const fetchMock = vi.fn<typeof fetch>(
    async (_input, _init) =>
      new Response(`{"id":"room-1","title":"Ops","members":["u-admin"],"messages":[]}`, { status: 200 }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("IM API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses the IM-native clearMessages custom method", async () => {
    const fetchMock = mockFetch();

    await clearRoomMessagesRequest("room-1");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/rooms/room-1:clearMessages",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("identifies the removed room member in the DELETE resource path", async () => {
    const fetchMock = mockFetch();

    await removeRoomUserRequest({
      room_id: "room/1",
      member_id: "user/2",
      inviter_id: "u-admin",
      locale: "en",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/rooms/room%2F1/members/user%2F2",
      expect.objectContaining({
        method: "DELETE",
        body: JSON.stringify({ inviter_id: "u-admin", locale: "en" }),
      }),
    );
  });

  it("sends multipart payloads when attachments are present", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => {
      return new Response(JSON.stringify({ id: "msg-1", content: "", attachments: [] }), {
        headers: { "Content-Type": "application/json" },
        status: 201,
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    await sendMessageRequest({
      room_id: "room-1",
      sender_id: "user-admin",
      content: "",
      attachments: [new File(["hello"], "note.txt", { type: "text/plain" })],
    });

    const [, init] = fetchMock.mock.calls[0];
    expect(init?.body).toBeInstanceOf(FormData);
    const form = init?.body as FormData;
    expect(form.get("payload")).toBe(JSON.stringify({ room_id: "room-1", sender_id: "user-admin", content: "" }));
    expect(form.getAll("files")).toHaveLength(1);
    expect(new Headers(init?.headers).has("Content-Type")).toBe(false);
  });

  it("reports multipart upload progress through XMLHttpRequest", async () => {
    const progress: number[] = [];
    const FakeXMLHttpRequest = createFakeXMLHttpRequest();
    vi.stubGlobal("XMLHttpRequest", FakeXMLHttpRequest);

    const request = sendMessageRequest(
      {
        room_id: "room-1",
        sender_id: "user-admin",
        content: "",
        attachments: [new File(["hello"], "note.txt", { type: "text/plain" })],
      },
      { onUploadProgress: (value) => progress.push(value) },
    );

    const xhr = FakeXMLHttpRequest.latest;
    xhr.upload.emit("progress", { lengthComputable: true, loaded: 5, total: 10 });
    xhr.status = 201;
    xhr.responseText = JSON.stringify({ id: "msg-1", content: "", attachments: [] });
    xhr.emit("load", {});

    await expect(request).resolves.toMatchObject({ id: "msg-1" });
    expect(progress).toEqual([0, 50]);
    expect(xhr.body).toBeInstanceOf(FormData);
  });
});

function createFakeXMLHttpRequest() {
  class FakeEventTarget {
    private listeners = new Map<string, Array<(event: unknown) => void>>();

    addEventListener(type: string, listener: (event: unknown) => void): void {
      this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
    }

    emit(type: string, event: unknown): void {
      this.listeners.get(type)?.forEach((listener) => listener(event));
    }
  }

  return class FakeXMLHttpRequest extends FakeEventTarget {
    static latest: InstanceType<typeof FakeXMLHttpRequest>;
    body: Document | XMLHttpRequestBodyInit | null = null;
    responseText = "";
    status = 0;
    statusText = "";
    upload = new FakeEventTarget();

    constructor() {
      super();
      FakeXMLHttpRequest.latest = this;
    }

    abort(): void {
      this.emit("abort", {});
    }

    open(): void {}

    send(body: Document | XMLHttpRequestBodyInit | null): void {
      this.body = body;
    }

    setRequestHeader(): void {}
  };
}
