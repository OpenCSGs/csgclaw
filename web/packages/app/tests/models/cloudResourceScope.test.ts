import { describe, expect, it } from "vitest";
import { cloudResourceScope, parseCloudResourceScope } from "@/models/cloudResourceScope";

describe("cloud resource scope", () => {
  it("separates the same remote agent across accounts and local agents", () => {
    const scope = cloudResourceScope("agt_1", "site:account/1");
    expect(parseCloudResourceScope(scope)).toEqual({ agentID: "agt_1", connectionID: "site:account/1" });
    expect(scope).not.toBe(cloudResourceScope("agt_1", "other-account"));
    expect(parseCloudResourceScope("agt_1")).toBeNull();
    expect(parseCloudResourceScope("cloud-resource:%invalid")).toBeNull();
    expect(parseCloudResourceScope(`cloud-resource:${encodeURIComponent('["agt_1",1]')}`)).toBeNull();
  });
});
