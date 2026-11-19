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
            cloud_read_available: source === "platform",
            recommended: true,
            owned_by_current_user: source === "platform" ? true : undefined,
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
    recommended: true,
    ownedByCurrentUser: source === "platform",
    publisher,
    cloudReadAvailable: source === "platform",
    type: "csgclaw",
    avatar: "avatar/cloud-csgclaw.png",
  });
  expect(agent).not.toHaveProperty("token");
});

it("routes cloud resource reads through the local read-only proxy", async () => {
  const { cloudResourceScope } = await import("@/models/cloudResourceScope");
  const { fetchConversationResourceBatch } = await import("@/api/agentResources");
  const fetcher = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ items: [], total: 0, list_revision: "revision", has_more: false }), {
      status: 200,
    }),
  );
  vi.stubGlobal("fetch", fetcher);
  await fetchConversationResourceBatch(
    cloudResourceScope("agt_remote", "account-reference"),
    "knowledge_base",
    "cursor",
    { search: "station" },
  );
  const requested = new URL(fetcher.mock.calls[0][0], "https://local.test/");
  expect(requested.pathname).toBe("/api/v1/relay/agents/agt_remote/resources");
  expect(requested.searchParams.get("connection_id")).toBe("account-reference");
  expect(requested.searchParams.get("type")).toBe("knowledge_base");
  expect(requested.searchParams.get("cursor")).toBe("cursor");
  expect(requested.searchParams.get("search")).toBe("station");
});
