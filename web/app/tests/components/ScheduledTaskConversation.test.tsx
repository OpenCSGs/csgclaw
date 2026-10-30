import { act, renderHook } from "@testing-library/react";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { sendMessageRequest } from "@/api/im";
import { useScheduledTaskConversation } from "@/pages/TasksPage/useScheduledTaskConversation";
import { createTranslator } from "@/shared/i18n";
import type { ReactNode } from "react";
vi.mock("@/api/im", () => ({ sendMessageRequest: vi.fn() }));
function setup(model = "test-model") {
  const create = vi.fn().mockResolvedValue({ id: "new-manager-task" });
  const open = vi.fn();
  const onMessage = vi.fn();
  const controller = {
    ready: true,
    t: createTranslator("zh"),
    taskEntry: { create, open, onMessage },
    taskViewProps: {
      agents: [
        {
          id: "agent-manager",
          name: "Manager",
          role: "manager",
          user_id: "user-manager",
          model_config: { model_id: model },
        },
      ],
    },
    sidebarProps: { currentUserID: "user-admin", locale: "zh" },
    agentViewProps: { authStatuses: {} },
  } as unknown as WorkspaceController;
  const wrapper = ({ children }: { children: ReactNode }) => (
    <WorkspaceControllerProvider controller={controller}>{children}</WorkspaceControllerProvider>
  );
  return { ...renderHook(() => useScheduledTaskConversation(), { wrapper }), create, open, onMessage };
}
it("opens a new Manager direct task and sends the original request and timezone", async () => {
  vi.mocked(sendMessageRequest).mockResolvedValue({ id: "message-1" } as never);
  const { result, create, open, onMessage } = setup();
  await act(async () => {
    expect(await result.current.send("每周一10点让Manager汇总工作")).toBe(true);
  });
  expect(create).toHaveBeenCalledWith(
    expect.objectContaining({ member_ids: ["manager"], is_direct: true, type: "free" }),
  );
  expect(sendMessageRequest).toHaveBeenCalledWith(
    expect.objectContaining({
      room_id: "new-manager-task",
      content: expect.stringContaining("每周一10点让Manager汇总工作"),
    }),
  );
  expect(vi.mocked(sendMessageRequest).mock.calls.at(-1)?.[0].content).toContain(
    Intl.DateTimeFormat().resolvedOptions().timeZone,
  );
  expect(onMessage).toHaveBeenCalled();
  expect(open).toHaveBeenCalledWith({ id: "new-manager-task" });
});
it("retries a failed message using the same room and idempotency key", async () => {
  vi.mocked(sendMessageRequest)
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce({ id: "message-1" } as never);
  const { result, create, open } = setup();
  await act(async () => {
    expect(await result.current.send("明天上午10点输出周报")).toBe(false);
  });
  expect(result.current.error).toBeTruthy();
  expect(open).not.toHaveBeenCalled();
  const first = vi.mocked(sendMessageRequest).mock.calls.at(-1)?.[0];
  await act(async () => {
    expect(await result.current.send("明天上午10点输出周报")).toBe(true);
  });
  expect(create).toHaveBeenCalledTimes(1);
  expect(vi.mocked(sendMessageRequest).mock.calls.at(-1)?.[0].client_message_id).toBe(first?.client_message_id);
});
it("requires a configured Manager model before creating a room", async () => {
  const { result, create } = setup("");
  await act(async () => {
    expect(await result.current.send("明天汇总工作")).toBe(false);
  });
  expect(create).not.toHaveBeenCalled();
  expect(result.current.error).toBeTruthy();
});
