import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CreateModelProviderModal } from "@/pages/WorkspacePage/components/WorkspaceModals";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";

const labels: Record<string, string> = {
  cancel: "Cancel",
  close: "Close",
  modelProviderAPIKeyHint: "Stored locally.",
  modelProviderAvatar: "Avatar",
  modelProviderBaseURLRequired: "Base URL is required.",
  modelProviderCheck: "Check",
  modelProviderCreateAction: "Create",
  modelProviderCreateConnectionDescription: "Save the endpoint, key, and model list.",
  modelProviderCreateConnectionTitle: "Connection",
  modelProviderCreateIdentityDescription: "Set a sidebar name.",
  modelProviderCreateIdentityTitle: "Identity",
  modelProviderCreatePresetDescription: "Choose a provider preset to prefill the endpoint and display name.",
  modelProviderCreatePresetTitle: "Type",
  modelProviderCreateSubtitle: "Add a reusable model API provider.",
  modelProviderCreateTitle: "Add model provider",
  modelProviderDisplayName: "Display name",
  modelProviderDuplicateDisplayName: "Display name is already used.",
  modelProviderModelCount: "{count} models",
  modelProviderModels: "Models",
  modelProviderModelsHint: "Optional models.",
  modelProviderModelSearch: "Search models",
  modelProviderNoModels: "No models",
  modelProviderNotChecked: "Not checked",
  modelProviderPreset: "Provider preset",
  modelProviderPresetCustom: "Custom",
  modelProviderPresetDeepSeek: "DeepSeek",
  modelProviderPresetOpenAI: "OpenAI",
  modelProviderPresetZhipu: "Zhipu",
  profileAPIKey: "API Key",
  profileAPIKeyNewPlaceholder: "sk-...",
  profileBaseURL: "Base URL",
};

const t: TranslateFn = (key, params = {}) =>
  Object.entries(params).reduce((text, [name, value]) => text.replace(`{${name}}`, String(value)), labels[key] ?? key);

describe("CreateModelProviderModal", () => {
  it("renders a full OpenAI-compatible provider form", () => {
    render(<CreateModelProviderModal busy={false} modelProviders={null} onClose={vi.fn()} onCreate={vi.fn()} t={t} />);

    expect(screen.getByLabelText(/Display name/)).toHaveValue("");
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Provider preset" })).toHaveTextContent("OpenAI");
    expect(screen.getByLabelText(/Base URL/)).toHaveValue("https://api.openai.com/v1");
    expect(screen.getByLabelText(/API Key/)).toHaveValue("");
    expect(screen.getByLabelText(/API Key/)).toHaveAttribute("type", "password");
    const modelList = screen.getByRole("list", { name: "Models" });
    expect(modelList).toBeInTheDocument();
    expect(within(modelList).getByText("Not checked")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Models" })).not.toBeInTheDocument();
  });

  it("submits endpoint, key, and normalized seed models", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn();
    const onCheckAccess = vi.fn().mockResolvedValue({
      id: "openai-draft",
      status: "connected",
      models: ["gpt-4.1", "gpt-4.1", "gpt-4.1-mini"],
    });

    render(
      <CreateModelProviderModal
        busy={false}
        modelProviders={null}
        onCheckAccess={onCheckAccess}
        onClose={vi.fn()}
        onCreate={onCreate}
        t={t}
      />,
    );

    await user.type(screen.getByLabelText(/Display name/), "Team API");
    await user.clear(screen.getByLabelText(/Base URL/));
    await user.type(screen.getByLabelText(/Base URL/), "http://127.0.0.1:4000/v1");
    await user.type(screen.getByLabelText(/API Key/), "sk-test");
    await user.click(screen.getByRole("button", { name: "Check" }));
    expect(await screen.findByText("gpt-4.1-mini")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(onCreate).toHaveBeenCalledWith({
      api_key: "sk-test",
      base_url: "http://127.0.0.1:4000/v1",
      display_name: "Team API",
      models: ["gpt-4.1", "gpt-4.1-mini"],
      preset: "openai",
    });
  });

  it("blocks duplicate display names", async () => {
    const user = userEvent.setup();
    const catalog = normalizeModelProviderCatalog({
      providers: [{ id: "openai", display_name: "OpenAI API", models: [] }],
    });

    render(
      <CreateModelProviderModal busy={false} modelProviders={catalog} onClose={vi.fn()} onCreate={vi.fn()} t={t} />,
    );

    expect(screen.queryByText("Display name is already used.")).not.toBeInTheDocument();
    await user.type(screen.getByLabelText(/Display name/), "OpenAI API");
    expect(screen.getByText("Display name is already used.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
  });

  it("loads models after an explicit connection check", async () => {
    const user = userEvent.setup();
    const onCheckAccess = vi.fn().mockResolvedValue({
      id: "openai-draft",
      status: "connected",
      models: ["gpt-4.1", "gpt-4.1-mini"],
    });

    render(
      <CreateModelProviderModal
        busy={false}
        modelProviders={null}
        onCheckAccess={onCheckAccess}
        onClose={vi.fn()}
        onCreate={vi.fn()}
        t={t}
      />,
    );

    await user.type(screen.getByLabelText(/Display name/), "Team API");
    await user.clear(screen.getByLabelText(/Base URL/));
    await user.type(screen.getByLabelText(/Base URL/), "http://127.0.0.1:4000/v1");
    await user.type(screen.getByLabelText(/API Key/), "sk-test");

    await user.click(screen.getByRole("button", { name: "Check" }));
    expect(await screen.findByText("gpt-4.1-mini")).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Models" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Models" })).not.toBeInTheDocument();
    expect(onCheckAccess).toHaveBeenCalledWith({
      api_key: "sk-test",
      base_url: "http://127.0.0.1:4000/v1",
      display_name: "Team API",
      models: [],
      preset: "openai",
    });
  });

  it("switches preset defaults before the user enters connection details", async () => {
    const user = userEvent.setup();

    render(<CreateModelProviderModal busy={false} modelProviders={null} onClose={vi.fn()} onCreate={vi.fn()} t={t} />);

    await user.click(screen.getByRole("combobox", { name: "Provider preset" }));
    await user.click(screen.getByRole("option", { name: "Zhipu" }));

    expect(screen.getByLabelText(/Base URL/)).toHaveValue("https://open.bigmodel.cn/api/paas/v4");
    expect(screen.getByLabelText(/Display name/)).toHaveAttribute("placeholder", "Zhipu API");
  });

  it("renders failed provider checks as inline warnings", async () => {
    const user = userEvent.setup();
    const onCheckAccess = vi.fn().mockRejectedValue(new Error("status 401 Unauthorized"));

    const { container } = render(
      <CreateModelProviderModal
        busy={false}
        modelProviders={null}
        onCheckAccess={onCheckAccess}
        onClose={vi.fn()}
        onCreate={vi.fn()}
        t={t}
      />,
    );

    await user.type(screen.getByLabelText(/API Key/), "bad-key");
    await user.click(screen.getByRole("button", { name: "Check" }));

    await screen.findAllByText("status 401 Unauthorized");
    const warning = container.querySelector(".create-model-provider-check-status.warning");
    expect(warning).toHaveTextContent("status 401 Unauthorized");
    expect(warning).not.toHaveClass("form-error");
  });

  it("keeps failed provider checks visible in the connection section", async () => {
    const user = userEvent.setup();
    const onCheckAccess = vi.fn().mockRejectedValue(new Error("status 401 Unauthorized"));

    const { container } = render(
      <CreateModelProviderModal
        busy={false}
        modelProviders={null}
        onCheckAccess={onCheckAccess}
        onClose={vi.fn()}
        onCreate={vi.fn()}
        t={t}
      />,
    );

    await user.type(screen.getByLabelText(/API Key/), "bad-key");
    await user.click(screen.getByRole("button", { name: "Check" }));

    await screen.findAllByText("status 401 Unauthorized");
    const warning = container.querySelector(".create-model-provider-check-status.warning");
    const header = container.querySelector(".create-model-provider-modal .modal-header");
    const body = container.querySelector(".create-model-provider-body");

    expect(header).not.toContainElement(warning as HTMLElement);
    expect(body).toContainElement(warning as HTMLElement);
    expect(warning?.closest(".create-model-provider-check-row")).toBeInTheDocument();
  });
});
