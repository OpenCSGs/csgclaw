import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setAgentResourceEnabled } from "@/api/agents";
import { useAgentResourceEnablement } from "@/hooks/workspace/useAgentResourceEnablement";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/agents", () => ({ setAgentResourceEnabled: vi.fn() }));

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("资源启用状态", () => {
  beforeEach(() => vi.mocked(setAgentResourceEnabled).mockReset());

  it("切换 Agent 后完成请求，再次打开原 Agent 时结束加载状态", async () => {
    const request = deferred();
    vi.mocked(setAgentResourceEnabled).mockImplementation(async () => {
      await request.promise;
      return {} as Awaited<ReturnType<typeof setAgentResourceEnabled>>;
    });
    const onChanged = vi.fn().mockResolvedValue(undefined);
    const { result, rerender } = renderHook(({ id }) => useAgentResourceEnablement(id, (key) => key, onChanged), {
      initialProps: { id: "agent-a" },
      wrapper: createQueryWrapper().wrapper,
    });
    let operation!: Promise<void>;
    act(() => {
      operation = result.current.setEnabled("skill", "reviewer", false);
    });
    expect(result.current.busy).toBe("skill:reviewer");
    rerender({ id: "agent-b" });
    await act(async () => {
      request.resolve();
      await operation;
    });
    expect(onChanged).toHaveBeenCalledWith("agent-a");
    rerender({ id: "agent-a" });
    expect(result.current.busy).toBe("");
  });

  it("提交成功但刷新失败时提供错误和相同目标的重试", async () => {
    vi.mocked(setAgentResourceEnabled).mockResolvedValue({} as Awaited<ReturnType<typeof setAgentResourceEnabled>>);
    const onChanged = vi.fn().mockRejectedValueOnce(new Error("刷新失败")).mockResolvedValue(undefined);
    const { result } = renderHook(() => useAgentResourceEnablement("agent-a", (key) => key, onChanged), {
      wrapper: createQueryWrapper().wrapper,
    });
    await act(async () => {
      await result.current.setEnabled("mcp", "search", false);
    });
    expect(result.current.error).not.toBe("");
    await act(async () => {
      await result.current.retry();
    });
    expect(setAgentResourceEnabled).toHaveBeenLastCalledWith("agent-a", "mcp", "search", false);
    expect(result.current.error).toBe("");
  });
});
