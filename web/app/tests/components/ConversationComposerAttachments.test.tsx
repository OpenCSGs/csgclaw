import { createRef } from "react";
import { createEvent, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationComposer } from "@/components/business/ConversationPane/ConversationComposer";
import type { ConversationComposerProps } from "@/components/business/ConversationPane/ConversationComposer";
import { createAttachmentDrafts } from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key, params) => {
  const labels: Record<string, string> = {
    composerAdd: "Add",
    composerAddContent: "Add content",
    composerFiles: "Files",
    inputPlaceholder: "Message",
    addAttachment: "Add attachment",
    attachments: "Attachments",
    previewAttachmentNamed: `Preview attachment: ${params?.name ?? ""}`,
    attachmentPreviewDescription: "Preview without leaving the app",
    attachmentPreviewFailed: "Preview failed",
    attachmentPreviewLoading: "Loading preview",
    attachmentPreviewUnavailable: "Preview unavailable",
    downloadAttachment: "download",
    close: "Close",
    attachmentsScrollPrevious: "View previous attachments",
    attachmentsScrollNext: "View more attachments",
    removeAttachment: "Remove attachment",
    removeAttachmentNamed: `Remove attachment: ${params?.name ?? ""}`,
    attachmentRemoved: `Removed attachment "${params?.name ?? ""}"`,
    attachmentsRemoved: `Removed ${params?.count ?? 0} attachments`,
    attachmentUploadingProgress: `Uploading ${params?.progress ?? 0}%`,
    attachmentUploadingNamed: `Uploading attachment: ${params?.name ?? ""}`,
    attachmentUploadFailed: "Upload failed",
    sendingWithProgress: `Sending ${params?.progress ?? 0}%`,
    retrySend: "Retry send",
    stopSending: "Stop sending",
    undo: "Undo",
    suggestedActions: "Suggested actions",
    suggestedActionsOnly: "Suggestions only; nothing runs automatically",
    composerTip: "Enter to send · Shift + Enter for a new line",
    send: "Send",
  };
  return labels[key] ?? key;
};

function renderComposer(props: Partial<ConversationComposerProps> = {}): ReturnType<typeof render> {
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
      t={t}
      onApplyMention={() => {}}
      onApplySlashCandidate={() => {}}
      onComposerCompositionEnd={() => {}}
      onComposerCompositionStart={() => {}}
      onComposerKeyDown={() => {}}
      onProviderLogin={() => {}}
      onSendMessage={() => {}}
      onSyncComposer={() => {}}
      {...props}
    />,
  );
}

describe("ConversationComposer attachments", () => {
  it("blocks message entry with the provided manager runtime warning", async () => {
    const user = userEvent.setup();
    const onSendMessage = vi.fn();
    renderComposer({
      composerDisabled: true,
      composerDisabledReason: "Install Codex CLI first.",
      draftText: "hello",
      onSendMessage,
    });

    expect(screen.getByText("Install Codex CLI first.")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Message" })).toHaveAttribute("contenteditable", "false");
    const sendButton = screen.getByRole("button", { name: "Send" });
    expect(sendButton).toBeDisabled();

    await user.click(sendButton);
    expect(onSendMessage).not.toHaveBeenCalled();
  });

  it("keeps file uploads available without legacy connector controls", async () => {
    const user = userEvent.setup();
    renderComposer();
    await user.click(screen.getByRole("button", { name: "Add content" }));
    const dialog = screen.getByRole("dialog", { name: "Add content" });
    expect(within(dialog).getByRole("button", { name: "Add attachment" })).toBeVisible();
    expect(within(dialog).queryByText("Connectors")).not.toBeInTheDocument();
    expect(screen.queryByText("GitHub")).not.toBeInTheDocument();
    expect(screen.queryByText("GitLab")).not.toBeInTheDocument();
    expect(within(dialog).getAllByRole("button")).toHaveLength(1);
  });

  it("places the add and send controls in one composer toolbar without visible shortcut copy", () => {
    const { container } = renderComposer();

    const row = container.querySelector(".composer-toolbar");
    expect(row).toBeInTheDocument();
    expect(within(row as HTMLElement).getByRole("button", { name: "Add content" })).toHaveClass("composer-add-button");
    expect(within(row as HTMLElement).getByRole("button", { name: "Send" })).toHaveClass("composer-send-button");
    expect(within(row as HTMLElement).getByText("Enter to send · Shift + Enter for a new line")).toHaveClass("sr-only");
    expect(container.querySelector(".composer-tip")).not.toBeInTheDocument();
  });

  it("opens the file picker from the Files menu item", async () => {
    const user = userEvent.setup();
    const inputClick = vi.spyOn(HTMLInputElement.prototype, "click");
    renderComposer();

    await user.click(screen.getByRole("button", { name: "Add content" }));
    await user.click(screen.getByRole("button", { name: "Add attachment" }));

    expect(inputClick).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog", { name: "Add content" })).not.toBeInTheDocument();
    inputClick.mockRestore();
  });

  it("allows sending an attachment-only draft", async () => {
    const user = userEvent.setup();
    const onSendMessage = vi.fn();
    const onRemoveAttachment = vi.fn();
    const onPreviewAttachment = vi.fn();
    const attachmentDrafts = createAttachmentDrafts([new File(["hello"], "note.txt", { type: "text/plain" })]);
    renderComposer({
      attachmentDrafts,
      draftText: "",
      onRemoveAttachment,
      onPreviewAttachment,
      onSendMessage,
    });

    expect(screen.getByTitle("note.txt")).toBeInTheDocument();
    const sendButton = screen.getByRole("button", { name: "Send" });
    expect(sendButton).not.toBeDisabled();

    expect(screen.getByTitle("note.txt")).toHaveAttribute("title", "note.txt");
    await user.click(screen.getByRole("button", { name: "Preview attachment: note.txt" }));
    expect(onPreviewAttachment).toHaveBeenCalledWith(
      expect.objectContaining({
        index: 0,
        items: [expect.objectContaining({ file: attachmentDrafts[0].file, name: "note.txt" })],
      }),
    );
    await user.click(screen.getByRole("button", { name: "Remove attachment: note.txt" }));
    expect(onRemoveAttachment).toHaveBeenCalledWith(attachmentDrafts[0].id);
    await user.click(sendButton);
    expect(onSendMessage).toHaveBeenCalledTimes(1);
  });

  it("offers mouse, keyboard, and wheel navigation when attachments overflow", async () => {
    const user = userEvent.setup();
    const attachmentDrafts = createAttachmentDrafts([
      new File(["one"], "one.txt", { type: "text/plain" }),
      new File(["two"], "two.txt", { type: "text/plain" }),
      new File(["three"], "three.txt", { type: "text/plain" }),
    ]);
    const { container } = renderComposer({ attachmentDrafts });
    const strip = container.querySelector<HTMLElement>(".attachment-draft-strip");
    expect(strip).not.toBeNull();
    Object.defineProperties(strip!, {
      clientWidth: { configurable: true, value: 240 },
      scrollLeft: { configurable: true, value: 0, writable: true },
      scrollWidth: { configurable: true, value: 700 },
    });

    fireEvent.scroll(strip!);
    expect(screen.getByRole("button", { name: "View more attachments" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "View previous attachments" })).not.toBeInTheDocument();
    expect(container.querySelector(".attachment-scroll-fade.is-next")).toBeInTheDocument();
    expect(container.querySelector(".attachment-scroll-fade.is-previous")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "View more attachments" }));
    expect(strip!.scrollLeft).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "View previous attachments" })).toBeInTheDocument();
    expect(container.querySelector(".attachment-scroll-fade.is-previous")).toBeInTheDocument();

    const scrollLeftBeforeWheel = strip!.scrollLeft;
    const wheelEvent = createEvent.wheel(strip!, { cancelable: true, deltaX: 0, deltaY: 40 });
    fireEvent(strip!, wheelEvent);
    expect(strip!.scrollLeft).toBeGreaterThan(scrollLeftBeforeWheel);
  });

  it("accepts files from the picker, paste, and drag and drop", async () => {
    const user = userEvent.setup();
    const onAddAttachments = vi.fn();
    const { container } = renderComposer({ onAddAttachments });
    const pickerFile = new File(["picker"], "picker.txt", { type: "text/plain" });
    const pastedFile = new File(["image"], "pasted.png", { type: "image/png" });
    const droppedFile = new File(["drop"], "dropped.pdf", { type: "application/pdf" });

    const input = container.querySelector<HTMLInputElement>('input[type="file"]');
    expect(input).not.toBeNull();
    await user.upload(input!, pickerFile);

    const editor = screen.getByRole("textbox", { name: "Message" });
    expect(editor).toHaveAttribute("aria-multiline", "true");
    fireEvent.paste(editor, {
      clipboardData: {
        files: [pastedFile],
        getData: () => "",
        items: [],
      },
    });

    const composerBox = container.querySelector<HTMLElement>(".composer-box");
    expect(composerBox).not.toBeNull();
    const dragData = {
      dropEffect: "none",
      files: [],
      items: [],
      types: ["Files"],
    };
    const dragOverEvent = createEvent.dragOver(composerBox!, { dataTransfer: dragData });
    fireEvent(composerBox!, dragOverEvent);
    expect(dragOverEvent.defaultPrevented).toBe(true);
    expect(dragData.dropEffect).toBe("copy");

    fireEvent.drop(composerBox!, {
      dataTransfer: {
        files: [droppedFile],
        items: [],
      },
    });

    expect(onAddAttachments).toHaveBeenNthCalledWith(1, [pickerFile]);
    expect(onAddAttachments).toHaveBeenNthCalledWith(2, [pastedFile]);
    expect(onAddAttachments).toHaveBeenNthCalledWith(3, [droppedFile]);
  });

  it("shows upload progress and lets the user stop sending", async () => {
    const user = userEvent.setup();
    const onStopSend = vi.fn();
    const attachmentDrafts = createAttachmentDrafts([new File(["hello"], "report.final.pdf")]);
    renderComposer({
      attachmentDrafts,
      sendProgress: 42,
      sendStatus: "sending",
      onStopSend,
    });

    expect(screen.getByRole("progressbar", { name: "Uploading attachment: report.final.pdf" })).toHaveAttribute(
      "aria-valuenow",
      "42",
    );
    expect(screen.getByText("Sending 42%")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Stop sending" }));
    expect(onStopSend).toHaveBeenCalledTimes(1);
  });

  it("offers retry after failure and undo after attachment removal", async () => {
    const user = userEvent.setup();
    const onRetrySend = vi.fn();
    const onUndoRemoveAttachment = vi.fn();
    renderComposer({
      removedAttachmentCount: 1,
      removedAttachmentName: "report.pdf",
      sendError: "Network unavailable",
      sendStatus: "failed",
      onRetrySend,
      onUndoRemoveAttachment,
    });

    expect(screen.getByRole("alert")).toHaveTextContent("Network unavailable");
    await user.click(screen.getByRole("button", { name: "Retry send" }));
    await user.click(screen.getByRole("button", { name: "Undo" }));
    expect(onRetrySend).toHaveBeenCalledTimes(1);
    expect(onUndoRemoveAttachment).toHaveBeenCalledTimes(1);
  });

  it("summarizes a batch of removed attachments behind one undo action", () => {
    renderComposer({
      removedAttachmentCount: 3,
      onUndoRemoveAttachment: vi.fn(),
    });

    expect(screen.getByRole("status")).toHaveTextContent("Removed 3 attachments");
    expect(screen.getByRole("button", { name: "Undo" })).toBeInTheDocument();
  });

  it("suggests natural-language actions without running them automatically", async () => {
    const user = userEvent.setup();
    const onApplySlashCandidate = vi.fn();
    renderComposer({
      draftText: "帮我创建一个 dev 智能体",
      onApplySlashCandidate,
    });

    expect(onApplySlashCandidate).not.toHaveBeenCalled();
    expect(screen.getByText("Suggestions only; nothing runs automatically")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "/创建智能体" }));
    expect(onApplySlashCandidate).toHaveBeenCalledWith("创建智能体");
  });
});
