import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TaskConfigurationToolbar } from "@/components/business/TaskConfigurationToolbar";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import type { WorkspaceController } from "@/hooks/workspace";
import type { TaskConfiguration } from "@/models/taskConfiguration";
import { pickLocalDirectoryPath } from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";

vi.mock("@/components/business/ProfileControls/runtimeOptionDirectoryPicker", () => ({
  pickLocalDirectoryPath: vi.fn(),
}));

function setup(configuration: TaskConfiguration = {}, onChange = vi.fn(), directoryLocked = false) {
  const controller = {
    ready: true,
    t: (key: string) => key,
    sidebarProps: { locale: "en", onCreateModelProvider: vi.fn() },
    modelProviders: {
      providers: [
        {
          id: "codex",
          display_name: "Codex",
          models: ["gpt-test", "image-test"],
          imageModels: ["image-test"],
          status: "connected",
        },
      ],
    },
    openAgentSettings: vi.fn(),
  } as unknown as WorkspaceController;
  function Toolbar() {
    const [current, setCurrent] = useState(configuration);
    return (
      <TaskConfigurationToolbar
        agent={{ id: "agent-test", name: "Test", runtime_kind: "codex", model_config: { model_id: "default-model" } }}
        configuration={current}
        directoryLocked={directoryLocked}
        onChange={async (next) => {
          await onChange(next);
          setCurrent(next);
        }}
      />
    );
  }
  render(
    <WorkspaceControllerProvider controller={controller}>
      <Toolbar />
    </WorkspaceControllerProvider>,
  );
  return { onChange, controller };
}

describe("task configuration", () => {
  beforeEach(() => localStorage.removeItem("csgclaw.recent-work-directories"));
  it("does not focus the directory search when opening the picker", () => {
    setup();
    const trigger = screen.getByRole("button", { name: "iaWorkdirTitle" });
    trigger.focus();
    fireEvent.click(trigger);
    const search = screen.getByRole("textbox", { name: "taskDirectorySearch" });
    expect(search).not.toHaveFocus();
    search.focus();
    expect(search).toHaveFocus();
  });
  it("opens the directory picker and applies the selected directory", async () => {
    vi.mocked(pickLocalDirectoryPath).mockResolvedValueOnce("/new-project");
    const { onChange } = setup({ workspace_dir: "/project" });
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirTitle" }));
    fireEvent.click(screen.getByRole("button", { name: "taskDirectoryAdd" }));
    expect(pickLocalDirectoryPath).toHaveBeenCalledOnce();
    await waitFor(() => expect(onChange).toHaveBeenCalledWith({ workspace_dir: "/new-project" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "taskDirectoryAdd" })).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirTitle" }));
    expect(screen.getByRole("button", { name: /new-project.*\/new-project/ })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "taskConfigurationDirectoryPath" })).not.toBeInTheDocument();
  });
  it("does not change the directory when selection is canceled", async () => {
    vi.mocked(pickLocalDirectoryPath).mockResolvedValueOnce(null);
    const { onChange } = setup({ workspace_dir: "/project" });
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirTitle" }));
    fireEvent.click(screen.getByRole("button", { name: "taskDirectoryAdd" }));
    await waitFor(() => expect(pickLocalDirectoryPath).toHaveBeenCalled());
    expect(onChange).not.toHaveBeenCalled();
  });
  it("filters previously added directories", () => {
    localStorage.setItem("csgclaw.recent-work-directories", JSON.stringify(["/work/notes", "/work/demo"]));
    setup();
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirTitle" }));
    fireEvent.change(screen.getByRole("textbox", { name: "taskDirectorySearch" }), { target: { value: "notes" } });
    expect(screen.getByRole("button", { name: /notes.*\/work\/notes/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /demo.*\/work\/demo/ })).not.toBeInTheDocument();
  });
  it("applies a model to this session and keeps the working directory", async () => {
    const { onChange, controller } = setup({ workspace_dir: "/project" });
    fireEvent.click(screen.getByRole("combobox", { name: "iaModelTitle" }));
    expect(screen.queryByRole("button", { name: "image-test" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("option", { name: /gpt-test/ }));
    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith({
        workspace_dir: "/project",
        model_provider_id: "codex",
        model_id: "gpt-test",
        reasoning_effort: "auto",
      }),
    );
    expect(controller.openAgentSettings).not.toHaveBeenCalled();
  });
  it("applies reasoning and reports failed saves without hiding the menu", async () => {
    const save = vi.fn().mockRejectedValue(new Error("busy conversation"));
    setup({ model_id: "gpt-test", model_provider_id: "codex", reasoning_effort: "medium" }, save);
    fireEvent.click(screen.getByRole("button", { name: "iaReasoningTitle" }));
    fireEvent.keyDown(screen.getByRole("slider", { name: "iaReasoningTitle" }), { key: "ArrowRight" });
    expect(await screen.findByRole("alert")).toHaveTextContent("busy conversation");
    expect(save).toHaveBeenCalledWith({ model_id: "gpt-test", model_provider_id: "codex", reasoning_effort: "high" });
    await waitFor(() => expect(screen.getByRole("slider")).toHaveAttribute("aria-valuetext", "profileReasoningMedium"));
  });
  it("saves slider steps with the keyboard, keeps the panel open and can restore the agent default", async () => {
    const { onChange } = setup({ workspace_dir: "/project", reasoning_effort: "low" });
    fireEvent.click(screen.getByRole("button", { name: "iaReasoningTitle" }));
    const slider = screen.getByRole("slider", { name: "iaReasoningTitle" });
    slider.focus();
    expect(slider).toHaveAttribute("aria-valuetext", "profileReasoningLow");
    fireEvent.keyDown(slider, { key: "End" });
    await waitFor(() => expect(slider).toHaveAttribute("aria-valuetext", "profileReasoningXHigh"));
    expect(slider).toHaveFocus();
    expect(onChange).toHaveBeenLastCalledWith({ workspace_dir: "/project", reasoning_effort: "xhigh" });
    fireEvent.keyDown(slider, { key: "Home" });
    await waitFor(() => expect(slider).toHaveAttribute("aria-valuetext", "profileReasoningDisabled"));
    expect(onChange).toHaveBeenLastCalledWith({ workspace_dir: "/project", reasoning_effort: "none" });
    fireEvent.click(screen.getByRole("button", { name: "iaReasoningFollowAgent" }));
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith({ workspace_dir: "/project", reasoning_effort: "" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "iaReasoningFollowAgent" })).toBeDisabled());
    expect(screen.getByText("iaReasoningFollowing")).toBeInTheDocument();
    expect(screen.getByRole("slider")).toHaveAttribute("aria-valuetext", "profileReasoningAuto");
  });
  it("keeps model default separate from following the agent", async () => {
    const { onChange } = setup({ reasoning_effort: "high" });
    fireEvent.click(screen.getByRole("button", { name: "iaReasoningTitle" }));
    fireEvent.click(screen.getByRole("button", { name: "profileReasoningAuto" }));
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith({ reasoning_effort: "auto" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "profileReasoningAuto" })).toHaveAttribute("aria-pressed", "true"),
    );
    expect(screen.getByRole("button", { name: "iaReasoningFollowAgent" })).toBeEnabled();
    const slider = screen.getByRole("slider");
    expect(slider).toHaveAttribute("aria-valuetext", "profileReasoningAuto");
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith({ reasoning_effort: "high" }));
    expect(screen.queryByRole("button", { name: "iaReasoningCustom" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "iaReasoningInfo" })).toBeInTheDocument();
  });
  it("opens custom model provider setup without connection actions in the picker", async () => {
    const { controller } = setup();
    fireEvent.click(screen.getByRole("combobox", { name: "iaModelTitle" }));
    expect(screen.getByRole("combobox", { name: "modelProviderModelSearch" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "modelConnectAdd" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "modelConnectRetry" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "iaModelConfigure" }));

    await waitFor(() =>
      expect(controller.sidebarProps?.onCreateModelProvider).toHaveBeenCalledWith(
        expect.objectContaining({ preset: "custom", onConnected: expect.any(Function) }),
      ),
    );
    expect(screen.queryByRole("combobox", { name: "modelProviderModelSearch" })).not.toBeInTheDocument();
  });
  it("locks the directory in an existing conversation", () => {
    setup({ workspace_dir: "/project" }, vi.fn(), true);
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirTitle" }));
    expect(screen.getByText("taskConfigurationDirectoryLocked")).toBeInTheDocument();
    expect(screen.getByText("/project")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "taskConfigurationDirectoryPath" })).not.toBeInTheDocument();
  });
});
