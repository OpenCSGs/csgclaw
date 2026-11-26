import { act, render as renderWithProviders, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import type { LocaleCode } from "@/models/conversations";
import { FeedbackDialog } from "@/components/business/Feedback";
import { useFeedback } from "@/components/business/Feedback/useFeedback";
import { TooltipProvider } from "@/components/ui";
import { createTranslator } from "@/shared/i18n";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const props = {
  open: true,
  busy: false,
  description: "",
  images: [],
  error: "",
  t: (key: string) => key,
  onOpenChange: vi.fn(),
  onDescriptionChange: vi.fn(),
  archives: [],
  receipt: null,
  onAddAttachments: vi.fn(),
  onRemoveAttachment: vi.fn(),
  onSubmit: vi.fn(),
};

function render(ui: ReactElement) {
  return renderWithProviders(ui, { wrapper: TooltipProvider });
}

it("requires text or images and sends text changes to the controller", async () => {
  render(<FeedbackDialog {...props} />);
  expect(screen.getByRole("button", { name: "feedbackSubmit" })).toBeDisabled();
  await userEvent.setup().type(screen.getByRole("textbox"), "x");
  expect(props.onDescriptionChange).toHaveBeenCalledWith("x");
});

it("disables edits and submission while sending", () => {
  render(<FeedbackDialog {...props} description="Problem" busy />);
  expect(screen.getByRole("textbox")).toBeDisabled();
  expect(screen.getByRole("button", { name: "feedbackSubmit" })).toBeDisabled();
});

function FeedbackForm({ locale, site }: { locale: LocaleCode; site?: string }) {
  const t = createTranslator(locale);
  const feedback = useFeedback(t);
  return (
    <FeedbackDialog
      open
      {...feedback}
      site={site}
      t={t}
      onOpenChange={() => {}}
      onDescriptionChange={feedback.changeDescription}
      onAddAttachments={feedback.addAttachments}
      onRemoveAttachment={feedback.removeAttachment}
      onClearError={feedback.clearError}
      onClearRejections={feedback.clearRejections}
      onSubmit={() => void feedback.submit()}
    />
  );
}

it("keeps attachment limits visible, reveals formats on demand, and explains where selected archives go", async () => {
  const user = userEvent.setup();
  render(<FeedbackForm locale="zh" site="https://example.test" />);
  expect(screen.getByText("图片 0/6 · 单张 ≤ 5 MB")).toBeVisible();
  expect(screen.getByText("压缩包 0/3 · 单个 ≤ 10 MB")).toBeVisible();
  const formats = screen.getByText("支持的文件格式");
  expect(formats.closest("details")).not.toHaveAttribute("open");
  await user.click(formats);
  expect(formats.closest("details")).toHaveAttribute("open");
  expect(screen.getByText("图片：JPG、JPEG、PNG")).toBeVisible();
  expect(screen.getByText("压缩包：ZIP、RAR、7Z、TAR、GZ")).toBeVisible();
  expect(screen.queryByText(/提交时，压缩包会上传/)).not.toBeInTheDocument();

  const input = screen.getByRole("dialog").querySelector<HTMLInputElement>('input[type="file"]');
  if (!input) throw new Error("Feedback file input is missing");
  await user.upload(input, new File(["zip"], "diagnostics.zip", { type: "application/zip" }));
  expect(screen.getByText("压缩包 1/3 · 单个 ≤ 10 MB")).toBeVisible();
  expect(screen.getByRole("button", { name: "提交" })).toHaveAccessibleDescription(
    "提交时，压缩包会上传至 example.test，下载链接会写入工单。请先移除凭据和个人信息。",
  );
  await user.click(screen.getByRole("button", { name: "移除 diagnostics.zip" }));
  expect(screen.getByText("压缩包 0/3 · 单个 ≤ 10 MB")).toBeVisible();
  expect(screen.queryByText(/提交时，压缩包会上传/)).not.toBeInTheDocument();
});

it("dismisses attachment warnings and returns focus to the file chooser", async () => {
  const user = userEvent.setup({ applyAccept: false });
  render(<FeedbackForm locale="zh" />);
  const input = screen.getByRole("dialog").querySelector<HTMLInputElement>('input[type="file"]');
  if (!input) throw new Error("Feedback file input is missing");
  await user.upload(input, new File(["text"], "notes.txt", { type: "text/plain" }));
  expect(screen.getByRole("alert")).toHaveTextContent("notes.txt");
  await user.click(screen.getByRole("button", { name: "关闭附件提示" }));
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "添加附件" })).toHaveFocus();
});

it("shows the filename on focus and opens the image preview with the keyboard", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL() {
        return "blob:feedback-image";
      }
      static revokeObjectURL() {}
    },
  );
  const image = new File(["image"], "问题复现步骤 1.png", { type: "image/png" });
  render(<FeedbackDialog {...props} images={[image]} t={createTranslator("zh")} />);
  const trigger = screen.getByRole("button", { name: `预览图片 ${image.name}` });
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  act(() => trigger.focus());
  expect(await screen.findByRole("tooltip")).toHaveTextContent(image.name);
  await user.keyboard("{Enter}");
  const preview = screen.getByRole("dialog", { name: image.name });
  expect(within(preview).getByRole("img", { name: image.name })).toHaveAttribute("src", "blob:feedback-image");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: image.name })).not.toBeInTheDocument();
  expect(screen.getByRole("dialog", { name: "意见反馈" })).toBeVisible();
  expect(trigger).toHaveFocus();
});

it("shows one count warning per attachment type and names files rejected for other reasons", () => {
  render(
    <FeedbackDialog
      {...props}
      rejections={[
        { file: "first.png", reason: "feedbackImageCount" },
        { file: "second.png", reason: "feedbackImageCount" },
        { file: "first.zip", reason: "feedbackArchiveCount" },
        { file: "second.zip", reason: "feedbackArchiveCount" },
        { file: "notes.txt", reason: "feedbackAttachmentFormat" },
        { file: "large.png", reason: "feedbackImageTooLarge" },
      ]}
    />,
  );
  const warnings = within(screen.getByRole("alert")).getAllByRole("listitem");
  expect(warnings.map((warning) => warning.textContent)).toEqual([
    "feedbackImageCount",
    "feedbackArchiveCount",
    "notes.txt: feedbackAttachmentFormat",
    "large.png: feedbackImageTooLarge",
  ]);
});

it("shows submission and status check messages in the body and brings updates into view", async () => {
  const user = userEvent.setup();
  const onCheckStatus = vi.fn();
  const pending = { ...props, awaitingStatus: true, requestID: "0123456789abcdef0123456789abcdef", onCheckStatus };
  const { rerender } = render(<FeedbackDialog {...pending} busy />);
  const footer = screen.getByRole("button", { name: "feedbackSubmit" }).closest(".csg-dialog-footer") as HTMLElement;
  const body = screen.getByRole("dialog").querySelector(".csg-dialog-body") as HTMLElement;
  const status = within(body).getByRole("status");
  expect(status).toHaveTextContent("feedbackSubmitting");
  rerender(<FeedbackDialog {...pending} />);
  expect(status).toHaveTextContent("feedbackOutcomeUnknown");
  await user.click(within(footer).getByRole("button", { name: "feedbackCheckStatus" }));
  expect(onCheckStatus).toHaveBeenCalledOnce();
  body.scrollTop = 240;
  rerender(<FeedbackDialog {...pending} checking />);
  expect(status).toHaveTextContent("feedbackCheckingStatus");
  expect(body.scrollTop).toBe(0);
  expect(within(footer).getByRole("button", { name: "feedbackCheckStatus" })).toBeDisabled();
  rerender(<FeedbackDialog {...pending} error="feedbackStillUnknown" />);
  expect(status).toHaveTextContent("feedbackStillUnknown");
  expect(within(footer).getByRole("button", { name: "feedbackCheckStatus" })).toBeEnabled();
});

it("offers a header close action and explains why adding attachments is disabled", async () => {
  const user = userEvent.setup();
  const onOpenChange = vi.fn();
  render(<FeedbackDialog {...props} awaitingStatus onOpenChange={onOpenChange} />);
  expect(screen.getByRole("button", { name: "feedbackChooseAttachments" })).toBeDisabled();
  act(() => screen.getByRole("group", { name: "feedbackChooseAttachments" }).focus());
  expect(await screen.findByRole("tooltip")).toHaveTextContent("feedbackAttachmentsAwaitingStatus");
  const close = screen.getByRole("dialog").querySelector<HTMLButtonElement>(".csg-dialog-close");
  expect(close).toHaveAccessibleName("close");
  await user.click(close!);
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

it.each([
  ["zh", "feedback_access_denied", 401, "无法验证本地服务访问权限，请通过本机页面或 CSGClaw App 提交反馈。"],
  [
    "en",
    "feedback_access_denied",
    401,
    "Local service access could not be verified. Submit feedback from the local page or CSGClaw App.",
  ],
  ["zh", "feedback_login_required", 401, "请登录 OpenCSG 后提交反馈。"],
  ["en", "feedback_login_required", 401, "Sign in to OpenCSG to submit feedback."],
  ["zh", "feedback_upload_failed", 502, "附件上传失败，反馈尚未提交。已保留填写内容和附件，可以重试。"],
  ["zh", "feedback_upload_too_large", 413, "门户拒绝了过大的附件，反馈尚未提交。请移除该附件或换用更小的文件后重试。"],
  [
    "en",
    "feedback_upload_too_large",
    413,
    "The portal rejected an attachment because it was too large. Remove it or choose a smaller file and try again. Your feedback has not been submitted.",
  ],
] as const)("shows the %s %s response and preserves feedback for retry", async (locale, code, status, expected) => {
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL() {
        return "blob:feedback-image";
      }
      static revokeObjectURL() {}
    },
  );
  let respond: (response: Response) => void = () => {};
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          respond = resolve;
        }),
    )
    .mockResolvedValueOnce(new Response(JSON.stringify({ success: true })));
  const t = createTranslator(locale);
  const user = userEvent.setup();
  render(<FeedbackForm locale={locale} />);
  await user.type(screen.getByRole("textbox"), "A problem");
  const image = new File(["image"], "feedback.png", { type: "image/png" });
  const imageInput = screen.getByRole("dialog").querySelector<HTMLInputElement>('input[type="file"]');
  if (!imageInput) throw new Error("Feedback image input is missing");
  await user.upload(imageInput, image);
  await user.click(screen.getByRole("button", { name: t("feedbackSubmit") }));

  expect(screen.getByRole("textbox")).toBeDisabled();
  for (const button of screen.getAllByRole("button", { name: t("close") })) expect(button).toBeDisabled();
  await act(async () => {
    respond(
      new Response(JSON.stringify({ error: { code, message: "unauthorized" } }), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  expect(await screen.findByRole("alert")).toHaveTextContent(expected);
  expect(screen.queryByText(t("errors.unauthorized"))).not.toBeInTheDocument();
  expect(screen.getByRole("textbox")).toHaveValue("A problem");
  expect(screen.getByRole("textbox")).toBeEnabled();
  expect(screen.getByRole("img", { name: image.name })).toBeInTheDocument();
  for (const button of screen.getAllByRole("button", { name: t("close") })) expect(button).toBeEnabled();
  expect(screen.getByRole("button", { name: t("feedbackSubmit") })).toBeEnabled();

  await user.click(screen.getByRole("button", { name: t("feedbackDismissSubmissionError") }));
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.getByRole("textbox")).toHaveFocus();
  expect(screen.getByRole("textbox")).toHaveValue("A problem");
  expect(screen.getByRole("img", { name: image.name })).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: t("feedbackSubmit") }));
  expect(fetchMock).toHaveBeenCalledTimes(2);
  const [path, options] = fetchMock.mock.calls[1];
  expect(path).toBe("api/v1/feedback");
  expect(options?.method).toBe("POST");
  expect(options?.body).toBeInstanceOf(FormData);
  const body = options?.body as FormData;
  expect(body.get("problem_description")).toBe("A problem");
  expect(body.getAll("images")).toEqual([image]);
  await waitFor(() => expect(screen.getByText(t("feedbackSuccess"))).toBeInTheDocument());
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  expect(screen.queryByRole("img", { name: image.name })).not.toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("accepts window paste and dropped files while the form is open", () => {
  const addAttachments = vi.fn();
  const { rerender } = render(<FeedbackDialog {...props} onAddAttachments={addAttachments} />);
  const image = new File(["image"], "screenshot.png");
  const archive = new File(["zip"], "diagnostics.zip");
  const paste = new Event("paste", { cancelable: true });
  Object.defineProperty(paste, "clipboardData", { value: { files: [image] } });
  window.dispatchEvent(paste);
  expect(paste.defaultPrevented).toBe(true);
  expect(addAttachments).toHaveBeenCalledWith([image]);
  const drop = new Event("drop", { bubbles: true, cancelable: true });
  Object.defineProperty(drop, "dataTransfer", { value: { files: [archive] } });
  screen.getByRole("textbox").dispatchEvent(drop);
  expect(drop.defaultPrevented).toBe(true);
  expect(addAttachments).toHaveBeenCalledWith([archive]);
  rerender(<FeedbackDialog {...props} open={false} onAddAttachments={addAttachments} />);
  window.dispatchEvent(paste);
  expect(addAttachments).toHaveBeenCalledTimes(2);
});

it("shows the issue number and a safe link after feedback is submitted", () => {
  const { rerender } = render(<FeedbackDialog {...props} />);
  const description = () => within(screen.getByRole("dialog").querySelector(".csg-dialog-description") as HTMLElement);
  const status = description().getByRole("status");
  expect(status).toBeEmptyDOMElement();
  rerender(
    <FeedbackDialog
      {...props}
      receipt={{ success: true, issue_id: 123, issue_url: "https://example.test/issues/123" }}
    />,
  );
  expect(description().getByRole("status")).toBe(status);
  expect(status).toHaveTextContent("feedbackSuccess");
  expect(screen.getByRole("link", { name: "feedbackViewIssue" })).toHaveAttribute(
    "href",
    "https://example.test/issues/123",
  );
  expect(screen.getByRole("button", { name: "feedbackCopyIssueNumber" })).toBeInTheDocument();
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  rerender(<FeedbackDialog {...props} receipt={{ success: true, issue_id: 123, issue_url: "javascript:alert(1)" }} />);
  expect(screen.queryByRole("link")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "feedbackCopyIssueNumber" })).toBeInTheDocument();
  rerender(<FeedbackDialog {...props} receipt={{ success: true }} />);
  expect(status).toHaveTextContent("feedbackSuccess");
  expect(screen.queryByRole("button", { name: "feedbackCopyIssueNumber" })).not.toBeInTheDocument();
});
