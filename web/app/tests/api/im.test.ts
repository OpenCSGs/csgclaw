import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { clearRoomMessagesRequest, removeRoomUserRequest } from "@/api/im";

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
});
