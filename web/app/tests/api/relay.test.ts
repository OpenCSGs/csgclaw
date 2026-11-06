import { afterEach, expect, it, vi } from "vitest";
import { fetchRelayCatalog } from "@/api/relay";
afterEach(() => vi.unstubAllGlobals());
it.each([
  ["platform", "Agentichub"],
  ["external", "CSGClaw"],
] as const)("maps %s community origins to the publisher label", async (source, publisher) => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        data: [
          {
            source,
            type: "csgclaw",
            relay_url: "http://relay.test",
            connection_id: "saved-connection",
            avatar: "avatar/pic-2.png",
            agent: {
              id: "agt_real",
              name: "Real agent",
              description: "Published description",
              owner_id: "owner",
              publisher_id: "publisher",
              source_agent_id: "local_1",
              visibility: "public",
              status: "unhealthy",
              last_seen_at: null,
            },
          },
        ],
      }),
      { status: 200 },
    ),
  );
  vi.stubGlobal("fetch", fetcher);
  const [agent] = await fetchRelayCatalog();
  expect(fetcher.mock.calls[0][0]).toContain("api/v1/relay/agents");
  expect(agent).toMatchObject({
    id: "agt_real",
    relayURL: "http://relay.test",
    connectionID: "saved-connection",
    status: "offline",
    owner: "owner",
    publisher,
    type: "csgclaw",
    avatar: "avatar/cloud-csgclaw.png",
  });
  expect(agent).not.toHaveProperty("token");
});
