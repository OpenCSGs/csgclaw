import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TaskConfigurationToolbar } from "@/components/business/TaskConfigurationToolbar";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import type { WorkspaceController } from "@/hooks/workspace";
import type { TaskConfiguration } from "@/models/taskConfiguration";

function setup(configuration: TaskConfiguration = {}, onChange = vi.fn(), directoryLocked = false) {
  const controller = {
    ready: true,
    t: (key: string) => key,
    sidebarProps: { locale: "en" },
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
  render(
    <WorkspaceControllerProvider controller={controller}>
      <TaskConfigurationToolbar
        agent={{ id: "agent-test", name: "Test", runtime_kind: "codex", model_config: { model_id: "default-model" } }}
        configuration={configuration}
        directoryLocked={directoryLocked}
        onChange={onChange}
      />
    </WorkspaceControllerProvider>,
  );
  return { onChange, controller };
}

describe("task configuration", () => {
  it("applies a model to this session and keeps the working directory", async () => {
    const { onChange, controller } = setup({ workspace_dir: "/project" });
    fireEvent.click(screen.getByRole("button", { name: "iaModelTitle" }));
    expect(screen.queryByRole("button", { name: "image-test" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "gpt-test" }));
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
    setup({ model_id: "gpt-test", model_provider_id: "codex" }, save);
    fireEvent.click(screen.getByRole("button", { name: "iaReasoningTitle" }));
    fireEvent.click(screen.getByRole("button", { name: /^high$/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("busy conversation");
    expect(save).toHaveBeenCalledWith({ model_id: "gpt-test", model_provider_id: "codex", reasoning_effort: "high" });
  });
  it("locks the directory in an existing conversation", () => {
    setup({ workspace_dir: "/project" }, vi.fn(), true);
    fireEvent.click(screen.getByRole("button", { name: "iaWorkdirTitle" }));
    expect(screen.getByText("taskConfigurationDirectoryLocked")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "taskConfigurationDirectoryPath" })).not.toBeInTheDocument();
  });
});
