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
    expect(screen.queryByRole("button")).toBeNull();
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
  it("replaces generating state with the provider reason and diagnostic code", () => {
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
    expect(screen.getByText("Your request was rejected by the safety system.")).toBeInTheDocument();
    expect(screen.getByText(/HTTP 400.*moderation_blocked.*request-123/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重新生成" })).toBeEnabled();
  });
});
