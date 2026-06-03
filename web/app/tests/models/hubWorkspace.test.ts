import {
  formatHubDate,
  formatHubDateTime,
  formatHubTemplateCount,
  isDeletableHubTemplate,
} from "@/models/hubWorkspace";

describe("hub workspace helpers", () => {
  it("formats hub dates in a stable UTC timezone", () => {
    expect(formatHubDate("", "en")).toBe("-");
    expect(formatHubDate("2026-05-15T12:34:56Z", "en")).toBe("05/15/2026");
    expect(formatHubDateTime("2026-05-15T12:34:56Z", "en")).toContain("12:34:56");
    expect(formatHubDateTime("2026-05-15T12:34:56Z", "en")).toContain("(UTC)");
  });

  it("allows deleting only local hub templates", () => {
    expect(isDeletableHubTemplate({ id: "local.gitlab-assistant", source: { kind: "local" } })).toBe(true);
    expect(isDeletableHubTemplate({ id: "builtin.picoclaw-worker", source: { kind: "builtin" } })).toBe(false);
    expect(isDeletableHubTemplate({ id: "official.review-bot", source: { kind: "remote" } })).toBe(false);
  });

  it("formats localized template counts", () => {
    const t = () => "templates";
    expect(formatHubTemplateCount(3, "en", t)).toBe("3 templates");
    expect(formatHubTemplateCount(3, "zh", t)).toBe("共 3 templates");
  });
});
