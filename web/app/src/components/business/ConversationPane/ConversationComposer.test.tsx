import { createRef } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConversationComposer } from "./ConversationComposer";
import { ConversationWorkingActions, type ConversationWorkingParticipant } from "./types";

function defaultTranslate(key: string, params?: Record<string, unknown>) {
  if (key === "composerAddContent") return "添加内容";
  if (key === "composerAdd") return "添加";
  if (key === "addAttachment") return "添加附件";
  if (key === "connectorGitHub") return "GitHub";
  if (key === "connectorGitLab") return "GitLab";
  if (key === "connectorNotConnected") return "未连接";
  if (key === "connectorConnect") return "连接";
  if (key === "connectorEdit") return "编辑";
  if (key === "connectorDisconnect") return "断开";
  if (key === "connectorSave") return "保存";
  if (key === "connectorGitLabBaseURL") return "GitLab 地址";
  if (key === "connectorGitLabToken") return "Personal Access Token";
  if (key === "connectorGitLabSaveSuccess") return "GitLab 连接配置已保存。";
  if (key === "connectorGitLabSaveFailed") return "保存 GitLab 连接器失败。请检查地址和 Token。";
  if (key === "conversationWorkingProcessTitle") return "运行过程";
  if (key === "conversationWorkingProcessSubtitle") return `${params?.count} 个智能体正在处理`;
  if (key === "conversationWorkingProcessCount") return `${params?.count} 个运行中`;
  if (key === "conversationWorkingActivityDrawer") return "活动记录";
  if (key === "conversationWorkingExpandProcess") return "展开过程";
  if (key === "conversationWorkingCollapseProcess") return "收起过程";
  if (key === "conversationWorkingResizeProcess") return "调整运行过程高度";
  if (key === "cancel") return "取消";
  if (key === "close") return "关闭";
  if (key === "connectorConnected") return "已连接";
  if (key === "conversationWorkingOpenActivity") return `查看 ${params?.name} 的活动记录`;
  return key;
}

function renderConversationComposer(overrideProps: Partial<React.ComponentProps<typeof ConversationComposer>> = {}) {
  return render(
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
      t={defaultTranslate}
      onAddAttachments={vi.fn()}
      onApplyMention={vi.fn()}
      onApplySlashCandidate={vi.fn()}
      onComposerCompositionEnd={vi.fn()}
      onComposerCompositionStart={vi.fn()}
      onComposerKeyDown={vi.fn()}
      onProviderLogin={vi.fn()}
      onSendMessage={vi.fn()}
      onSyncComposer={vi.fn()}
      {...overrideProps}
    />,
  );
}

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
        detail: "csgclaw-cli participant list --channel csgclaw\n\nFound 3 participants",
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
          if (key === "conversationWorkingProcessTitle") return "运行过程";
          if (key === "conversationWorkingProcessSubtitle") return `${params?.count} 个智能体正在处理`;
          if (key === "conversationWorkingProcessCount") return `${params?.count} 个运行中`;
          if (key === "conversationWorkingActivityDrawer") return "活动记录";
          if (key === "conversationWorkingExpandProcess") return "展开过程";
          if (key === "conversationWorkingCollapseProcess") return "收起过程";
          if (key === "conversationWorkingResizeProcess") return "调整运行过程高度";
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

    const processToggle = screen.getByRole("button", { name: "收起过程" });
    expect(processToggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getAllByText("正在检查可用的 agent").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("manager")).toBeInTheDocument();
    expect(screen.getByText("exec_command")).toBeInTheDocument();
    const drawerButton = screen.getByRole("button", { name: "活动记录" });
    await user.click(drawerButton);
    expect(onWorkingAction).toHaveBeenCalledWith();

    const activity = screen.getByRole("button", { name: "查看 manager 的活动记录" });
    expect(activity).toHaveTextContent("manager");
    expect(activity).toHaveTextContent("正在回复");
    expect(activity).toHaveTextContent("正在检查可用的 agent");
    expect(screen.getByText("exec_command")).toBeInTheDocument();
    expect(screen.getByText("csgclaw-cli participant list --channel csgclaw")).toBeInTheDocument();
    expect(screen.getByText(/Found 3 participants/)).toBeInTheDocument();
    expect(screen.queryByText("正在运行")).not.toBeInTheDocument();
    expect(screen.queryByText("manager 正在工作")).not.toBeInTheDocument();
    expect(container.querySelector(".composer > .composer-working")).toBeInTheDocument();
    expect(container.querySelector(".composer-box .composer-working")).not.toBeInTheDocument();

    await user.click(activity);
    expect(onWorkingAction).toHaveBeenCalledWith(participant);
    await user.click(processToggle);
    expect(screen.queryByText("运行过程")).not.toBeInTheDocument();
    expect(screen.getByText("manager")).toBeInTheDocument();
    expect(screen.queryByText("exec_command")).toBeInTheDocument();
    const compactExpand = screen.getByRole("button", { name: "展开过程" });
    expect(compactExpand).toHaveAttribute("aria-expanded", "false");
  });

  it("shows only the latest thinking line inline and stops the exact lease", async () => {
    const user = userEvent.setup();
    const participant: ConversationWorkingParticipant = {
      canStop: true,
      activity: {
        action: ConversationWorkingActions.thinking,
        detail: "tool detail line",
      },
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
          if (key === "conversationWorkingProcessTitle") return "运行过程";
          if (key === "conversationWorkingProcessSubtitle") return `${params?.count} 个智能体正在处理`;
          if (key === "conversationWorkingProcessCount") return `${params?.count} 个运行中`;
          if (key === "conversationWorkingActivityDrawer") return "活动记录";
          if (key === "conversationWorkingExpandProcess") return "展开过程";
          if (key === "conversationWorkingCollapseProcess") return "收起过程";
          if (key === "conversationWorkingResizeProcess") return "调整运行过程高度";
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

    const processToggle = screen.getByRole("button", { name: "收起过程" });
    expect(processToggle).toHaveAttribute("aria-expanded", "true");
    const thinkingLatest = container.querySelector(".composer-thinking-latest");
    const thinkingTranscript = container.querySelector(".composer-thinking-transcript");
    const stopButton = screen.getByRole("button", { name: "停止 worker 的当前请求" });
    expect(thinkingLatest).toHaveTextContent("next");
    expect(thinkingTranscript).toHaveTextContent("<b>checking</b>");
    expect(thinkingTranscript).toHaveTextContent("next");
    expect(thinkingTranscript).toHaveTextContent("tool detail line");
    expect(stopButton.nextElementSibling).toBe(thinkingLatest);
    expect(stopButton).not.toHaveTextContent(/\S/);
    expect(stopButton.querySelector(".composer-working-stop-icon")).toBeInTheDocument();
    expect(screen.getByText("正在准备回复")).toBeInTheDocument();
    expect(container.querySelectorAll(".composer-thinking-latest")).toHaveLength(1);
    await user.hover(stopButton);
    expect(await screen.findByRole("tooltip")).toHaveTextContent("停止");
    await user.click(stopButton);
    expect(onStop).toHaveBeenCalledWith(participant);
    await user.click(processToggle);
    expect(screen.queryByText("运行过程")).not.toBeInTheDocument();
    expect(container.querySelector(".composer-working-status")).toBeInTheDocument();
    expect(container.querySelector(".composer-thinking-latest")).toHaveTextContent("next");
    expect(container.querySelector(".composer-thinking-transcript")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "停止 worker 的当前请求" })).toBeInTheDocument();
    const compactExpand = screen.getByRole("button", { name: "展开过程" });
    expect(compactExpand).toHaveAttribute("aria-expanded", "false");
    await user.click(compactExpand);
    expect(screen.getByText("运行过程")).toBeInTheDocument();
    expect(container.querySelector(".composer-thinking-transcript")).toHaveTextContent("next");
  });

  it("lets users resize the expanded process window", () => {
    const { container } = renderConversationComposer({
      workingParticipants: [
        {
          id: "u-manager",
          leaseID: "lease-1",
          name: "manager",
          thinkingText: "line one\nline two\nline three",
        },
      ],
    });

    const workingPanel = container.querySelector<HTMLElement>(".composer-working");
    const resizeHandle = screen.getByRole("separator", { name: "调整运行过程高度" });

    expect(workingPanel).toHaveStyle({ "--composer-thinking-transcript-max-height": "104px" });
    fireEvent.pointerDown(resizeHandle, { clientY: 200 });
    fireEvent.pointerMove(window, { clientY: 120 });
    fireEvent.pointerUp(window);
    expect(workingPanel).toHaveStyle({ "--composer-thinking-transcript-max-height": "184px" });

    fireEvent.keyDown(resizeHandle, { key: "ArrowDown" });
    expect(workingPanel).toHaveStyle({ "--composer-thinking-transcript-max-height": "168px" });
    fireEvent.keyDown(resizeHandle, { key: "End" });
    expect(workingPanel).toHaveStyle({ "--composer-thinking-transcript-max-height": "280px" });
  });

  it("reopens the working process when a new turn starts", async () => {
    const user = userEvent.setup();
    const firstParticipant: ConversationWorkingParticipant = {
      id: "u-manager",
      leaseID: "lease-1",
      name: "manager",
      thinkingText: "first turn",
    };
    const secondParticipant: ConversationWorkingParticipant = {
      id: "u-manager",
      leaseID: "lease-2",
      name: "manager",
      thinkingText: "second turn",
    };

    const { container, rerender } = renderConversationComposer({
      workingParticipants: [firstParticipant],
    });

    await user.click(screen.getByRole("button", { name: "收起过程" }));
    expect(container.querySelector(".composer-thinking-transcript")).not.toBeInTheDocument();

    rerender(
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
        t={defaultTranslate}
        workingParticipants={[secondParticipant]}
        onAddAttachments={vi.fn()}
        onApplyMention={vi.fn()}
        onApplySlashCandidate={vi.fn()}
        onComposerCompositionEnd={vi.fn()}
        onComposerCompositionStart={vi.fn()}
        onComposerKeyDown={vi.fn()}
        onProviderLogin={vi.fn()}
        onSendMessage={vi.fn()}
        onSyncComposer={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "收起过程" })).toHaveAttribute("aria-expanded", "true");
    expect(container.querySelector(".composer-thinking-transcript")).toHaveTextContent("second turn");
  });

  it("keeps the working process visible briefly after the turn finishes", () => {
    vi.useFakeTimers();
    const participant: ConversationWorkingParticipant = {
      activity: {
        action: ConversationWorkingActions.replying,
        summary: "正在整理答案",
      },
      canStop: true,
      id: "u-manager",
      name: "manager",
    };

    try {
      const { container, rerender } = renderConversationComposer({
        workingParticipants: [participant],
        onStopWorkingTurn: vi.fn(),
      });

      expect(screen.getAllByText("正在整理答案").length).toBeGreaterThanOrEqual(2);
      expect(container.querySelector(".composer-working-stop")).toBeInTheDocument();

      rerender(
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
          t={defaultTranslate}
          workingParticipants={[]}
          onAddAttachments={vi.fn()}
          onApplyMention={vi.fn()}
          onApplySlashCandidate={vi.fn()}
          onComposerCompositionEnd={vi.fn()}
          onComposerCompositionStart={vi.fn()}
          onComposerKeyDown={vi.fn()}
          onProviderLogin={vi.fn()}
          onSendMessage={vi.fn()}
          onStopWorkingTurn={vi.fn()}
          onSyncComposer={vi.fn()}
        />,
      );

      expect(screen.getAllByText("正在整理答案").length).toBeGreaterThanOrEqual(2);
      expect(container.querySelector(".composer-working-stop")).not.toBeInTheDocument();

      act(() => {
        vi.advanceTimersByTime(3500);
      });

      expect(screen.queryByText("正在整理答案")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("ConversationComposer attachment menu", () => {
  it("does not expose the retired connector actions", async () => {
    const user = userEvent.setup();
    renderConversationComposer();
    await user.click(screen.getByRole("button", { name: "添加内容" }));
    expect(screen.queryByText("GitHub")).not.toBeInTheDocument();
    expect(screen.queryByText("GitLab")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "连接" })).not.toBeInTheDocument();
  });
});
