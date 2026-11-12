import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchAgentSkillBatch } from "@/api/agentResources";
import { useInfiniteAgentSkills } from "@/hooks/workspace/useInfiniteAgentSkills";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/agentResources", () => ({ fetchAgentSkillBatch: vi.fn() }));
it("loads on demand, appends batches, searches the server, and refreshes enabled membership", async () => {
  let disabled = false;
  vi.mocked(fetchAgentSkillBatch).mockImplementation(async (_id, cursor, { search } = {}) => ({
    items: search ? [{ name: "git-review" }] : cursor ? [{ name: "beta" }] : disabled ? [] : [{ name: "alpha" }],
    total: 2,
    page: 1,
    per: 20,
    list_revision: "test",
    next_cursor: search || cursor || disabled ? undefined : "next",
    has_more: !search && !cursor && !disabled,
  }));
  const query = createQueryWrapper();
  const { result, rerender } = renderHook(({ search, enabled }) => useInfiniteAgentSkills("agent", search, enabled), {
    initialProps: { search: "", enabled: false },
    wrapper: query.wrapper,
  });
  expect(fetchAgentSkillBatch).not.toHaveBeenCalled();
  rerender({ search: "", enabled: true });
  await waitFor(() => expect(result.current.items.map((item) => item.name)).toEqual(["alpha"]));
  await act(async () => {
    await result.current.continuation.loadMore();
  });
  await waitFor(() => expect(result.current.items.map((item) => item.name)).toEqual(["alpha", "beta"]));
  expect(result.current.continuation.hasMore).toBe(false);
  disabled = true;
  await act(async () => {
    await query.client.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills("agent") });
  });
  await waitFor(() => expect(result.current.items).toEqual([]));
  rerender({ search: "gtr", enabled: true });
  await waitFor(() => expect(result.current.items.map((item) => item.name)).toEqual(["git-review"]));
  expect(fetchAgentSkillBatch).toHaveBeenLastCalledWith(
    "agent",
    "",
    { search: "gtr", enabled: true, exclude: ["new", "创建智能体", "创建房间"] },
    expect.any(AbortSignal),
  );
});
