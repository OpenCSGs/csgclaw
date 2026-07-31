import { createRef } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConversationComposer } from "./ConversationComposer";
import { ConversationWorkingActions, type ConversationWorkingParticipant } from "./types";

describe("ConversationComposer working activity", () => {
  it("shows the latest activity instead of a generic working label and opens that activity", async () => {
    const user = userEvent.setup();
    const participant: ConversationWorkingParticipant = {
      activity: {
        action: ConversationWorkingActions.replying,
        entryID: "manager:message-7",
        summary: "正在检查可用的 agent",
      },
      id: "u-manager",
      name: "manager",
    };
    const toolParticipant: ConversationWorkingParticipant = {
      activity: {
        action: ConversationWorkingActions.running,
        entryID: "dev:tool-8",
        summary: "csgclaw-cli participant list --channel csgclaw",
        toolName: "exec_command",
      },
      id: "u-dev",
      name: "dev",
    };
    const onWorkingAction = vi.fn();

    const { container } = render(
      <ConversationComposer
        authBusyProvider=""
        authStatuses={{}}
        composerDisabled={false}
        composerError=""
        draftSegments={[]}
        draftText=""
        editorRef={createRef<HTMLDivElement>()}
        managerProvider=""
        mentionCandidates={[]}
        mentionIndex={0}
        mentionableUsersByName={new Map()}
        slashCandidates={[]}
        slashIndex={0}
        slashPickerLoading={false}
        slashPickerOpen={false}
        t={(key, params) => {
          if (key === "conversationWorkingReplying") return "正在回复";
          if (key === "conversationWorkingRunning") return "正在运行";
          if (key === "conversationWorkingOpenActivity") return `查看 ${params?.name} 的活动记录`;
          return key;
        }}
        workingParticipants={[participant, toolParticipant]}
        onApplyMention={vi.fn()}
        onApplySlashCandidate={vi.fn()}
        onComposerCompositionEnd={vi.fn()}
        onComposerCompositionStart={vi.fn()}
        onComposerKeyDown={vi.fn()}
        onProviderLogin={vi.fn()}
        onSendMessage={vi.fn()}
        onSyncComposer={vi.fn()}
        onWorkingAction={onWorkingAction}
      />,
    );

    const activity = screen.getByRole("button", { name: "查看 manager 的活动记录" });
    expect(activity).toHaveTextContent("manager");
    expect(activity).toHaveTextContent("正在回复");
    expect(activity).toHaveTextContent("正在检查可用的 agent");
    expect(screen.getByText("exec_command")).toBeInTheDocument();
    expect(screen.getByText("csgclaw-cli participant list --channel csgclaw")).toBeInTheDocument();
    expect(screen.queryByText("正在运行")).not.toBeInTheDocument();
    expect(screen.queryByText("manager 正在工作")).not.toBeInTheDocument();
    expect(container.querySelector(".composer > .composer-working")).toBeInTheDocument();
    expect(container.querySelector(".composer-box .composer-working")).not.toBeInTheDocument();

    await user.click(activity);
    expect(onWorkingAction).toHaveBeenCalledWith(participant);
  });

  it("shows only the latest thinking line inline and stops the exact lease", async () => {
    const user = userEvent.setup();
    const participant: ConversationWorkingParticipant = {
      canStop: true,
      id: "user-worker",
      leaseID: "lease-2",
      name: "worker",
      participantID: "pt-worker",
      requestID: "message-2",
      roomID: "room-1",
      thinkingText: "<b>checking</b>\nnext",
      thinkingTruncated: true,
    };
    const emptyReasoning: ConversationWorkingParticipant = {
      id: "user-preparing",
      name: "preparing-worker",
      thinkingText: "",
      workStage: "thinking",
    };
    const onStop = vi.fn();

    const { container } = render(
      <ConversationComposer
        authBusyProvider=""
        authStatuses={{}}
        composerDisabled={false}
        composerError=""
        draftSegments={[]}
        draftText=""
        editorRef={createRef<HTMLDivElement>()}
        managerProvider=""
        mentionCandidates={[]}
        mentionIndex={0}
        mentionableUsersByName={new Map()}
        slashCandidates={[]}
        slashIndex={0}
        slashPickerLoading={false}
        slashPickerOpen={false}
        t={(key, params) => {
          if (key === "conversationWorkingStop") return "停止";
          if (key === "conversationWorkingStopAria") return `停止 ${params?.name} 的当前请求`;
          if (key === "conversationWorkingThinking") return "正在思考";
          if (key === "conversationWorkingPreparingReply") return "正在准备回复";
          return key;
        }}
        workingParticipants={[participant, emptyReasoning]}
        onApplyMention={vi.fn()}
        onApplySlashCandidate={vi.fn()}
        onComposerCompositionEnd={vi.fn()}
        onComposerCompositionStart={vi.fn()}
        onComposerKeyDown={vi.fn()}
        onProviderLogin={vi.fn()}
        onSendMessage={vi.fn()}
        onStopWorkingTurn={onStop}
        onSyncComposer={vi.fn()}
      />,
    );

    const thinkingLatest = screen.getByText("next");
    const stopButton = screen.getByRole("button", { name: "停止 worker 的当前请求" });
    expect(thinkingLatest).toHaveClass("composer-thinking-latest");
    expect(stopButton.nextElementSibling).toBe(thinkingLatest);
    expect(stopButton).not.toHaveTextContent(/\S/);
    expect(stopButton.querySelector(".composer-working-stop-icon")).toBeInTheDocument();
    expect(screen.queryByText(/<b>checking<\/b>/)).not.toBeInTheDocument();
    expect(screen.getByText("正在准备回复")).toBeInTheDocument();
    expect(container.querySelectorAll(".composer-thinking-latest")).toHaveLength(1);
    await user.hover(stopButton);
    expect(await screen.findByRole("tooltip")).toHaveTextContent("停止");
    await user.click(stopButton);
    expect(onStop).toHaveBeenCalledWith(participant);
  });
});
