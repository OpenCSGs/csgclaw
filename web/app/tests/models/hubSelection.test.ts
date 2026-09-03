import { resolveHubListSelection } from "@/models/hubSelection";

const resources = [{ id: "first" }, { id: "addressed" }];

describe("resolveHubListSelection", () => {
  it("uses the first item only when no resource is addressed", () => {
    expect(resolveHubListSelection(resources, "", (item) => item.id)).toBe(resources[0]);
  });

  it("resolves an addressed resource", () => {
    expect(resolveHubListSelection(resources, "addressed", (item) => item.id)).toBe(resources[1]);
  });

  it("does not substitute the first item when the addressed resource is missing", () => {
    expect(resolveHubListSelection(resources, "missing", (item) => item.id)).toBeNull();
  });
});
