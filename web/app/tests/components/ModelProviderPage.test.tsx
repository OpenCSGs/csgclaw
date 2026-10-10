import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { checkModelProvider, deleteModelProvider, updateModelProvider } from "@/api/modelProviders";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import { ModelProviderPage } from "@/pages/ModelProviderPage/ModelProviderPage";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";
import { WorkspacePaneTypes } from "@/models/routing";
import type { TranslateFn } from "@/models/conversations";

vi.mock("@/api/modelProviders", () => ({
  checkModelProvider: vi.fn(),
  deleteModelProvider: vi.fn(),
  updateModelProvider: vi.fn(),
}));

const labels: Record<string, string> = {
  agentDelete: "Delete",
  agentName: "Name",
  agentUpdateSave: "Save",
  csghubLoginPending: "Waiting for sign-in",
  csghubNotSignedIn: "Not signed in",
  csghubReauthorize: "Authorize again",
  csghubSignIn: "Sign in",
  modelProviderAIGatewayAddress: "AI Gateway address",
  modelProviderCheck: "Check",
  modelProviderConfiguration: "Configuration",
  modelProviderConnected: "Connected",
  modelContextRestoreDefaults: "Restore defaults",
  modelProviderCustomSettings: "OpenAI-compatible provider settings",
  modelProviderOpenCSGSettings: "OpenCSG built-in models are served by AI Gateway.",
  modelProviderOpenCSGSignInRequired:
    "Sign in in Settings to access OpenCSG Models. The model list is available after your OpenCSG account is connected.",
  modelProviderModelCount: "{count} models",
  modelProviderModelSearch: "Search models",
  modelProviderModels: "Models",
  modelProviderNoModels: "No models",
  modelsSection: "Models",
  profileAPIKey: "API Key",
  profileBaseURL: "Base URL",
  profileLoadingModels: "Loading models...",
  profileModel: "Model",
  profileSavedToast: "Saved",
  profileSelectModel: "Select model",
};

const t: TranslateFn = (key, params = {}) =>
  Object.entries(params).reduce((text, [name, value]) => text.replace(`{${name}}`, String(value)), labels[key] ?? key);

function createCatalog(providerOverrides: Record<string, unknown> = {}) {
  return normalizeModelProviderCatalog({
    providers: [
      {
        id: "openai",
        kind: "openai_compatible",
        preset: "openai",
        display_name: "OpenAI API",
        base_url: "https://api.openai.com/v1",
        api_key_set: true,
        api_key_preview: "sk-...",
        models: ["gpt-4.1"],
        status: "connected",
        ...providerOverrides,
      },
    ],
  });
}

function renderModelProviderPage(
  catalog = createCatalog(),
  providerID = "openai",
  controllerOverrides: Record<string, unknown> = {},
) {
  const refreshWorkspaceModelProviders = vi.fn().mockResolvedValue(null);
  const renderPage = (nextCatalog = catalog) => (
    <WorkspaceControllerProvider
      controller={
        {
          activePane: { type: WorkspacePaneTypes.modelProvider, id: providerID },
          modelProviders: nextCatalog,
          ready: true,
          refreshWorkspaceModelProviders,
          sidebarProps: {
            authBusy: false,
            authPending: false,
            authStatus: { authenticated: true },
            onLogin: vi.fn(),
          },
          t,
          ...controllerOverrides,
        } as never
      }
    >
      <ModelProviderPage />
    </WorkspaceControllerProvider>
  );

  const view = render(renderPage());
  const card = view.container.querySelector(".model-provider-provider-card.active");
  if (
    card &&
    !(controllerOverrides.sidebarProps as { onSelectModelProvider?: unknown } | undefined)?.onSelectModelProvider
  )
    fireEvent.click(card);

  return {
    container: view.container,
    refreshWorkspaceModelProviders,
    rerenderWithCatalog: (nextCatalog: ReturnType<typeof createCatalog>) => view.rerender(renderPage(nextCatalog)),
  };
}

describe("ModelProviderPage", () => {
  beforeEach(() => {
    vi.mocked(checkModelProvider).mockResolvedValue({
      id: "openai",
      last_checked_at: "2026-06-23T12:00:00Z",
      models: ["gpt-4.1"],
      status: "connected",
    });
    vi.mocked(updateModelProvider).mockResolvedValue({
      id: "openai",
      kind: "openai_compatible",
      preset: "openai",
      builtin: false,
      display_name: "OpenAI API",
      base_url: "https://api.openai.com/v1",
      models: ["gpt-4.1"],
      status: "connected",
    });
    vi.mocked(deleteModelProvider).mockResolvedValue();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it.each(["codex", "claude_code"])("clears stale %s models after a failed auth check", async (id) => {
    vi.mocked(checkModelProvider).mockResolvedValue({ id, models: [], status: "failed", message: "Sign in first" });
    const catalog = createCatalog({ id, kind: id, builtin: true, models: ["stale-model"] });
    const { rerenderWithCatalog, refreshWorkspaceModelProviders } = renderModelProviderPage(catalog, id);
    await waitFor(() => expect(refreshWorkspaceModelProviders).toHaveBeenCalled());
    rerenderWithCatalog(
      createCatalog({ id, kind: id, builtin: true, models: [], status: "failed", message: "Sign in first" }),
    );
    await waitFor(() => expect(screen.queryByText("stale-model")).not.toBeInTheDocument());
    expect(screen.getAllByText("Sign in first").length).toBeGreaterThan(0);
    expect(screen.queryByText("Connected")).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Check" }));
    expect(within(screen.getByRole("dialog")).getAllByText("Sign in first")).toHaveLength(1);
  });

  it("checks the provider when opening the page", async () => {
    renderModelProviderPage();

    await waitFor(() =>
      expect(checkModelProvider).toHaveBeenCalledWith("openai", {
        api_key: "",
        base_url: "https://api.openai.com/v1",
      }),
    );
  });

  it("keeps the OpenCSG detail closed when authentication intercepts its card", async () => {
    const user = userEvent.setup();
    const onSelectModelProvider = vi.fn(() => false);
    renderModelProviderPage(
      normalizeModelProviderCatalog({
        providers: [
          {
            id: "opencsg",
            kind: "opencsg",
            builtin: true,
            display_name: "OpenCSG",
            models: [],
            status: "failed",
          },
        ],
      }),
      "opencsg",
      {
        sidebarProps: {
          authBusy: false,
          authPending: false,
          authStatus: { authenticated: false },
          onLogin: vi.fn(),
          onSelectModelProvider,
        },
      },
    );

    await user.click(screen.getByRole("button", { name: /OpenCSG/ }));

    expect(onSelectModelProvider).toHaveBeenCalledWith(expect.objectContaining({ id: "opencsg" }), {
      requireAuthentication: true,
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows the resolved CSGHub Lite desktop API after fallback discovery", async () => {
    vi.mocked(checkModelProvider).mockResolvedValue({
      id: "csghub-lite",
      base_url: "http://127.0.0.1:11436/v1",
      last_checked_at: "2026-08-03T12:00:00Z",
      models: ["Qwen3.5-2B"],
      status: "connected",
    });
    renderModelProviderPage(
      createCatalog({
        id: "csghub-lite",
        kind: "csghub_lite",
        preset: "csghub-lite",
        builtin: true,
        display_name: "CSGHub Lite",
        base_url: "http://127.0.0.1:11435/v1",
        models: [],
        status: "failed",
      }),
      "csghub-lite",
    );

    await waitFor(() => expect(screen.getByLabelText("Base URL")).toHaveValue("http://127.0.0.1:11436/v1"));
    expect(screen.getByText("Qwen3.5-2B")).toBeInTheDocument();
  });

  it("keeps form values while typing and saves the updated provider", async () => {
    const user = userEvent.setup();
    renderModelProviderPage();

    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Team OpenAI");
    await user.clear(screen.getByLabelText("Base URL"));
    await user.type(screen.getByLabelText("Base URL"), "http://127.0.0.1:4000/v1");
    await user.type(screen.getByLabelText("API Key"), "sk-test");
    expect(screen.getByRole("list", { name: "Models" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Model" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Add model")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Remove/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(updateModelProvider).toHaveBeenCalledWith("openai", {
        api_key: "sk-test",
        base_url: "http://127.0.0.1:4000/v1",
        display_name: "Team OpenAI",
        models: ["gpt-4.1"],
        model_overrides: {},
      }),
    );
  });

  it("keeps unsaved edits when the provider catalog refreshes", async () => {
    const user = userEvent.setup();
    const { rerenderWithCatalog } = renderModelProviderPage();

    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Team OpenAI");
    await user.clear(screen.getByLabelText("Base URL"));
    await user.type(screen.getByLabelText("Base URL"), "http://127.0.0.1:4000/v1");
    await user.type(screen.getByLabelText("API Key"), "sk-unsaved");

    rerenderWithCatalog(
      createCatalog({
        last_checked_at: "2026-06-23T12:01:00Z",
        message: "connected",
        status: "connected",
      }),
    );

    expect(screen.getByLabelText("Name")).toHaveValue("Team OpenAI");
    expect(screen.getByLabelText("Base URL")).toHaveValue("http://127.0.0.1:4000/v1");
    expect(screen.getByLabelText("API Key")).toHaveValue("sk-unsaved");
  });

  it("filters model rows without changing the saved catalog", async () => {
    const user = userEvent.setup();
    renderModelProviderPage();

    expect(screen.getByRole("list", { name: "Models" })).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Search models"), "mini");

    expect(screen.queryByText("gpt-4.1")).not.toBeInTheDocument();
    expect(screen.getByText("No models")).toBeInTheDocument();
  });

  it("restores all OpenCSG model defaults even when search hides customized models, then saves", async () => {
    const user = userEvent.setup();
    renderModelProviderPage(
      createCatalog({
        id: "opencsg",
        kind: "opencsg",
        builtin: true,
        display_name: "OpenCSG",
        models: ["gpt-4.1", "qwen3.7-plus", "unknown-model"],
        model_overrides: {
          "gpt-4.1": { context_window: 100_000 },
          "qwen3.7-plus": { context_window: 100_000 },
          "unknown-model": { context_window: 100_000 },
        },
        model_defaults: {
          "gpt-4.1": { context_window: 1_050_000, context_source: "provider" },
          "qwen3.7-plus": { context_window: 1_000_000, context_source: "catalog" },
        },
      }),
      "opencsg",
    );
    await user.type(screen.getByLabelText("Search models"), "gpt");
    await user.click(screen.getByRole("button", { name: "Restore defaults" }));

    expect(updateModelProvider).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "gpt-4.1 modelMetadataContext" })).toHaveTextContent("1.05 M tokens");
    expect(screen.getByRole("button", { name: "Restore defaults" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    await user.clear(screen.getByLabelText("Search models"));
    expect(screen.getByRole("button", { name: "qwen3.7-plus modelMetadataContext" })).toHaveTextContent("1 M tokens");
    expect(screen.getByRole("button", { name: "unknown-model modelMetadataContext" })).toHaveTextContent(
      "200 K tokens",
    );

    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateModelProvider).toHaveBeenCalledWith("opencsg", { model_overrides: {} }));
  });

  it("disables restoring defaults when the provider has no customized context sizes", async () => {
    renderModelProviderPage();
    await waitFor(() => expect(checkModelProvider).toHaveBeenCalled());
    expect(screen.getByRole("button", { name: "Restore defaults" })).toBeDisabled();
  });

  it("restores one model from its row and saves the other model's customized context", async () => {
    const user = userEvent.setup();
    renderModelProviderPage(
      createCatalog({
        id: "opencsg",
        kind: "opencsg",
        builtin: true,
        models: ["gpt-4.1", "qwen3.7-plus"],
        model_overrides: {
          "gpt-4.1": { context_window: 100_000 },
          "qwen3.7-plus": { context_window: 100_000 },
        },
        model_defaults: {
          "gpt-4.1": { context_window: 1_050_000, context_source: "provider" },
          "qwen3.7-plus": { context_window: 1_000_000, context_source: "catalog" },
        },
      }),
      "opencsg",
    );
    await user.click(screen.getByRole("button", { name: "gpt-4.1 modelMetadataReset" }));
    expect(screen.getByRole("button", { name: "gpt-4.1 modelMetadataContext" })).toHaveTextContent("1.05 M tokens");
    expect(screen.getByRole("button", { name: "gpt-4.1 modelMetadataReset" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "qwen3.7-plus modelMetadataContext" })).toHaveTextContent("100 K tokens");
    expect(screen.getByRole("button", { name: "qwen3.7-plus modelMetadataReset" })).toBeEnabled();
    expect(updateModelProvider).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(updateModelProvider).toHaveBeenCalledWith("opencsg", {
        model_overrides: { "qwen3.7-plus": { context_window: 100_000 } },
      }),
    );
  });

  it("shows the OpenCSG built-in model page with sign-in guidance and AI Gateway address", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    renderModelProviderPage(
      normalizeModelProviderCatalog({
        providers: [
          {
            id: "opencsg",
            kind: "csghub",
            preset: "opencsg",
            display_name: "OpenCSG",
            builtin: true,
            base_url: "https://ai.space.opencsg.com/v1",
            models: [],
            status: "unknown",
          },
        ],
      }),
      "opencsg",
      {
        sidebarProps: {
          authBusy: false,
          authPending: false,
          authStatus: { authenticated: false },
          onLogin,
        },
      },
    );

    expect(await screen.findByRole("heading", { name: "OpenCSG" })).toBeInTheDocument();
    expect(
      screen.getByText(
        "Sign in in Settings to access OpenCSG Models. The model list is available after your OpenCSG account is connected.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("AI Gateway address")).toBeInTheDocument();
    expect(screen.getAllByText("https://ai.space.opencsg.com/v1")).toHaveLength(2);
    expect(document.querySelector(".model-provider-header-avatar")).toHaveAttribute(
      "src",
      "model-providers/opencsg.svg",
    );

    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(onLogin).toHaveBeenCalledWith();
  });

  it("allows OpenCSG authorization to be started again while a login is pending", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    renderModelProviderPage(
      normalizeModelProviderCatalog({
        providers: [
          {
            id: "opencsg",
            kind: "csghub",
            preset: "opencsg",
            display_name: "OpenCSG",
            builtin: true,
            base_url: "https://ai.space.opencsg.com/v1",
            models: [],
            status: "unknown",
          },
        ],
      }),
      "opencsg",
      {
        sidebarProps: {
          authBusy: false,
          authPending: true,
          authStatus: { authenticated: false },
          onLogin,
        },
      },
    );

    const retryButton = await screen.findByRole("button", { name: "Authorize again" });
    expect(retryButton).toBeEnabled();
    await user.click(retryButton);
    expect(onLogin).toHaveBeenCalledWith();
  });

  it("keeps the selected stage gateway visible and delegates login to the shared environment state", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    renderModelProviderPage(
      normalizeModelProviderCatalog({
        providers: [
          {
            id: "opencsg",
            kind: "csghub",
            preset: "opencsg",
            display_name: "OpenCSG",
            builtin: true,
            base_url: "https://aigateway.opencsg-stg.com/v1",
            models: [],
            status: "unknown",
          },
        ],
      }),
      "opencsg",
      {
        sidebarProps: {
          authBusy: false,
          authPending: false,
          authStatus: { authenticated: false },
          onLogin,
        },
      },
    );

    expect(screen.getAllByText("https://aigateway.opencsg-stg.com/v1")).toHaveLength(2);
    await user.click(await screen.findByRole("button", { name: "Sign in" }));

    expect(onLogin).toHaveBeenCalledWith();
  });

  it("loads OpenCSG models automatically after authentication", async () => {
    vi.mocked(checkModelProvider).mockResolvedValueOnce({
      id: "opencsg",
      last_checked_at: "2026-07-16T09:00:00Z",
      models: ["stage-model"],
      status: "connected",
    });
    renderModelProviderPage(
      normalizeModelProviderCatalog({
        providers: [
          {
            id: "opencsg",
            kind: "csghub",
            preset: "opencsg",
            display_name: "OpenCSG",
            builtin: true,
            base_url: "https://aigateway.opencsg-stg.com/v1",
            models: [],
            status: "unknown",
          },
        ],
      }),
      "opencsg",
      {
        sidebarProps: {
          authBusy: false,
          authPending: false,
          authStatus: { authenticated: true },
          onLogin: vi.fn(),
        },
      },
    );

    await waitFor(() =>
      expect(checkModelProvider).toHaveBeenCalledWith("opencsg", {
        api_key: "",
        base_url: "https://aigateway.opencsg-stg.com/v1",
      }),
    );
    expect(await screen.findByText("stage-model")).toBeInTheDocument();
  });

  it("clears rendered OpenCSG models when the provider catalog is cleared", async () => {
    vi.mocked(checkModelProvider).mockResolvedValueOnce({
      id: "opencsg",
      last_checked_at: "2026-07-16T09:00:00Z",
      models: [],
      status: "failed",
    });
    const openCSGProvider = {
      id: "opencsg",
      kind: "csghub",
      preset: "opencsg",
      display_name: "OpenCSG",
      builtin: true,
      base_url: "https://aigateway.opencsg-stg.com/v1",
      status: "connected",
    };
    const { rerenderWithCatalog } = renderModelProviderPage(
      normalizeModelProviderCatalog({
        providers: [{ ...openCSGProvider, models: ["stale-model"] }],
      }),
      "opencsg",
      {
        sidebarProps: {
          authBusy: false,
          authPending: false,
          authStatus: { authenticated: true },
          onLogin: vi.fn(),
        },
      },
    );

    expect(await screen.findByText("stale-model")).toBeInTheDocument();

    rerenderWithCatalog(
      normalizeModelProviderCatalog({
        providers: [{ ...openCSGProvider, models: [], status: "failed" }],
      }),
    );

    await waitFor(() => expect(screen.queryByText("stale-model")).not.toBeInTheDocument());
    expect(screen.getAllByText("No models")).toHaveLength(3);
  });
});
