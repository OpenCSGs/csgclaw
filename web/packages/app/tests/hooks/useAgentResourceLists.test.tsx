import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchAgentSkillBatch, fetchAgentMCPBatch } from "@/api/agentResources";
import { useAgentResourceLists } from "@/hooks/workspace/useAgentResourceLists";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/agentResources", () => ({ fetchAgentSkillBatch: vi.fn(), fetchAgentMCPBatch: vi.fn() }));
it("appends resources, refreshes mutations, and isolates agents", async () => {
  let total = 25;
  let enabled = true;
  vi.mocked(fetchAgentSkillBatch).mockImplementation(async (id, cursor) => ({
    items: Array.from({ length: total }, (_, index) => ({ name: `${id}-${index}`, enabled })).slice(
      Number(cursor || 0),
      Number(cursor || 0) + 20,
    ),
    total,
    list_revision: "test",
    has_more: !cursor && total > 20,
    next_cursor: !cursor && total > 20 ? "20" : undefined,
  }));
  vi.mocked(fetchAgentMCPBatch).mockResolvedValue({ items: [], total: 0, list_revision: "mcp", has_more: false });
  const query = createQueryWrapper();
  const { result, rerender } = renderHook(({ id }) => useAgentResourceLists(id), {
    initialProps: { id: "alpha" },
    wrapper: query.wrapper,
  });
  await waitFor(() => expect(result.current.skills.items).toHaveLength(20));
  await act(async () => {
    await result.current.skills.continuation.loadMore();
  });
  await waitFor(() => expect(result.current.skills.items).toHaveLength(25));
  expect(new Set(result.current.skills.items.map((item) => item.name)).size).toBe(25);
  expect(result.current.skills.continuation.hasMore).toBe(false);
  expect(fetchAgentSkillBatch).toHaveBeenLastCalledWith("alpha", "20", {}, expect.any(AbortSignal));
  total = 19;
  enabled = false;
  await act(async () => {
    await query.client.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills("alpha") });
  });
  await waitFor(() => expect(result.current.skills.items).toHaveLength(19));
  expect(result.current.skills.items.every((item) => item.enabled === false)).toBe(true);
  rerender({ id: "beta" });
  expect(result.current.skills.items).toEqual([]);
  await waitFor(() => expect(result.current.skills.items[0].name).toBe("beta-0"));
});

it("restarts a changed cursor sequence without mixing old and new membership", async () => {
  let changed = false;
  vi.mocked(fetchAgentSkillBatch).mockImplementation(async (_id, cursor) => {
    if (cursor) {
      changed = true;
      throw { status: 409, code: "resource_list_changed", message: "Changed" };
    }
    return {
      items: [{ name: changed ? "new" : "old" }],
      total: changed ? 1 : 2,
      list_revision: changed ? "new" : "old",
      has_more: !changed,
      next_cursor: changed ? undefined : "next",
    };
  });
  const { result } = renderHook(() => useAgentResourceLists("agent"), { wrapper: createQueryWrapper().wrapper });
  await waitFor(() => expect(result.current.skills.items[0]?.name).toBe("old"));
  await act(async () => {
    await result.current.skills.continuation.loadMore();
  });
  await waitFor(() => expect(result.current.skills.items.map((item) => item.name)).toEqual(["new"]));
  expect(result.current.skills.continuation.hasMore).toBe(false);
});

it("keeps loaded entries on failure and retries the same cursor", async () => {
  let fail = true;
  vi.mocked(fetchAgentSkillBatch).mockImplementation(async (_id, cursor) => {
    if (cursor && fail) throw new Error("Temporary failure");
    return {
      items: [{ name: cursor ? "second" : "first" }],
      total: 2,
      list_revision: "test",
      has_more: !cursor,
      next_cursor: cursor ? undefined : "next",
    };
  });
  const { result } = renderHook(() => useAgentResourceLists("agent"), { wrapper: createQueryWrapper().wrapper });
  await waitFor(() => expect(result.current.skills.items).toHaveLength(1));
  await act(async () => {
    await result.current.skills.continuation.loadMore();
  });
  expect(result.current.skills.items[0].name).toBe("first");
  await waitFor(() => expect(result.current.skills.continuation.failed).toBe(true));
  fail = false;
  act(() => result.current.skills.continuation.retry());
  await waitFor(() => expect(result.current.skills.items.map((item) => item.name)).toEqual(["first", "second"]));
});
