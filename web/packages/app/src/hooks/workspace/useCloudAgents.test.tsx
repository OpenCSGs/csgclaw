import { emptyAuthStatus } from "@/models/auth";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { fetchRelayCatalog } from "@/api/relay";
import { CLOUD_AGENTS_STORAGE_KEY } from "@/shared/storage/keys";
import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";
import type { CloudAgent } from "@/models/cloudAgents";
import { useCloudAgents } from "./useCloudAgents";
import { createQueryWrapper } from "../../../tests/helpers/queryClient";

vi.mock("@/api/relay", () => ({ fetchRelayCatalog: vi.fn() }));
const agent: CloudAgent = {
  id: "agt_0123456789abcdef01234567",
  relayURL: "https://community.test/api/v1/relay",
  connectionID: "account-a",
  name: "Worker",
  description: "",
  owner: "",
  publisher: "",
  sourceAgentID: "local",
  visibility: "public",
  status: "online",
  lastSeenAt: null,
  avatar: "",
};
let queries: ReturnType<typeof createQueryWrapper>;
beforeEach(() => {
  queries = createQueryWrapper();
  localStorage.clear();
  vi.resetAllMocks();
  vi.mocked(fetchRelayCatalog).mockResolvedValue([agent]);
});
it("does not request a catalog until a consumer needs cloud agents", async () => {
  const { result, rerender } = renderHook(({ enabled }) => useCloudAgents(undefined, enabled), {
    initialProps: { enabled: false },
    wrapper: queries.wrapper,
  });
  expect(fetchRelayCatalog).not.toHaveBeenCalled();
  rerender({ enabled: true });
  await waitFor(() => expect(result.current.catalog).toEqual([agent]));
});
it("discards the prototype mock cache instead of migrating it", async () => {
  localStorage.setItem("csgclaw.agents.cloud.v2", JSON.stringify([{ ...agent, name: "resume-scorer" }]));
  const { result } = renderHook(() => useCloudAgents(), { wrapper: queries.wrapper });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.agents).toEqual([]);
  expect(localStorage.getItem("csgclaw.agents.cloud.v2")).toBeNull();
});

it("polls once for active consumers and stops when all consumers are disabled", async () => {
  vi.useFakeTimers();
  try {
    const { wrapper } = createQueryWrapper();
    const { rerender } = renderHook(
      ({ enabled }) => {
        useCloudAgents(undefined, enabled);
        useCloudAgents(undefined, enabled);
      },
      { wrapper, initialProps: { enabled: false } },
    );
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(fetchRelayCatalog).not.toHaveBeenCalled();

    rerender({ enabled: true });
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(fetchRelayCatalog).toHaveBeenCalledTimes(1);
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(fetchRelayCatalog).toHaveBeenCalledTimes(2);

    rerender({ enabled: false });
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(fetchRelayCatalog).toHaveBeenCalledTimes(2);
  } finally {
    vi.useRealTimers();
  }
});

it("loads the conversation agent independently of a failed full catalog", async () => {
  const target = { agent_id: agent.id, connection_id: agent.connectionID };
  vi.mocked(fetchRelayCatalog).mockImplementation(async (_signal, requestedTarget) => {
    if (!requestedTarget) throw new Error("Another agent probe failed");
    return [agent];
  });
  localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify([agent]));
  const { wrapper } = createQueryWrapper();
  const directory = renderHook(() => useCloudAgents(), { wrapper });
  const conversation = renderHook(() => useCloudAgents(undefined, true, target), { wrapper });

  await waitFor(() => expect(directory.result.current.error).toBeTruthy());
  await waitFor(() => expect(conversation.result.current.catalog).toEqual([agent]));
  expect(conversation.result.current.addedIDs).toEqual([agent.id]);
  expect(fetchRelayCatalog).toHaveBeenCalledWith(expect.any(AbortSignal), target);
});

it("reloads the catalog after authorization", async () => {
  const loginRequired = { status: 401, code: "relay_login_required" };
  vi.mocked(fetchRelayCatalog).mockRejectedValueOnce(loginRequired);
  const { result, rerender } = renderHook(
    ({ authenticated }) => useCloudAgents({ ...emptyAuthStatus(), authenticated }),
    {
      initialProps: { authenticated: false },
      wrapper: queries.wrapper,
    },
  );
  await waitFor(() => expect(result.current.error).toEqual(loginRequired));
  rerender({ authenticated: true });
  await waitFor(() => expect(result.current.catalog).toEqual([agent]));
  expect(result.current.error).toBeNull();
});
it("synchronizes membership across mounted consumers without losing other additions", async () => {
  const other = { ...agent, id: "agt_abcdef0123456789abcdef01" };
  vi.mocked(fetchRelayCatalog).mockResolvedValue([agent, other]);
  const first = renderHook(() => useCloudAgents(), { wrapper: queries.wrapper });
  const second = renderHook(() => useCloudAgents(), { wrapper: queries.wrapper });
  await waitFor(() => expect(second.result.current.loading).toBe(false));
  await waitFor(() => expect(first.result.current.loading).toBe(false));
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(1);
  act(() => window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT)));
  await waitFor(() => expect(fetchRelayCatalog).toHaveBeenCalledTimes(2));
  act(() => {
    first.result.current.add(agent.id);
    second.result.current.add(other.id);
  });
  expect(first.result.current.agents).toHaveLength(2);
  expect(second.result.current.agents).toHaveLength(2);
  act(() => first.result.current.remove(agent.id));
  expect(second.result.current.agents).toEqual([other]);
  expect(second.result.current.catalog).toHaveLength(2);
});
it("uses current community membership and identity, preserving registered offline agents", async () => {
  localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify([agent]));
  const { result } = renderHook(() => useCloudAgents(), { wrapper: queries.wrapper });
  await waitFor(() => expect(result.current.agents).toEqual([agent]));
  const refresh = async (catalog: CloudAgent[]) => {
    vi.mocked(fetchRelayCatalog).mockResolvedValue(catalog);
    act(() => window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT)));
    await waitFor(() => expect(result.current.catalog).toEqual(catalog));
  };
  await refresh([{ ...agent, status: "offline" }]);
  expect(result.current.agents[0].status).toBe("offline");
  await refresh([]);
  expect(result.current.agents).toEqual([]);
  await refresh([{ ...agent, connectionID: "account-b" }]);
  expect(result.current.agents).toEqual([]);
  await refresh([agent]);
  expect(result.current.agents).toEqual([agent]);
  vi.mocked(fetchRelayCatalog).mockRejectedValue(new Error("Directory unavailable"));
  act(() => window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT)));
  await waitFor(() => expect(result.current.error).toBeTruthy());
  expect(result.current.agents).toEqual([]);
  await refresh([agent]);
  expect(result.current.agents).toEqual([agent]);
});

it("clears the previous account's private catalog while loading the next account", async () => {
  vi.mocked(fetchRelayCatalog).mockResolvedValueOnce([{ ...agent, visibility: "private" }]);
  const { result, rerender } = renderHook(
    ({ user }) => useCloudAgents({ ...emptyAuthStatus(), authenticated: true, user_id: user }),
    { initialProps: { user: "owner" }, wrapper: queries.wrapper },
  );
  await waitFor(() => expect(result.current.catalog[0]?.visibility).toBe("private"));
  rerender({ user: "other" });
  expect(result.current.catalog).toEqual([]);
  await waitFor(() => expect(result.current.catalog).toEqual([agent]));
});
