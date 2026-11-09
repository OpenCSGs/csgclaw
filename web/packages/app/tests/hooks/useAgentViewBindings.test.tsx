import { renderHook, act } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import type { ReactNode } from "react";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { useAgentViewBindings } from "@/hooks/workspace/useAgentViewBindings";

it("opens task preparation with the current agent selected without opening an existing DM", () => {
  const onOpenDM = vi.fn();
  const controller = {
    ready: true,
    agentViewProps: { item: { id: "agent/one", name: "Worker" }, onOpenDM },
  } as unknown as WorkspaceController;
  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={["/agents/agent%2Fone"]}>
      <WorkspaceControllerProvider controller={controller}>{children}</WorkspaceControllerProvider>
    </MemoryRouter>
  );
  const { result } = renderHook(() => ({ bindings: useAgentViewBindings(), location: useLocation() }), { wrapper });
  act(() => result.current.bindings?.onNewTask(result.current.bindings.item));
  expect(result.current.location.pathname).toBe("/new");
  expect(new URLSearchParams(result.current.location.search).get("agent")).toBe("agent/one");
  expect(onOpenDM).not.toHaveBeenCalled();
});
