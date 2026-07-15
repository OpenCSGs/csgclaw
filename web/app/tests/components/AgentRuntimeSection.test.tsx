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
  computerRuntimeComingSoon: "Coming soon",
  computerRuntimeComingSoonHint: "Installation support will arrive later.",
  computerRuntimeExecutable: "Executable",
  computerRuntimeFailed: "Install failed",
  computerRuntimeInstalled: "Installed",
  computerRuntimeInstalling: "Installing...",
  computerRuntimeInstallingHint: "Downloading in the background.",
  computerRuntimeInstall: "Install",
  computerRuntimeInstallHint: "Install with one click.",
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

const missingCodex: AgentRuntime = {
  name: "codex",
  label: "Codex CLI",
  supported: true,
  installed: false,
  installable: true,
  status: "not_installed",
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

describe("AgentRuntimeSection", () => {
  it("shows installable Codex and a clearly unavailable Claude Code card", async () => {
    const user = userEvent.setup();
    const onInstall = vi.fn();
    const { container } = render(
      <AgentRuntimeSection runtimes={[missingCodex, claudeCode]} t={t} onInstall={onInstall} />,
    );

    expect(screen.getByRole("heading", { name: "Agent runtimes" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Codex CLI" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Claude Code" })).toBeInTheDocument();
    expect(screen.getByText("Not installed")).toBeInTheDocument();
    expect(screen.getByText("Coming soon")).toBeInTheDocument();

    const logos = [...container.querySelectorAll("img")];
    expect(logos.map((logo) => logo.getAttribute("src"))).toEqual([
      "model-providers/codex.svg",
      "model-providers/claude-code.svg",
    ]);

    await user.click(screen.getByRole("button", { name: "Install" }));
    expect(onInstall).toHaveBeenCalledWith("codex");
    expect(screen.queryByRole("button", { name: /Claude/ })).not.toBeInTheDocument();
  });

  it("presents background installation as a disabled busy action", () => {
    render(
      <AgentRuntimeSection
        runtimes={[{ ...missingCodex, status: "installing" }, claudeCode]}
        busyRuntimeName="codex"
        t={t}
      />,
    );

    const installing = screen.getByRole("button", { name: "Installing..." });
    expect(installing).toBeDisabled();
    expect(installing).toHaveAttribute("aria-busy", "true");
    expect(screen.getAllByText("Installing...").length).toBeGreaterThan(0);
  });

  it("shows the detected executable for installed Codex without an install action", () => {
    const path = "/Users/test/.csgclaw/bin/codex";
    render(
      <AgentRuntimeSection
        runtimes={[
          {
            ...missingCodex,
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

  it("keeps a failed install message next to a manual retry", async () => {
    const user = userEvent.setup();
    const onInstall = vi.fn();
    render(
      <AgentRuntimeSection
        runtimes={[{ ...missingCodex, status: "failed", message: "archive checksum mismatch" }, claudeCode]}
        t={t}
        onInstall={onInstall}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("archive checksum mismatch");
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onInstall).toHaveBeenCalledWith("codex");
  });

  it("shows unsupported Codex as a terminal state without an install action", () => {
    render(
      <AgentRuntimeSection
        runtimes={[{ ...missingCodex, status: "unsupported", installable: true }, claudeCode]}
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
      <ComputerDetailPane t={t} runtimeSectionProps={{ runtimes: [missingCodex, claudeCode], t }} />,
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
