import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentCreateProgress, APIKeyField, CLIProxyAuthControl } from "@/components/business/ProfileControls";

const labels: Record<string, string> = {
  agentCreateProgressDone: "Done",
  agentCreateProgressFailed: "Failed",
  agentCreateProgressPreparing: "Preparing",
  authConnect: "Connect",
  authConnected: "connected",
  authConnecting: "Connecting",
  authMissing: "not connected",
  profileAPIKey: "API key",
  profileAPIKeyNewPlaceholder: "Enter API key",
  stepConfigure: "Configure",
  stepStart: "Start",
};

function t(key: string): string {
  return labels[key] ?? key;
}

describe("ProfileControls", () => {
  it("shows a stored API key mask until the user enters a replacement", async () => {
    function Harness() {
      const [value, setValue] = useState("");
      return (
        <APIKeyField
          profile={{ api_key_preview: "sk-test...", api_key_set: true }}
          t={t}
          value={value}
          onInput={(event) => setValue(event.currentTarget.value)}
        />
      );
    }

    const user = userEvent.setup();
    const { container } = render(<Harness />);

    expect(screen.getByLabelText("API key")).toHaveValue("");
    expect(screen.getByLabelText("API key")).not.toHaveAttribute("placeholder", "Enter API key");
    expect(container.querySelector(".api-key-mask")).toHaveTextContent("sk-test...");

    await user.type(screen.getByLabelText("API key"), "new-secret");

    expect(screen.getByLabelText("API key")).toHaveValue("new-secret");
    expect(container.querySelector(".api-key-mask")).toBeNull();
  });

  it("marks first-time API keys as required when requested", () => {
    const { container } = render(
      <APIKeyField profile={{ api_key_set: false }} required t={t} value="" onInput={() => {}} />,
    );

    expect(screen.getByRole("textbox", { name: /API key/ })).toHaveAttribute("placeholder", "Enter API key");
    expect(screen.getByRole("textbox", { name: /API key/ })).toBeRequired();
    expect(container.querySelector(".field-required-star")).toHaveTextContent("*");
  });

  it("normalizes CLI proxy auth providers before starting login", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();

    const { rerender } = render(
      <CLIProxyAuthControl provider="claude-code" status={{ authenticated: false }} t={t} onLogin={onLogin} />,
    );

    await user.click(screen.getByRole("button", { name: "Connect Claude Code" }));
    expect(onLogin).toHaveBeenCalledWith("claude_code");

    rerender(<CLIProxyAuthControl provider="claude-code" status={{ authenticated: true }} t={t} onLogin={onLogin} />);

    expect(screen.getByText("Claude Code connected")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Connect Claude Code/ })).not.toBeInTheDocument();
  });

  it("does not render CLI proxy auth controls for providers that do not need browser login", () => {
    const { container } = render(<CLIProxyAuthControl provider="api" t={t} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("renders agent creation progress status, clamped percent, and step state", () => {
    render(
      <AgentCreateProgress
        t={t}
        progress={{
          index: 1,
          percent: 145,
          startedAt: 1,
          status: "running",
          steps: [
            { label: "stepConfigure", target: 16 },
            { label: "stepStart", target: 88 },
          ],
        }}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Start");
    expect(screen.getByRole("status")).toHaveTextContent("100%");
    expect(screen.getByText("Configure")).toHaveClass("complete");
    expect(screen.getAllByText("Start").at(-1)).toHaveClass("active");
  });
});
