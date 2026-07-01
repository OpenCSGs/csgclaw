import { describe, expect, it } from "vitest";
import { isReadonlySkill, skillSourceBadgeName } from "@/models/skillhub";

describe("skill hub helpers", () => {
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
});
