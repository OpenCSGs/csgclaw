import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentRuntimeSection } from "@/pages/ComputerPage/components";
import { ComputerDetailPane } from "@/pages/ComputerPage/components";
import type { AgentRuntime } from "@/models/agentRuntimes";
import type { TranslateFn } from "@/models/conversations";

const labels: Record<string, string> = {
  activeNow: "Active now",
  channelsSection: "Rooms",
  computerAgentsSection: "Agents",
  computerOverview: "Computer overview",
  computerRuntimesEmpty: "No agent runtimes are available yet.",
  computerRuntimesLoading: "Loading agent runtimes...",
  computerRuntimesRefreshing: "Updating status",
  computerRuntimesSubtitle: "Manage command-line runtimes.",
  computerRuntimesTitle: "Agent runtimes",
  computerRuntimeClaudeDescription: "Anthropic runtime",
  computerRuntimeCodexDescription: "OpenAI runtime",
  computerRuntimeDSHDescription: "DeepSeek Harness runtime",
  computerRuntimeComingSoon: "Coming soon",
  computerRuntimeComingSoonHint: "Installation support will arrive later.",
  computerRuntimeExecutable: "Executable",
  computerRuntimeVersion: "Version",
  computerRuntimeDocs: "Installation guide",
  computerRuntimeFailed: "Install failed",
  computerRuntimeInstalled: "Installed",
  computerRuntimeInstalling: "Installing...",
  computerRuntimeInstallingHint: "Downloading in the background.",
  computerRuntimeInstall: "Install",
  computerRuntimeInstallHint: "Install with one click.",
  computerRuntimeExternalInstallHint: "Install a compatible version using the guide, then retry detection.",
  computerRuntimeBundleMissingHint: "Codex CLI is missing from this CSGClaw bundle. Reinstall CSGClaw.",
  computerRuntimeNotInstalled: "Not installed",
  computerRuntimeReadyHint: "Ready for local agents.",
  computerRuntimeRetry: "Retry",
  computerRuntimeRetryHint: "Review the error and retry.",
  computerRuntimeUnsupported: "Unsupported platform",
  computerRuntimeUnsupportedHint: "No package is available for this platform.",
  createAgent: "Create",
  directMessagesSection: "Direct Messages",
  localComputer: "Local computer",
  noAgents: "No workers yet.",
  online: "online",
};

const t: TranslateFn = (key) => labels[key] ?? key;

const bundledCodex: AgentRuntime = {
  name: "codex",
  label: "Codex CLI",
  supported: true,
  installed: true,
  installable: false,
  status: "installed",
  path: "/opt/csgclaw/bin/codex",
  os: "darwin",
  arch: "arm64",
};

const claudeCode: AgentRuntime = {
  name: "claude_code",
  label: "Claude Code",
  supported: false,
  installed: false,
  installable: false,
  status: "coming_soon",
  os: "darwin",
  arch: "arm64",
};

const dsh: AgentRuntime = {
  name: "dsh",
  label: "DeepSeek Harness",
  supported: true,
  installed: true,
  installable: false,
  status: "installed",
  path: "/usr/local/bin/dsh",
  version: "0.1.6-alpha.2",
};

describe("AgentRuntimeSection", () => {
  it("shows bundled Codex and a clearly unavailable Claude Code card", () => {
    const { container } = render(<AgentRuntimeSection runtimes={[bundledCodex, claudeCode]} t={t} />);

    expect(screen.getByRole("heading", { name: "Agent runtimes" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Codex CLI" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Claude Code" })).toBeInTheDocument();
    expect(screen.getByText("Installed")).toBeInTheDocument();
    expect(screen.getByText("Coming soon")).toBeInTheDocument();

    const logos = [...container.querySelectorAll("img")];
    expect(logos.map((logo) => logo.getAttribute("src"))).toEqual([
      "model-providers/codex.svg",
      "model-providers/claude-code.svg",
    ]);

    expect(screen.queryByRole("button", { name: "Install" })).not.toBeInTheDocument();
  });

  it("shows the detected executable for installed Codex without an install action", () => {
    const path = "/Users/test/.csgclaw/bin/codex";
    render(
      <AgentRuntimeSection
        runtimes={[
          {
            ...bundledCodex,
            installed: true,
            status: "installed",
            path,
          },
          claudeCode,
        ]}
        t={t}
      />,
    );

    expect(screen.getByText("Installed")).toBeInTheDocument();
    expect(screen.getByText(path)).not.toHaveAttribute("title");
    expect(screen.queryByRole("button", { name: "Install" })).not.toBeInTheDocument();
  });

  it("shows the detected DSH version and links to installation guidance when missing", () => {
    const { rerender } = render(<AgentRuntimeSection runtimes={[dsh]} t={t} />);

    expect(screen.getByText("0.1.6-alpha.2")).toBeInTheDocument();
    expect(screen.getByText("/usr/local/bin/dsh")).toBeInTheDocument();

    rerender(
      <AgentRuntimeSection
        runtimes={[
          {
            ...dsh,
            installed: false,
            status: "not_installed",
            path: undefined,
            version: undefined,
            docsURL: "https://github.com/deepseek-ai/deepseek-harness",
            message: "DSH CLI is unavailable",
          },
        ]}
        t={t}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("DSH CLI is unavailable");
    expect(screen.getByText("Install a compatible version using the guide, then retry detection.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Installation guide" })).toHaveAttribute(
      "href",
      "https://github.com/deepseek-ai/deepseek-harness",
    );
  });

  it("explains when a required bundled Codex binary is missing", () => {
    render(
      <AgentRuntimeSection
        runtimes={[
          { ...bundledCodex, installed: false, status: "failed", message: "bundled Codex missing" },
          claudeCode,
        ]}
        t={t}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("bundled Codex missing");
    expect(screen.getByText("Codex CLI is missing from this CSGClaw bundle. Reinstall CSGClaw.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  });

  it("shows unsupported Codex as a terminal state without an install action", () => {
    render(
      <AgentRuntimeSection
        runtimes={[{ ...bundledCodex, installed: false, status: "unsupported" }, claudeCode]}
        t={t}
      />,
    );

    expect(screen.getByText("Unsupported platform")).toBeInTheDocument();
    expect(screen.getByText("No package is available for this platform.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Install" })).not.toBeInTheDocument();
  });

  it("shows accessible load and retry states", async () => {
    const user = userEvent.setup();
    const onRetryLoad = vi.fn();
    const { rerender } = render(<AgentRuntimeSection loading t={t} />);

    expect(screen.getByRole("status")).toHaveTextContent("Loading agent runtimes...");

    rerender(<AgentRuntimeSection error="runtime service unavailable" t={t} onRetryLoad={onRetryLoad} />);
    expect(screen.getByRole("alert")).toHaveTextContent("runtime service unavailable");
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetryLoad).toHaveBeenCalledTimes(1);
  });

  it("places agent runtimes between the computer overview and agent list", () => {
    const { container } = render(
      <ComputerDetailPane t={t} runtimeSectionProps={{ runtimes: [bundledCodex, claudeCode], t }} />,
    );

    const overview = container.querySelector(".computer-overview-card");
    const runtimeSection = screen.getByRole("region", { name: "Agent runtimes" });
    const agentPanel = container.querySelector(".computer-agent-panel");
    expect(overview).not.toBeNull();
    expect(agentPanel).not.toBeNull();
    if (!overview || !agentPanel) {
      throw new Error("Computer page sections are missing");
    }
    expect(overview.compareDocumentPosition(runtimeSection)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(runtimeSection.compareDocumentPosition(agentPanel)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });
});
