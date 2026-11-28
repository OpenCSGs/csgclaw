import { describe, it, expect } from "vitest";
import {
  draftScheduleFromTask,
  nextRunISOFromDraft,
  scheduleDetailFromDraft,
  scheduleDraftBounds,
} from "@/pages/TasksPage/components/TasksView/ScheduleControls/scheduleDraft";
import {
  computeNextRunAt,
  emptyScheduleDetail,
  normalizeScheduledTask,
  scheduledTaskExecutors,
  scheduledTaskMention,
  scheduledTaskWallTimeISO,
} from "@/models/scheduledTasks";

describe("scheduled task executors", () => {
  it("includes Manager and puts it before other executors without mutating the list", () => {
    const agents = [
      { id: "agent-linda", name: "Linda", role: "worker" },
      { id: "agent-manager", name: "Manager", role: "manager" },
    ];
    expect(scheduledTaskExecutors(agents).map((item) => item.id)).toEqual(["agent-manager", "agent-linda"]);
    expect(agents[0].id).toBe("agent-linda");
  });
  it.each([
    ["汇总一下@Lin", 8, { query: "Lin", start: 4, end: 8 }],
    ["明天@ 继续报告", 3, { query: "", start: 2, end: 3 }],
    ["@测试工程师 Linda ", 12, null],
    ["明天下午汇总报告", 8, null],
  ])("recognizes mentions at the caret: %s", (text, caret, want) => {
    expect(scheduledTaskMention(text, caret)).toEqual(want);
  });
});

describe("schedule rules", () => {
  it("retains Sunday and interval lengths returned by the API", () => {
    const task = normalizeScheduledTask({
      id: "schedule-1",
      recurrence: "biweekly",
      starts_at: "2026-12-01T00:00:00Z",
      schedule_detail: { weekdays: [0, 4, 0, -1], interval_minutes: 60 * 24 * 60 },
    });
    expect(task).toMatchObject({
      recurrence: "biweekly",
      starts_at: "2026-12-01T00:00:00Z",
      schedule: { weekdays: [0, 4], interval_minutes: 60 * 24 * 60 },
    });
  });

  it("honors selected weekdays when the effective period starts later", () => {
    const next = computeNextRunAt({
      recurrence: "weekly",
      schedule: { ...emptyScheduleDetail(), weekdays: [0] },
      clock: "09:00",
      now: new Date(2026, 6, 1, 9, 0),
      startsAt: new Date(2026, 6, 6),
    });
    expect(next).toEqual(new Date(2026, 6, 12, 9, 0));
  });

  it.each(["weekly", "biweekly"] as const)("skips the missing spring-forward time for %s schedules", (recurrence) => {
    const next = computeNextRunAt({
      recurrence,
      schedule: { ...emptyScheduleDetail(), weekdays: [0], time_zone: "America/New_York" },
      clock: "02:30",
      now: new Date("2026-03-07T17:00:00Z"),
    });
    expect(next?.toISOString()).toBe("2026-03-15T06:30:00.000Z");
  });

  it("edits clock and date conditions in the saved schedule time zone", () => {
    const task = normalizeScheduledTask({
      id: "schedule-1",
      recurrence: "weekly",
      next_run_at: "2026-07-06T01:00:00Z",
      starts_at: "2026-07-05T16:00:00Z",
      expires_at: "2026-07-31T15:59:59.999Z",
      schedule_detail: { clock: "09:00", weekdays: [1], time_zone: "Asia/Shanghai" },
    })!;
    const draft = draftScheduleFromTask(task);
    expect(draft).toMatchObject({ time: "09:00", weekdays: [1], startsDate: "2026-07-06", expiresDate: "2026-07-31" });
    expect(scheduledTaskWallTimeISO(draft.expiresDate, "23:59:59.999", draft.timeZone)).toBe(task.expires_at);
    const edited = { ...draft, startsDate: "2026-07-13" };
    expect(scheduleDetailFromDraft(edited).time_zone).toBe("Asia/Shanghai");
    expect(nextRunISOFromDraft(edited, new Date("2026-07-06T00:00:00Z"))).toBe("2026-07-13T01:00:00.000Z");
  });

  it("retains the alternating week when a validity date changes", () => {
    const task = normalizeScheduledTask({
      id: "schedule-1",
      recurrence: "biweekly",
      next_run_at: "2026-12-21T09:00:00Z",
      schedule_detail: { clock: "09:00", weekdays: [1], time_zone: "UTC" },
    })!;
    const edited = { ...draftScheduleFromTask(task), startsDate: "2026-12-09" };
    const { startsAt } = scheduleDraftBounds(edited, task);
    expect(nextRunISOFromDraft(edited, new Date("2026-12-08T10:00:00Z"), startsAt, new Date(task.next_run_at))).toBe(
      "2026-12-21T09:00:00.000Z",
    );
  });

  it("finds the next leap day across a non-leap century", () => {
    const next = computeNextRunAt({
      recurrence: "yearly",
      schedule: { ...emptyScheduleDetail(), month: 2, day: 29 },
      clock: "09:00",
      now: new Date(2096, 2, 1),
    });
    expect(next).toEqual(new Date(2104, 1, 29, 9, 0));
  });
});
