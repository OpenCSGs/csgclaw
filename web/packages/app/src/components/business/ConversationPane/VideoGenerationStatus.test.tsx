import { render, screen } from "@testing-library/react";
import { createTranslator } from "@/shared/i18n";
import { VideoGenerationStatus } from "./VideoGenerationStatus";

it.each([
  ["zh", "failed", "invalid_video_seconds", "视频生成失败"],
  ["en", "failed", "invalid_parameter", "Video generation failed"],
  ["zh", "delivery_failed", "video_delivery_failed", "视频已生成，但发送失败"],
] as const)("shows only the failure status in %s for %s / %s", (locale, state, code, label) => {
  render(
    <VideoGenerationStatus
      message={{
        content: "Original video prompt",
        metadata: {
          video_generation: {
            state,
            error: code,
            error_details: {
              code,
              message: 'The parameter duration is not valid; received: "2" Request id: private-123',
            },
          },
        },
      }}
      t={createTranslator(locale)}
    />,
  );

  expect(screen.getByText(label)).toBeVisible();
  expect(
    screen.queryByText(/parameter duration|private-123|invalid_video_seconds|invalid_parameter/),
  ).not.toBeInTheDocument();
  expect(screen.queryByText(/重试|try again/i)).not.toBeInTheDocument();
});
