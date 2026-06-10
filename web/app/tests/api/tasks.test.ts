import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { createTeamRequest } from "@/api/tasks";

function mockFetch(): Mock<typeof fetch> {
  const fetchMock = vi.fn<typeof fetch>(
    async () =>
      new Response(
        JSON.stringify({
          channel: "csgclaw",
          created_at: "2026-06-10T00:00:00Z",
          id: "team-1",
          lead_agent_id: "u-manager",
          room_id: "room-1",
          status: "active",
          title: "release",
          updated_at: "2026-06-10T00:00:00Z",
        }),
        { status: 201 },
      ),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("tasks API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("creates teams with agent id request fields", async () => {
    const fetchMock = mockFetch();

    await createTeamRequest({
      channel: "csgclaw",
      lead_agent_id: "u-manager",
      member_agent_ids: ["u-worker"],
      title: "release",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/teams",
      expect.objectContaining({
        method: "POST",
      }),
    );
    const init = fetchMock.mock.calls[0]?.[1];
    const body = JSON.parse(String(init?.body));
    expect(body).toMatchObject({
      channel: "csgclaw",
      lead_agent_id: "u-manager",
      member_agent_ids: ["u-worker"],
      title: "release",
    });
    expect(body).not.toHaveProperty("lead_participant_id");
    expect(body).not.toHaveProperty("member_participant_ids");
  });
});
