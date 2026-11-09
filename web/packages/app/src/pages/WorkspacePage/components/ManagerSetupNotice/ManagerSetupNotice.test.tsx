import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentLike } from "@/models/agents";
import { createTranslator } from "@/shared/i18n";
import { MANAGER_SETUP_DISMISSED_STORAGE_KEY } from "@/shared/storage/keys";
import { ManagerSetupNotice } from "./ManagerSetupNotice";

const manager: AgentLike = {
  id: "agent-manager",
  name: "manager",
  role: "manager",
  runtime: { name: "codex", state: "profile_incomplete" },
  model_config: { model_provider_id: "csghub-lite" },
};
const configured: AgentLike = {
  ...manager,
  runtime: { name: "codex", state: "stopped" },
  model_config: { model_provider_id: "opencsg", model_id: "auto" },
};
const t = createTranslator("en");

beforeEach(() => localStorage.clear());

describe("ManagerSetupNotice", () => {
  it.each(["en", "zh"] as const)("offers optional setup in %s without moving focus or blocking a worker", (locale) => {
    const translate = createTranslator(locale);
    const configure = vi.fn();
    const run = vi.fn();
    render(
      <>
        <input aria-label="Worker prompt" autoFocus />
        <button onClick={run}>Run worker</button>
        <ManagerSetupNotice manager={manager} onConfigure={configure} t={translate} />
      </>,
    );
    expect(screen.getByRole("region", { name: translate("managerSetupTitle") })).toBeInTheDocument();
    expect(screen.getByLabelText("Worker prompt")).toHaveFocus();
    expect(configure).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Run worker" }));
    expect(run).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: translate("managerSetupAction") }));
    expect(configure).toHaveBeenCalledWith(manager);
  });

  it("remembers Maybe later across remounts", () => {
    const first = render(<ManagerSetupNotice manager={manager} onConfigure={vi.fn()} t={t} />);
    fireEvent.click(screen.getByRole("button", { name: "Maybe later" }));
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    expect(localStorage.getItem(MANAGER_SETUP_DISMISSED_STORAGE_KEY)).toBe("true");
    first.unmount();
    render(<ManagerSetupNotice manager={manager} onConfigure={vi.fn()} t={t} />);
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });

  it("waits for a manager record and disappears when a model is configured", () => {
    const configure = vi.fn();
    const view = render(<ManagerSetupNotice manager={undefined} onConfigure={configure} t={t} />);
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    view.rerender(<ManagerSetupNotice manager={manager} onConfigure={configure} t={t} />);
    expect(screen.getByRole("region")).toBeInTheDocument();
    view.rerender(<ManagerSetupNotice manager={configured} onConfigure={configure} t={t} />);
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });

  it.each(["failed", "unknown", "stopped"])("still offers setup when an unconfigured runtime reports %s", (state) => {
    render(
      <ManagerSetupNotice manager={{ ...manager, runtime: { name: "codex", state } }} onConfigure={vi.fn()} t={t} />,
    );
    expect(screen.getByRole("region")).toBeInTheDocument();
  });

  it("does not treat a configured offline manager as unfinished setup", () => {
    render(<ManagerSetupNotice manager={configured} onConfigure={vi.fn()} t={t} />);
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });

  it("can be skipped when browser storage is unavailable", () => {
    const read = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("Storage unavailable");
    });
    const write = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("Storage unavailable");
    });
    try {
      render(<ManagerSetupNotice manager={manager} onConfigure={vi.fn()} t={t} />);
      fireEvent.click(screen.getByRole("button", { name: "Maybe later" }));
      expect(screen.queryByRole("region")).not.toBeInTheDocument();
    } finally {
      read.mockRestore();
      write.mockRestore();
    }
  });
});
