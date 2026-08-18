import {
  canPublishHubTemplateToCommunity,
  formatHubDate,
  formatHubDateTime,
  formatHubTemplateCount,
  isDeletableHubTemplate,
  isHubTemplateAccountEmailMissing,
  isHubTemplateNameConflict,
  isVisibleInHubTemplateList,
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

  it("only allows local Codex templates to publish to the community", () => {
    expect(
      canPublishHubTemplateToCommunity({
        id: "local.codex",
        runtime_kind: "codex",
        source: { kind: "local" },
      }),
    ).toBe(true);
    expect(
      canPublishHubTemplateToCommunity({
        id: "local.openclaw",
        runtime_kind: "openclaw_sandbox",
        source: { kind: "local" },
      }),
    ).toBe(false);
  });

  it("recognizes duplicate community template errors", () => {
    expect(
      isHubTemplateNameConflict({
        status: 502,
        message:
          'remote hub request failed with status 500: {"code":"SPACE_ERR_1","msg":"SPACE_ERR_1: The space name already exists."}',
      }),
    ).toBe(true);
    expect(isHubTemplateNameConflict(new Error("The space name already exists."))).toBe(true);
    expect(isHubTemplateNameConflict(new Error("network unavailable"))).toBe(false);
  });

  it("recognizes a missing OpenCSG account email during community publishing", () => {
    expect(
      isHubTemplateAccountEmailMissing({
        status: 502,
        message:
          'remote hub request failed with status 500: {"code":"USER-ERR-18","msg":"USER-ERR-18: User email is empty"}',
      }),
    ).toBe(true);
    expect(isHubTemplateAccountEmailMissing({ code: "USER-ERR-18", message: "publish failed" })).toBe(true);
    expect(isHubTemplateAccountEmailMissing(new Error("network unavailable"))).toBe(false);
  });

  it("shows worker templates and official remote templates in hub lists", () => {
    expect(
      isVisibleInHubTemplateList({ id: "builtin.picoclaw-worker", role: "worker", source: { kind: "builtin" } }),
    ).toBe(true);
    expect(
      isVisibleInHubTemplateList({ id: "builtin.manager-codex", role: "manager", source: { kind: "builtin" } }),
    ).toBe(false);
    expect(
      isVisibleInHubTemplateList({
        id: "official.review-bot",
        role: "manager",
        source: { kind: "remote", name: "official" },
      }),
    ).toBe(true);
    expect(
      isVisibleInHubTemplateList({ id: "team.review-bot", role: "worker", source: { kind: "remote", name: "team" } }),
    ).toBe(false);
  });

  it("formats localized template counts", () => {
    const t = () => "templates";
    expect(formatHubTemplateCount(3, "en", t)).toBe("3 templates");
    expect(formatHubTemplateCount(3, "zh", t)).toBe("共 3 templates");
  });
});
