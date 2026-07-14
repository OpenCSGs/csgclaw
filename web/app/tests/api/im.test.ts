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
});
