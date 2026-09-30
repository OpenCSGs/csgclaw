import { describe, expect, it } from "vitest";
import { videoProgressTiming } from "@/models/videoGeneration";
import { progressDuration, type TurnProgress } from "@/models/turnProgress";

const start = "2026-09-30T02:44:00Z";
const progress: TurnProgress = {
  id: "turn",
  revision: 1,
  status: "succeeded",
  started_at: start,
  ended_at: "2026-09-30T02:44:09Z",
  updated_at: start,
  items: [],
};
const video = (state: string, ended_at?: string) => ({
  content: "",
  metadata: { video_generation: { state, started_at: "2026-09-30T02:44:06Z", ended_at } },
});

describe("asynchronous video timing", () => {
  it("keeps advancing after the runtime reply ends and freezes at video completion", () => {
    const running = videoProgressTiming(progress, [video("generating")])!;
    expect(running.progress.status).toBe("running");
    expect(progressDuration(running.progress, Date.parse("2026-09-30T02:45:10Z"))).toBe("1m 10s");
    const done = videoProgressTiming(progress, [video("completed", "2026-09-30T02:46:10Z")])!;
    expect(progressDuration(done.progress, Date.parse("2026-09-30T03:00:00Z"))).toBe("2m 10s");
    expect(progress.ended_at).toBe("2026-09-30T02:44:09Z");
  });
  it("waits for every video, includes failures, and never adds parallel durations", () => {
    const failed = video("failed", "2026-09-30T02:45:00Z");
    expect(videoProgressTiming(progress, [failed, video("generating")])?.pending).toBe(true);
    const done = videoProgressTiming(progress, [failed, video("completed", "2026-09-30T02:46:00Z")])!;
    expect(done.failed).toBe(true);
    expect(progressDuration(done.progress, 0)).toBe("2m 0s");
  });
  it("does not invent timing for missing or invalid timestamps", () => {
    expect(
      videoProgressTiming(progress, [{ content: "", metadata: { video_generation: { state: "completed" } } }]),
    ).toBeNull();
    expect(videoProgressTiming(progress, [video("completed", "invalid")])).toBeNull();
  });
});
