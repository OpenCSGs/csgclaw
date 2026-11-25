import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { LocaleCode } from "@/models/conversations";
import { FeedbackDialog } from "@/components/business/Feedback";
import { useFeedback } from "@/components/business/Feedback/useFeedback";
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

function FeedbackForm({ locale }: { locale: LocaleCode }) {
  const t = createTranslator(locale);
  const feedback = useFeedback(t);
  return (
    <FeedbackDialog
      open
      {...feedback}
      t={t}
      onOpenChange={() => {}}
      onDescriptionChange={feedback.changeDescription}
      onAddAttachments={feedback.addAttachments}
      onRemoveAttachment={feedback.removeAttachment}
      onSubmit={() => void feedback.submit()}
    />
  );
}

it.each([
  ["zh", "feedback_access_denied", "无法验证本地服务访问权限，请通过本机页面或 CSGClaw App 提交反馈。"],
  [
    "en",
    "feedback_access_denied",
    "Local service access could not be verified. Submit feedback from the local page or CSGClaw App.",
  ],
  ["zh", "feedback_login_required", "请登录 OpenCSG 后提交反馈。"],
  ["en", "feedback_login_required", "Sign in to OpenCSG to submit feedback."],
  ["zh", "feedback_upload_too_large", "门户拒绝了过大的附件，反馈尚未提交。请移除该附件或换用更小的文件后重试。"],
  [
    "en",
    "feedback_upload_too_large",
    "The portal rejected an attachment because it was too large. Remove it or choose a smaller file and try again. Your feedback has not been submitted.",
  ],
] as const)("shows the %s %s response and preserves feedback for retry", async (locale, code, expected) => {
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
  expect(screen.getByRole("button", { name: t("close") })).toBeDisabled();
  await act(async () => {
    respond(
      new Response(JSON.stringify({ error: { code, message: "unauthorized" } }), {
        status: code === "feedback_upload_too_large" ? 413 : 401,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  expect(await screen.findByRole("alert")).toHaveTextContent(expected);
  expect(screen.queryByText(t("errors.unauthorized"))).not.toBeInTheDocument();
  expect(screen.getByRole("textbox")).toHaveValue("A problem");
  expect(screen.getByRole("textbox")).toBeEnabled();
  expect(screen.getByRole("img", { name: image.name })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: t("close") })).toBeEnabled();
  expect(screen.getByRole("button", { name: t("feedbackSubmit") })).toBeEnabled();

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
  const status = screen.getByRole("status");
  expect(status).toBeEmptyDOMElement();
  rerender(
    <FeedbackDialog
      {...props}
      receipt={{ success: true, issue_id: 123, issue_url: "https://example.test/issues/123" }}
    />,
  );
  expect(screen.getByRole("status")).toBe(status);
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
  expect(screen.getByRole("status")).toHaveTextContent("feedbackSuccess");
  expect(screen.queryByRole("button", { name: "feedbackCopyIssueNumber" })).not.toBeInTheDocument();
});
