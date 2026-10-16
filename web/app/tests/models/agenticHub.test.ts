import { agenticHubCreateAgentURL } from "@/models/agenticHub";

describe("agenticHubCreateAgentURL", () => {
  it.each(["https://opencsg.com", "https://opencsg-stg.com", "https://custom.example:8443/login"])(
    "uses the origin of %s",
    (site) => {
      const url = new URL(agenticHubCreateAgentURL(site, "alice/template-name&test"));
      expect(url.origin).toBe(new URL(site).origin);
      expect(url.pathname).toBe("/agentichub");
      expect(url.searchParams.get("action")).toBe("create-agent");
      expect(url.searchParams.get("template_name")).toBe("alice/template-name&test");
    },
  );
  it.each(["", "bad url", "javascript:alert(1)"])("rejects invalid site %s", (site) => {
    expect(agenticHubCreateAgentURL(site, "template")).toBe("");
  });
  it("encodes the namespace separator once and decodes it through URLSearchParams", () => {
    const url = agenticHubCreateAgentURL("https://qa.opencsg.com", "wanghj/agent-jvgyld-my99tf14");
    expect(url).toBe(
      "https://qa.opencsg.com/agentichub?action=create-agent&template_name=wanghj%2Fagent-jvgyld-my99tf14",
    );
    expect(new URL(url).searchParams.get("template_name")).toBe("wanghj/agent-jvgyld-my99tf14");
    expect(url).not.toContain("%252F");
  });
  it("requires a template ID", () => {
    expect(agenticHubCreateAgentURL("https://opencsg.com", "")).toBe("");
  });
});
