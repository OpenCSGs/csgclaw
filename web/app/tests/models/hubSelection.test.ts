import { resolveHubListSelection } from "@/models/hubSelection";

const resources = [{ id: "first" }, { id: "addressed" }];

describe("resolveHubListSelection", () => {
  it("keeps the resource list open when no item is addressed", () => {
    expect(resolveHubListSelection(resources, "", (item) => item.id)).toBeNull();
  });

  it("resolves an addressed resource", () => {
    expect(resolveHubListSelection(resources, "addressed", (item) => item.id)).toBe(resources[1]);
  });

  it("does not substitute the first item when the addressed resource is missing", () => {
    expect(resolveHubListSelection(resources, "missing", (item) => item.id)).toBeNull();
  });
});
