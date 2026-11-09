import { describe, expect, it } from "vitest";
import { localizeAgentLogTimestamps } from "@/models/agentLogs";

const timestamp = "2026-10-19T03:36:00.123456Z";
const localSeconds = new Intl.DateTimeFormat("sv-SE", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
})
  .format(new Date(timestamp))
  .replace(" ", "T");

describe("Agent log timestamps", () => {
  it.each([
    `${timestamp} ERROR request failed`,
    `time=${timestamp} level=ERROR msg="request failed"`,
    `time="${timestamp}" level=ERROR`,
    `{"timestamp":"${timestamp}","level":"ERROR","fields":{"message":"request failed"}}`,
    `{"time": "${timestamp}", "message": "request failed"}`,
  ])("uses the computer's local time and preserves submillisecond precision: %s", (input) => {
    const result = localizeAgentLogTimestamps(input);
    expect(result).toContain(`${localSeconds}.123456`);
    const localized = result.match(/\d{4}-\d{2}-\d{2}T[^"\s]+/)?.[0];
    expect(localized).toMatch(/[+-]\d{2}:\d{2}$/);
    expect(new Date(localized!).getTime()).toBe(new Date(timestamp).getTime());
    expect(localizeAgentLogTimestamps(result)).toBe(result);
  });

  it("converts explicit offsets across dates without changing log messages or line endings", () => {
    const line = 'time=2026-10-19T00:30:00+09:00 level=INFO msg="2026-10-19T00:30:00Z"\r\n';
    const result = localizeAgentLogTimestamps(line);
    expect(new Date(result.match(/time=([^ ]+)/)![1]).toISOString()).toBe("2026-10-18T15:30:00.000Z");
    expect(result).toContain('msg="2026-10-19T00:30:00Z"\r\n');
  });

  it.each([
    "",
    "plain runtime output",
    "time=2026-10-19T03:36:00 no-zone",
    "time=2026-99-19T03:36:00Z invalid",
    'message="2026-10-19T03:36:00Z"',
    '{"message":"time=2026-10-19T03:36:00Z"}',
  ])("preserves content without an unambiguous logger timestamp: %s", (input) => {
    expect(localizeAgentLogTimestamps(input)).toBe(input);
  });
});
