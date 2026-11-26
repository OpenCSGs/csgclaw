import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { retryImageGenerationRequest } from "@/api/im";
import { createTranslator } from "@/shared/i18n";
import { ImageGenerationStatus } from "./ImageGenerationStatus";

vi.mock("@/api/im", () => ({ retryImageGenerationRequest: vi.fn() }));
const t = createTranslator("zh");

describe("Image generation status", () => {
  it("shows real generation state without offering duplicate generation", () => {
    render(
      <ImageGenerationStatus
        roomID="room-1"
        t={t}
        message={{ id: "image-1", content: "blue sky", metadata: { image_generation: { state: "generating" } } }}
      />,
    );
    expect(screen.getByRole("status").textContent).toContain("正在生成图片");
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("keeps a delivered attachment and the prompt available when delivery reports a failure", async () => {
    const onPreview = vi.fn();
    render(
      <ImageGenerationStatus
        t={t}
        message={{ id: "image-1", content: "blue sky", metadata: { image_generation: { state: "delivery_failed" } } }}
        result={
          <button type="button" onClick={onPreview}>
            Preview attachment
          </button>
        }
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Preview attachment" }));
    expect(onPreview).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "重新发送图片" })).toBeEnabled();

    const disclosure = screen.getByText(t("imageViewPrompt"));
    expect(disclosure.closest("details")).not.toHaveAttribute("open");
    await user.click(disclosure);
    expect(disclosure.closest("details")).toHaveAttribute("open");
    expect(screen.getByText("blue sky")).toBeVisible();
  });

  it("retries only the failed image task and blocks repeated clicks", async () => {
    let finish: ((result: { status: string }) => void) | undefined;
    vi.mocked(retryImageGenerationRequest).mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const user = userEvent.setup();
    render(
      <ImageGenerationStatus
        roomID="room-1"
        t={t}
        message={{
          id: "image-1",
          content: "blue sky",
          metadata: { image_generation: { state: "delivery_failed", error: "image_delivery_failed" } },
        }}
      />,
    );
    const button = screen.getByRole("button", { name: "重新发送图片" });
    await user.click(button);
    expect(retryImageGenerationRequest).toHaveBeenCalledWith("image-1", "room-1");
    expect(button).toBeDisabled();
    finish?.({ status: "succeeded" });
    await waitFor(() => expect(button).not.toBeDisabled());
  });
  it.each([
    ["zh", "重新生成", "原始参考图片已不可用，请重新上传图片后发起新的编辑请求。"],
    ["en", "Regenerate", "The original reference image is unavailable. Upload it again and start a new edit."],
  ] as const)(
    "localizes the missing reference without exposing provider diagnostics in %s",
    async (locale, retryLabel, message) => {
      vi.mocked(retryImageGenerationRequest).mockRejectedValueOnce({
        status: 409,
        code: "image_reference_unavailable",
        message: "The original reference image is unavailable.",
      });
      render(
        <ImageGenerationStatus
          roomID="room-1"
          t={createTranslator(locale)}
          message={{
            id: "image-1",
            content: "original prompt",
            metadata: {
              image_generation: { state: "failed", error_details: { message: "Previous provider failure." } },
            },
          }}
        />,
      );

      await userEvent.setup().click(screen.getByRole("button", { name: retryLabel }));
      expect(await screen.findByRole("alert")).toHaveTextContent(message);
      expect(screen.queryByText("Previous provider failure.")).not.toBeInTheDocument();
    },
  );

  it("shows configuration guidance for an older failed task without offering regeneration", () => {
    render(
      <ImageGenerationStatus
        t={t}
        message={{
          id: "image-1",
          content: "original prompt",
          metadata: { image_generation: { state: "failed", error: "image_model_not_configured" } },
        }}
      />,
    );
    expect(screen.getByText(t("image_model_not_configured"))).toBeVisible();
    expect(screen.queryByRole("button", { name: t("imageRegenerate") })).not.toBeInTheDocument();
    expect(screen.queryByText("image_model_not_configured")).not.toBeInTheDocument();
  });

  it.each(["image_model_not_configured", "upstream secret diagnostics"])(
    "does not expose raw retry result %s",
    async (message) => {
      vi.mocked(retryImageGenerationRequest).mockResolvedValueOnce({
        status: "failed",
        error: { code: "runtime_failed", message },
      });
      render(
        <ImageGenerationStatus
          t={t}
          roomID="room-1"
          message={{ id: "image-1", content: "original prompt", metadata: { image_generation: { state: "failed" } } }}
        />,
      );
      await userEvent.setup().click(screen.getByRole("button", { name: t("imageRegenerate") }));
      expect(await screen.findByRole("alert")).toHaveTextContent(
        t(message === "image_model_not_configured" ? message : "imageRetryFailed"),
      );
      expect(screen.queryByText(message)).not.toBeInTheDocument();
    },
  );

  it("replaces generating state with localized guidance and hides provider diagnostics", () => {
    const pending = {
      id: "image-1",
      content: "original prompt",
      metadata: { image_generation: { state: "generating" } },
    };
    const { rerender } = render(<ImageGenerationStatus roomID="room-1" message={pending} t={t} />);
    expect(screen.getByText("正在生成图片")).toBeInTheDocument();
    rerender(
      <ImageGenerationStatus
        roomID="room-1"
        t={t}
        message={{
          ...pending,
          metadata: {
            image_generation: {
              state: "failed",
              error: "image_generation_output_blocked",
              error_details: {
                http_status: 400,
                code: "moderation_blocked",
                stage: "output",
                message: "Your request was rejected by the safety system.",
                request_id: "request-123",
              },
            },
          },
        }}
      />,
    );
    expect(screen.queryByText("正在生成图片")).toBeNull();
    expect(screen.getByText("生成结果未通过生图服务的安全检查，请调整描述后重试。")).toBeInTheDocument();
    expect(screen.queryByText("Your request was rejected by the safety system.")).not.toBeInTheDocument();
    expect(screen.queryByText(/HTTP 400|moderation_blocked|request-123/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重新生成" })).toBeEnabled();
  });
});
