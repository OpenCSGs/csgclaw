import { describe, expect, it } from "vitest";
import { filterCloudAgents, parseCloudAgents, sameCloudAgentReference, type CloudAgent } from "@/models/cloudAgents";
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
