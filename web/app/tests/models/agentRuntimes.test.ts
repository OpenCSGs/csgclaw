import {
  AgentRuntimeStatuses,
  agentRuntimeByName,
  normalizeAgentRuntime,
  normalizeAgentRuntimeList,
  shouldPollAgentRuntimeInstallation,
  upsertAgentRuntime,
} from "@/models/agentRuntimes";

describe("agent runtimes", () => {
  it("normalizes the API list and keeps Codex before Claude Code", () => {
    const runtimes = normalizeAgentRuntimeList([
      {
        name: "claude-code",
        label: "Claude Code",
        supported: false,
        installed: false,
        installable: false,
        status: "coming_soon",
        docs_url: "https://example.com/claude",
      },
      { name: "", label: "Invalid" },
      {
        name: "codex",
        label: "Codex CLI",
        supported: true,
        installed: true,
        installable: true,
        status: "installed",
        path: "/tmp/codex",
        os: "darwin",
        arch: "arm64",
      },
    ]);

    expect(runtimes.map((runtime) => runtime.name)).toEqual(["codex", "claude_code"]);
    expect(runtimes[0]).toMatchObject({
      installed: true,
      path: "/tmp/codex",
      status: AgentRuntimeStatuses.installed,
    });
    expect(runtimes[1]).toMatchObject({
      docsURL: "https://example.com/claude",
      status: AgentRuntimeStatuses.comingSoon,
    });
  });

  it("derives safe fallback states for incomplete records", () => {
    expect(normalizeAgentRuntime({ name: "codex", status: "unexpected" })).toMatchObject({
      label: "Codex CLI",
      status: AgentRuntimeStatuses.notInstalled,
      supported: true,
    });
    expect(normalizeAgentRuntime({ name: "claude_code", status: "unexpected" })).toMatchObject({
      label: "Claude Code",
      status: AgentRuntimeStatuses.comingSoon,
      supported: false,
    });
    expect(normalizeAgentRuntime({ name: "codex", status: "unsupported", installable: true })).toMatchObject({
      status: AgentRuntimeStatuses.unsupported,
      installed: false,
    });
    expect(normalizeAgentRuntime(null)).toBeNull();
  });

  it("upserts install results without disturbing runtime order", () => {
    const initial = normalizeAgentRuntimeList([
      { name: "codex", status: "not_installed", installed: false, installable: true, supported: true },
      { name: "claude_code", status: "coming_soon", installed: false, supported: false },
    ]);

    const updated = upsertAgentRuntime(initial, {
      name: "codex",
      status: "installed",
      installed: true,
      installable: true,
      supported: true,
      path: "/opt/csgclaw/codex",
    });

    expect(updated.map((runtime) => runtime.name)).toEqual(["codex", "claude_code"]);
    expect(agentRuntimeByName(updated, "codex")).toMatchObject({
      installed: true,
      path: "/opt/csgclaw/codex",
    });
  });

  it("polls during the serve-time install race and stops at terminal states", () => {
    const runtime = (status: string) =>
      normalizeAgentRuntimeList([
        { name: "codex", status, installed: status === "installed", supported: true, installable: true },
      ]);

    expect(shouldPollAgentRuntimeInstallation(runtime("not_installed"))).toBe(true);
    expect(shouldPollAgentRuntimeInstallation(runtime("installing"))).toBe(true);
    expect(shouldPollAgentRuntimeInstallation(runtime("installed"))).toBe(false);
    expect(shouldPollAgentRuntimeInstallation(runtime("failed"))).toBe(false);
    expect(shouldPollAgentRuntimeInstallation(runtime("unsupported"))).toBe(false);
  });
});
