import { GUIDE_ACKNOWLEDGED_KEY, GUIDE_REVISION, hasAcknowledgedGuide } from "./onboardingGuidePersistence";

describe("onboarding guide revision persistence", () => {
  beforeEach(() => localStorage.clear());

  it("shows unacknowledged content and keeps acknowledged content dismissed", () => {
    expect(hasAcknowledgedGuide(localStorage)).toBe(false);
    localStorage.setItem(GUIDE_ACKNOWLEDGED_KEY, GUIDE_REVISION);
    expect(hasAcknowledgedGuide(localStorage)).toBe(true);
    expect(hasAcknowledgedGuide(localStorage, "2")).toBe(false);
    localStorage.setItem(GUIDE_ACKNOWLEDGED_KEY, "2");
    expect(hasAcknowledgedGuide(localStorage, "2")).toBe(true);
  });

  it.each(["v0.3.0", "dev"])("migrates a legacy %s record without replaying unchanged content", (version) => {
    localStorage.setItem(`csgclaw:onboarding-guide:seen:${version}`, "seen");
    expect(hasAcknowledgedGuide(localStorage)).toBe(true);
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBe(GUIDE_REVISION);
    expect(localStorage.getItem(`csgclaw:onboarding-guide:seen:${version}`)).toBe("seen");
  });

  it("does not migrate legacy records to a new content revision", () => {
    localStorage.setItem("csgclaw:onboarding-guide:seen:v0.3.0", "seen");
    expect(hasAcknowledgedGuide(localStorage, "2")).toBe(false);
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBeNull();
    localStorage.setItem(GUIDE_ACKNOWLEDGED_KEY, "1");
    expect(hasAcknowledgedGuide(localStorage, "2")).toBe(false);
    localStorage.setItem(GUIDE_ACKNOWLEDGED_KEY, "2");
    expect(hasAcknowledgedGuide(localStorage, "2")).toBe(true);
  });

  it("ignores unrelated records and invalid legacy values", () => {
    localStorage.setItem("other-feature", "seen");
    localStorage.setItem("csgclaw:onboarding-guide:seen:v0.3.0", "false");
    expect(hasAcknowledgedGuide(localStorage)).toBe(false);
  });

  it("honors a legacy acknowledgment even if writing the migration fails", () => {
    localStorage.setItem("csgclaw:onboarding-guide:seen:v0.3.0", "seen");
    const save = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("Storage unavailable");
    });
    try {
      expect(hasAcknowledgedGuide(localStorage)).toBe(true);
    } finally {
      save.mockRestore();
    }
  });
});
