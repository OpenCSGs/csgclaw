import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { LocaleCode } from "@/models/conversations";
import { FeedbackDialog } from "@/pages/SettingsPage/components/FeedbackDialog";
import { useFeedback } from "@/pages/SettingsPage/useFeedback";
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
  onImagesChange: vi.fn(),
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
      onImagesChange={feedback.changeImages}
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
        status: 401,
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
  await waitFor(() => expect(screen.getByRole("textbox")).toHaveValue(""));
  expect(screen.queryByRole("img", { name: image.name })).not.toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
