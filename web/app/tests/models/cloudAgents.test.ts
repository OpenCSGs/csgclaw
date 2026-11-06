import { describe, expect, it } from "vitest";
import {
  cloudAgentAvatar,
  filterCloudAgents,
  parseCloudAgents,
  sameCloudAgentReference,
  type CloudAgent,
} from "@/models/cloudAgents";
import { normalizeAvatarPath } from "@/shared/avatar";
import { agentPlazaSearch, cloudAgentSearch } from "@/models/routing";
const agent: CloudAgent = {
  id: "agt_contract",
  relayURL: "http://relay.test",
  connectionID: "test-connection",
  name: "Contract reviewer",
  description: "Review clauses",
  owner: "OpenCSG",
  publisher: "Relay",
  sourceAgentID: "local_1",
  visibility: "public",
  status: "online",
  lastSeenAt: null,
  avatar: "",
};
describe("cloud agent type avatars", () => {
  it.each([
    ["openclaw", "openclaw"],
    ["langflow", "langflow"],
    ["csgclaw", "csgclaw"],
    ["csgclaw-workspace", "csgclaw"],
    ["code", "code"],
    [" OpenClaw ", "openclaw"],
  ])("matches %s to its original icon", (type, icon) => {
    const path = cloudAgentAvatar(type, "avatar/pic-2.png");
    expect(path).toBe(`avatar/cloud-${icon}.png`);
    expect(normalizeAvatarPath(path)).toBe(path);
  });
  it("keeps an existing avatar for an unknown type and supplies an icon for legacy empty avatars", () => {
    expect(cloudAgentAvatar("unknown", "avatar/pic-2.png")).toBe("avatar/pic-2.png");
    expect(cloudAgentAvatar(undefined, "")).toBe("avatar/cloud-csgclaw.png");
  });
  it("persists type metadata while remaining compatible with legacy saved entries", () => {
    expect(parseCloudAgents(JSON.stringify([{ ...agent, type: "code" }]))[0]).toMatchObject({ type: "code" });
    expect(parseCloudAgents(JSON.stringify([agent]))[0]).not.toHaveProperty("type");
  });
});
describe("cloud agent membership", () => {
  it.each([null, "invalid", "{}", '[1,null,"unknown"]'])("rejects malformed or unknown saved IDs: %s", (raw) => {
    expect(parseCloudAgents(raw)).toEqual([]);
  });
  it("preserves invocation references without trusting cached availability or extra secrets", () => {
    const saved = parseCloudAgents(
      JSON.stringify([agent, agent, { ...agent, id: "agt_other", token: "must-not-persist" }]),
    );
    expect(saved).toHaveLength(2);
    expect(saved[0]).toEqual({ ...agent, status: "offline" });
    expect(JSON.stringify(saved)).not.toContain("must-not-persist");
  });
  it("does not reuse a remote ID under another connection identity", () => {
    expect(sameCloudAgentReference(agent, { ...agent })).toBe(true);
    expect(sameCloudAgentReference(agent, { ...agent, connectionID: "another-user" })).toBe(false);
    expect(sameCloudAgentReference(agent, { ...agent, relayURL: "http://other-relay.test" })).toBe(false);
  });
  it("preserves separate memberships when two accounts share a relay ID", () => {
    const other = { ...agent, connectionID: "another-user" };
    expect(parseCloudAgents(JSON.stringify([agent, other, agent]))).toEqual([
      { ...agent, status: "offline" },
      { ...other, status: "offline" },
    ]);
  });
  it("searches names, descriptions, owners and relay IDs without case sensitivity", () => {
    for (const query of [" contract ", "CLAUSES", "opencsg", "agt_contract"])
      expect(filterCloudAgents([agent], query)).toEqual([agent]);
    expect(filterCloudAgents([agent], "missing")).toEqual([]);
  });
  it("encodes cloud selection separately from local agent paths", () => {
    expect(agentPlazaSearch()).toBe("?tab=plaza");
    expect(cloudAgentSearch("agt/a?b")).toBe("?cloud=agt%2Fa%3Fb");
  });
});
