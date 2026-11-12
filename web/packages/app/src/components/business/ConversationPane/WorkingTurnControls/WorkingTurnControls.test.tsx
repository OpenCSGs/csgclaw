import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WorkingTurnControls } from "./WorkingTurnControls";
import { createTranslator } from "@/shared/i18n";
import type { ConversationWorkingParticipant } from "../types";
const t = createTranslator("zh");
const participant: ConversationWorkingParticipant = {
  id: "user-manager",
  name: "manager",
  requestID: "request",
  leaseID: "lease",
  roomID: "room",
  participantID: "pt-manager",
  canStop: true,
  showContextUsage: true,
};
describe("message turn controls", () => {
  it("uses the existing stop callback and disables repeated requests", async () => {
    const onStop = vi.fn();
    const user = userEvent.setup();
    const { rerender } = render(
      <WorkingTurnControls participant={participant} t={t} onStop={onStop} placement="message" />,
    );
    expect(screen.queryByRole("button", { name: "尚未获得上下文用量" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "停止 manager 的当前请求" }));
    expect(onStop).toHaveBeenCalledWith(participant);
    rerender(
      <WorkingTurnControls
        participant={{ ...participant, stopping: true }}
        t={t}
        onStop={onStop}
        placement="message"
      />,
    );
    expect(screen.getByRole("button", { name: "停止 manager 的当前请求" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent(t("conversationWorkingStopping"));
  });
  it("keeps stop errors visible and permits retry", () => {
    render(
      <WorkingTurnControls
        participant={{ ...participant, stopError: "Please retry" }}
        t={t}
        onStop={vi.fn()}
        placement="message"
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Please retry");
    expect(screen.getByRole("button", { name: "停止 manager 的当前请求" })).toBeEnabled();
  });
});
