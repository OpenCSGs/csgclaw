import { pickLocalDirectoryPath } from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";
import { TASK_DIRECTORIES_STORAGE_KEY } from "@/shared/storage/keys";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { TaskConfigurationToolbar } from "@/components/business/TaskConfigurationToolbar";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import type { WorkspaceController } from "@/hooks/workspace";
import type { TaskConfiguration } from "@/models/taskConfiguration";

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
  beforeEach(() => {
    localStorage.clear();
    vi.mocked(pickLocalDirectoryPath).mockReset();
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
    expect(slider).toHaveAttribute("aria-valuetext", "profileReasoningAuto");
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
    expect(screen.getByRole("slider")).toHaveAttribute("aria-valuetext", "profileReasoningAuto");
  });
  it("groups chat models, keeps the agent default searchable and restores it without changing the agent", async () => {
    const { onChange } = setup({ model_provider_id: "codex", model_id: "gpt-test", reasoning_effort: "high" });
    fireEvent.click(screen.getByRole("combobox", { name: "iaModelTitle" }));
    expect(screen.getByText("Codex")).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /iaModelFollowAgent/ })).toHaveTextContent("default-model");
    expect(screen.queryByRole("option", { name: /image-test/ })).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "iaModelSearch" }), { target: { value: "no match" } });
    fireEvent.click(screen.getByRole("option", { name: /iaModelFollowAgent/ }));
    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith({ model_provider_id: "", model_id: "", reasoning_effort: "" }),
    );
  });
  it("opens custom provider creation directly and keeps the conversation on completion", async () => {
    const { controller, onChange } = setup();
    fireEvent.click(screen.getByRole("combobox", { name: "iaModelTitle" }));
    fireEvent.click(screen.getByRole("button", { name: "iaModelConfigure" }));
    expect(controller.sidebarProps?.onCreateModelProvider).toHaveBeenCalledWith({
      preset: "custom",
      onConnected: expect.any(Function),
    });
    const request = vi.mocked(controller.sidebarProps!.onCreateModelProvider!).mock.calls[0][0];
    act(() => request?.onConnected?.(controller.modelProviders!.providers[0]));
    expect(screen.getByRole("listbox", { name: "iaModelTitle" })).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
    expect(controller.openAgentSettings).not.toHaveBeenCalled();
  });
  it("searches remembered directories and keeps the default available", async () => {
    localStorage.setItem(TASK_DIRECTORIES_STORAGE_KEY, JSON.stringify(["/work/frontend", "/work/backend"]));
    const { onChange } = setup();
    fireEvent.click(screen.getByRole("combobox", { name: "iaWorkdirTitle" }));
    fireEvent.change(screen.getByRole("combobox", { name: "iaWorkdirSearch" }), { target: { value: "backend" } });
    expect(screen.queryByRole("option", { name: /frontend/ })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: /iaWorkdirDefault/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("option", { name: /backend/ }));
    await waitFor(() => expect(onChange).toHaveBeenCalledWith({ workspace_dir: "/work/backend" }));
  });
  it("adds a native directory only after its configuration is saved", async () => {
    vi.mocked(pickLocalDirectoryPath).mockResolvedValue("/work/new-project");
    const { onChange } = setup();
    fireEvent.click(screen.getByRole("combobox", { name: "iaWorkdirTitle" }));
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirAdd" }));
    await waitFor(() => expect(onChange).toHaveBeenCalledWith({ workspace_dir: "/work/new-project" }));
    await waitFor(() =>
      expect(JSON.parse(localStorage.getItem(TASK_DIRECTORIES_STORAGE_KEY)!)).toEqual(["/work/new-project"]),
    );
  });
  it("keeps the saved model and reports errors when a session update fails", async () => {
    const save = vi.fn().mockRejectedValue(new Error("still running"));
    setup({}, save);
    fireEvent.click(screen.getByRole("combobox", { name: "iaModelTitle" }));
    fireEvent.click(screen.getByRole("option", { name: /gpt-test/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("still running");
    expect(screen.getByRole("option", { name: /iaModelFollowAgent/ })).toHaveAttribute("aria-selected", "true");
  });
  it("does not remember a directory after a canceled or rejected selection", async () => {
    const save = vi.fn().mockRejectedValue(new Error("invalid directory"));
    vi.mocked(pickLocalDirectoryPath).mockResolvedValueOnce(null).mockResolvedValueOnce("/invalid");
    setup({}, save);
    fireEvent.click(screen.getByRole("combobox", { name: "iaWorkdirTitle" }));
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirAdd" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "iaWorkdirAdd" })).toBeEnabled());
    expect(save).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirAdd" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("invalid directory");
    expect(localStorage.getItem(TASK_DIRECTORIES_STORAGE_KEY)).toBeNull();
  });
  it("locks the directory in an existing conversation", () => {
    setup({ workspace_dir: "/project" }, vi.fn(), true);
    expect(screen.queryByRole("combobox", { name: "iaWorkdirTitle" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("iaWorkdirTitle")).toHaveAttribute("title", "/project · iaWorkdirLocked");
  });
});
