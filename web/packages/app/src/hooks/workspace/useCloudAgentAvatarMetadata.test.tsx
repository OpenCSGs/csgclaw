import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { fetchRelayCatalog } from "@/api/relay";
import { emptyAuthStatus } from "@/models/auth";
import { cloudAgentAvatar, withCloudAgentAvatars } from "@/models/cloudAgents";
import { cloudAgentFixture } from "../../../tests/helpers/cloudAgentFixture";
import { createQueryWrapper } from "../../../tests/helpers/queryClient";
import { cloudAgentCatalogKey, useCloudAgentCatalog } from "./useCloudAgentCatalog";
import { useCloudAgentAvatarMetadata } from "./useCloudAgentAvatarMetadata";

vi.mock("@/api/relay", () => ({ fetchRelayCatalog: vi.fn() }));
const auth = { ...emptyAuthStatus(), authenticated: true, base_url: "https://community.test", user_id: "alice" };
const agent = { ...cloudAgentFixture[0], type: "code" };
const target = { agent_id: agent.id, connection_id: agent.connectionID };

beforeEach(() => vi.mocked(fetchRelayCatalog).mockReset().mockResolvedValue([agent]));

it("shares a cold-start targeted lookup with passive avatar readers without fetching a full catalog", async () => {
  const { wrapper, client } = createQueryWrapper();
  const list = renderHook(() => useCloudAgentAvatarMetadata(auth), { wrapper });
  const conversation = renderHook(
    () => {
      useCloudAgentCatalog(auth, true, target);
      return useCloudAgentAvatarMetadata(auth);
    },
    { wrapper },
  );
  await waitFor(() => expect(list.result.current[0]?.type).toBe("code"));
  expect(conversation.result.current).toEqual(list.result.current);
  const users = withCloudAgentAvatars(
    [{ id: "cloud-user", name: "123456", avatar: "1" }],
    [{ id: "room", members: [], messages: [], cloud_agent: { ...target, user_id: "cloud-user" } }],
    conversation.result.current,
  );
  expect(users[0].avatar).toBe(cloudAgentAvatar(list.result.current[0].type));
  expect(client.getQueryData(cloudAgentCatalogKey(auth))).toBeUndefined();
  expect(fetchRelayCatalog).toHaveBeenCalledExactlyOnceWith(expect.any(AbortSignal), target);
});

it("indexes connections separately and uses the newest metadata from either query scope", async () => {
  const { wrapper, client } = createQueryWrapper();
  client.setQueryData(cloudAgentCatalogKey(auth), [{ ...agent, type: "langflow" }], { updatedAt: 100 });
  client.setQueryData(cloudAgentCatalogKey(auth, target), [agent], { updatedAt: 200 });
  const other = { ...agent, connectionID: "another-connection", type: "openclaw" };
  client.setQueryData(cloudAgentCatalogKey(auth, { ...target, connection_id: other.connectionID }), [other]);
  const { result } = renderHook(() => useCloudAgentAvatarMetadata(auth), { wrapper });
  expect(result.current).toEqual([
    expect.objectContaining({ id: agent.id, connectionID: agent.connectionID, type: "code" }),
    expect.objectContaining({ id: agent.id, connectionID: other.connectionID, type: "openclaw" }),
  ]);
  act(() => {
    client.setQueryData(cloudAgentCatalogKey(auth), [{ ...agent, type: "csgclaw" }], { updatedAt: 300 });
  });
  await waitFor(() => expect(result.current[0].type).toBe("csgclaw"));
  expect(fetchRelayCatalog).not.toHaveBeenCalled();
});

it("prefers targeted metadata on timestamp ties and updates when its cache is removed", async () => {
  const { wrapper, client } = createQueryWrapper();
  client.setQueryData(cloudAgentCatalogKey(auth, target), [agent], { updatedAt: 100 });
  client.setQueryData(cloudAgentCatalogKey(auth), [{ ...agent, type: "langflow" }], { updatedAt: 100 });
  const { result } = renderHook(() => useCloudAgentAvatarMetadata(auth), { wrapper });
  expect(result.current[0].type).toBe("code");
  act(() => client.removeQueries({ queryKey: cloudAgentCatalogKey(auth, target), exact: true }));
  await waitFor(() => expect(result.current[0].type).toBe("langflow"));
  act(() => client.clear());
  await waitFor(() => expect(result.current).toEqual([]));
});

it.each([
  { user_id: "bob" },
  { base_url: "https://other.test" },
  { logged_in_at: "new-session" },
  { authenticated: false },
])("isolates avatar metadata after an identity change: %j", (change) => {
  const { wrapper, client } = createQueryWrapper();
  client.setQueryData(cloudAgentCatalogKey(auth, target), [agent]);
  const { result, rerender } = renderHook(({ identity }) => useCloudAgentAvatarMetadata(identity), {
    wrapper,
    initialProps: { identity: auth },
  });
  expect(result.current[0].type).toBe("code");
  rerender({ identity: { ...auth, ...change } });
  expect(result.current).toEqual([]);
  expect(fetchRelayCatalog).not.toHaveBeenCalled();
});
