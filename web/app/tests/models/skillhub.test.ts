import { describe, expect, it } from "vitest";
import { isReadonlySkill, remoteSkillInstallName, skillOriginKind, skillSourceBadgeName } from "@/models/skillhub";

describe("skill hub helpers", () => {
  it.each([
    ["market", "market"],
    ["official", "market"],
    ["local", "local"],
    ["system", "builtin"],
    ["unknown", "unknown"],
    [undefined, "unknown"],
  ])("preserves installation source %s without guessing from the skill name", (source, expected) => {
    expect(skillOriginKind({ name: "market-skill", source })).toBe(expected);
  });

  it("treats official and system skills as readonly", () => {
    expect(isReadonlySkill({ name: "remote", source: "official" })).toBe(true);
    expect(isReadonlySkill({ name: "mine", source: "personal" })).toBe(true);
    expect(isReadonlySkill({ name: "system", source: "system" })).toBe(true);
    expect(isReadonlySkill({ name: "builtin", readonly: true })).toBe(true);
    expect(isReadonlySkill({ name: "local" })).toBe(false);
  });

  it("uses source badges for builtin, official, personal, and local skills", () => {
    expect(skillSourceBadgeName({ name: "remote", source: "official" })).toBe("official");
    expect(skillSourceBadgeName({ name: "mine", source: "personal" })).toBe("personal");
    expect(skillSourceBadgeName({ name: "builtin", source: "builtin" })).toBe("builtin");
    expect(skillSourceBadgeName({ name: "system", source: "system" })).toBe("builtin");
    expect(skillSourceBadgeName({ name: "readonly", readonly: true })).toBe("builtin");
    expect(skillSourceBadgeName({ name: "local" })).toBe("local");
    expect(skillSourceBadgeName(null)).toBe("");
  });

  it("uses the remote path basename as the installed remote skill name", () => {
    expect(remoteSkillInstallName({ name: "Display Name", remotePath: "AIWizards/agent-builder" })).toBe(
      "agent-builder",
    );
    expect(remoteSkillInstallName({ name: "local-name" })).toBe("local-name");
    expect(remoteSkillInstallName(null)).toBe("");
  });
});
