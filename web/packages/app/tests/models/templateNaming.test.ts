import { defaultTemplateName, isValidTemplateName } from "@/models/templateNaming";

describe("template naming", () => {
  it("appends a compact millisecond timestamp to the agent ID", () => {
    expect(defaultTemplateName("u-worker", 1760000000000)).toBe(`u-worker-${(1760000000000).toString(36)}`);
    expect(defaultTemplateName("u-worker", 1760000000001)).not.toBe(defaultTemplateName("u-worker", 1760000000000));
  });

  it.each(["a".repeat(80), "123-worker", "中文助手", "", "worker / test"])("generates a valid name from %s", (id) => {
    const name = defaultTemplateName(id, 1760000000000);
    expect(name.length).toBeLessThanOrEqual(32);
    expect(isValidTemplateName(name)).toBe(true);
    expect(name.endsWith(`-${(1760000000000).toString(36)}`)).toBe(true);
  });

  it("accepts 32 characters and rejects 33 characters", () => {
    expect(isValidTemplateName("a".repeat(32))).toBe(true);
    expect(isValidTemplateName("a".repeat(33))).toBe(false);
  });
});
