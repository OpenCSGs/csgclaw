import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import type { AppInstallation, AppDefinition } from "@/api/apps";
import { createAppResource, fetchAppResource, cancelCLIOperation, startCLIOperation } from "@/api/apps";
import { CLIDialog } from "./CLIDialog";

vi.mock("@/api/apps", () => ({
  fetchAppResource: vi.fn(),
  createAppResource: vi.fn(),
  startCLIOperation: vi.fn(),
  cancelCLIOperation: vi.fn(),
}));
const t = createTranslator("en");
const resource: AppInstallation = {
  installation_id: "global-cli",
  agent_id: "",
  app_id: "feishu",
  name: "Feishu",
  kind: "cli",
  revision: 4,
  enabled: true,
  disconnected: false,
  status: "connecting",
  config: {},
  credentials_set: {},
  tools: [],
  created_at: "",
  updated_at: "",
  operation: {
    operation_id: "connect-1",
    session_id: "session-1",
    resource_id: "global-cli",
    resource_revision: 4,
    action: "connect",
    stage: "user_authorization",
    status: "running",
    authorization_url: "https://open.feishu.cn/device",
  },
};
afterEach(() => {
  vi.resetAllMocks();
  vi.useRealTimers();
});
const definition: AppDefinition = {
  app_id: "feishu",
  kind: "cli",
  name: "Feishu",
  description: "Feishu CLI",
  version: "0.1.0",
  interface: {},
  config_schema: {},
  auth_methods: ["command"],
  oauth_supported: true,
};

describe("Feishu CLI authorization", () => {
  it.each([
    ["user_authorization", false, false],
    ["user_authorization", true, false],
    ["applying", true, true],
  ] as const)(
    "shows authorization progress and retains cancellation during %s (user=%s, authorized=%s)",
    async (stage, user, authorized) => {
      const progress: AppInstallation = {
        ...resource,
        authorized,
        operation: { ...resource.operation!, stage },
        auth_status: {
          appId: "cli_test",
          defaultAs: "auto",
          identities: {
            bot: { status: "ready", available: true, verified: true, appName: "Test bot" },
            user: { status: user ? "ready" : "pending", available: user, verified: user, userName: "Test user" },
          },
        },
      };
      vi.mocked(fetchAppResource).mockResolvedValue(progress);
      vi.mocked(cancelCLIOperation).mockResolvedValue(progress.operation!);
      render(<CLIDialog definition={definition} existing={resource} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);
      const progressPanel = await screen.findByLabelText("Authorization progress");
      expect(within(progressPanel).getByText("Test bot")).toBeInTheDocument();
      expect(within(progressPanel).getByText("Test user")).toBeInTheDocument();
      expect(within(progressPanel).getAllByText("Completed")).toHaveLength(user ? 2 : 1);
      if (!user) expect(within(progressPanel).getByText("In progress")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Disconnect" })).not.toBeInTheDocument();
      await userEvent.setup().click(screen.getByRole("button", { name: "Cancel" }));
      expect(cancelCLIOperation).toHaveBeenCalledWith("global-cli", "session-1");
    },
  );
  it("makes the bot the current step before personal authorization begins", async () => {
    const appAuthorization: AppInstallation = {
      ...resource,
      operation: {
        ...resource.operation!,
        stage: "app_authorization",
        authorization_url: "https://feishu.example/app",
      },
      auth_status: {
        appId: "cli_test",
        defaultAs: "auto",
        identities: {
          bot: { status: "pending", available: false },
          user: { status: "pending", available: false },
        },
      },
    };
    vi.mocked(fetchAppResource).mockResolvedValue(appAuthorization);
    render(
      <CLIDialog definition={definition} existing={appAuthorization} t={t} onClose={vi.fn()} onChanged={vi.fn()} />,
    );

    const progressPanel = await screen.findByLabelText("Authorization progress");
    expect(within(progressPanel).getByText("In progress")).toBeInTheDocument();
    expect(within(progressPanel).getByText("Authorization required")).toBeInTheDocument();
  });
  it("disconnects both identities through one action", async () => {
    const connected: AppInstallation = {
      ...resource,
      status: "connected",
      authorized: true,
      operation: undefined,
      auth_status: {
        appId: "cli_test",
        defaultAs: "auto",
        identities: {
          bot: { status: "ready", available: true, verified: true },
          user: { status: "ready", available: true, verified: true },
        },
      },
    };
    vi.mocked(fetchAppResource).mockResolvedValue(connected);
    vi.mocked(startCLIOperation).mockResolvedValue({ ...resource.operation!, action: "disconnect" });
    render(<CLIDialog definition={definition} existing={connected} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Disconnect" }));
    expect(startCLIOperation).toHaveBeenCalledWith(connected, "disconnect", expect.any(String));
  });
  it("uses the connector authorization result for connection actions", async () => {
    const unverified: AppInstallation = {
      ...resource,
      authorized: false,
      status: "needs_configuration",
      operation: undefined,
      auth_status: {
        appId: "cli_test",
        defaultAs: "auto",
        identities: { bot: { status: "unknown", available: true }, user: { status: "unknown", available: false } },
      },
    };
    vi.mocked(fetchAppResource).mockResolvedValue(unverified);
    render(<CLIDialog definition={definition} existing={unverified} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Connect Feishu" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Disconnect" })).not.toBeInTheDocument();
  });
  it.each([null, []])("renders completed migration with workers %j", async (workers) => {
    const completed: AppInstallation = {
      ...resource,
      status: "connected",
      authorized: true,
      operation: undefined,
      migration: { status: "completed", workers },
    };
    vi.mocked(fetchAppResource).mockResolvedValue(completed);
    render(<CLIDialog definition={definition} existing={completed} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);
    expect(await screen.findByRole("button", { name: "Close" })).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
  it("restores the active authorization and keeps it running when the dialog closes", async () => {
    vi.mocked(fetchAppResource).mockResolvedValue(resource);
    const close = vi.fn();
    render(<CLIDialog definition={definition} existing={resource} t={t} onClose={close} onChanged={vi.fn()} />);
    expect(await screen.findByRole("link", { name: "Authorize account" })).toHaveAttribute(
      "href",
      "https://open.feishu.cn/device",
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "Close" }));
    expect(close).toHaveBeenCalledOnce();
    expect(cancelCLIOperation).not.toHaveBeenCalled();
    expect(startCLIOperation).not.toHaveBeenCalled();
  });
  it("cancels the persisted session explicitly", async () => {
    const operation = {
      ...resource.operation!,
      status: "cancelled",
      authorization_url: undefined,
    };
    vi.mocked(fetchAppResource).mockResolvedValue(resource);
    vi.mocked(cancelCLIOperation).mockImplementation(async () => {
      vi.mocked(fetchAppResource).mockResolvedValue({ ...resource, operation });
      return operation;
    });
    render(<CLIDialog definition={definition} existing={resource} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(cancelCLIOperation).toHaveBeenCalledWith("global-cli", "session-1"));
    expect(screen.queryByRole("link", { name: "Authorize account" })).not.toBeInTheDocument();
  });
  it.each([false, true])(
    "refreshes cancelled authorization before the regular polling tick (pending=%s)",
    async (pending) => {
      vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
      const user = userEvent.setup();
      const cancelled: AppInstallation = {
        ...resource,
        status: "needs_configuration",
        operation: { ...resource.operation!, status: "cancelled", authorization_url: undefined },
      };
      vi.mocked(fetchAppResource).mockResolvedValue(resource);
      vi.mocked(cancelCLIOperation).mockImplementation(async () => {
        if (!pending) vi.mocked(fetchAppResource).mockResolvedValue(cancelled);
        return resource.operation!;
      });
      const changed = vi.fn().mockResolvedValue(undefined);
      render(<CLIDialog definition={definition} existing={resource} t={t} onClose={vi.fn()} onChanged={changed} />);
      await act(async () => {});
      await user.click(screen.getByRole("button", { name: "Cancel" }));
      if (pending) {
        expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
        expect(changed).not.toHaveBeenCalled();
        vi.mocked(fetchAppResource).mockResolvedValue(cancelled);
        await act(async () => {
          await vi.advanceTimersByTimeAsync(100);
        });
      }
      await waitFor(() => expect(changed).toHaveBeenCalledOnce());
      expect(screen.queryByText("In progress")).not.toBeInTheDocument();
      expect(screen.queryByRole("link", { name: "Authorize account" })).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Connect Feishu" })).toBeEnabled();
    },
  );

  it("ignores a resource response started before cancellation", async () => {
    let release!: (value: AppInstallation) => void;
    vi.mocked(fetchAppResource).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );
    const cancelled: AppInstallation = {
      ...resource,
      status: "needs_configuration",
      operation: { ...resource.operation!, status: "cancelled", authorization_url: undefined },
    };
    vi.mocked(fetchAppResource).mockResolvedValue(cancelled);
    vi.mocked(cancelCLIOperation).mockResolvedValue(resource.operation!);
    render(<CLIDialog definition={definition} existing={resource} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Cancel" }));
    await screen.findByRole("button", { name: "Connect Feishu" });
    await act(async () => {
      release(resource);
    });
    expect(screen.queryByText("In progress")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect Feishu" })).toBeEnabled();
  });
  it("recovers from a failed refresh after cancellation", async () => {
    vi.mocked(fetchAppResource).mockResolvedValue(resource);
    vi.mocked(cancelCLIOperation).mockImplementation(async () => {
      vi.mocked(fetchAppResource).mockRejectedValueOnce(new Error("Resource service unavailable"));
      return resource.operation!;
    });
    const changed = vi.fn();
    render(<CLIDialog definition={definition} existing={resource} t={t} onClose={vi.fn()} onChanged={changed} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Resource service unavailable");
    expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();
    expect(changed).not.toHaveBeenCalled();
  });
  it("stops refreshing a pending cancellation when the dialog unmounts", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    vi.mocked(fetchAppResource).mockResolvedValue(resource);
    vi.mocked(cancelCLIOperation).mockResolvedValue(resource.operation!);
    const { unmount } = render(
      <CLIDialog definition={definition} existing={resource} t={t} onClose={vi.fn()} onChanged={vi.fn()} />,
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    unmount();
    vi.mocked(fetchAppResource).mockClear();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
      window.dispatchEvent(new Event("focus"));
      document.dispatchEvent(new Event("visibilitychange"));
    });
    expect(fetchAppResource).not.toHaveBeenCalled();
  });
  it("shows migration failures and offers reconnection", async () => {
    const failed = {
      ...resource,
      status: "error" as const,
      last_error_code: "migration_failed",
      operation: undefined,
      migration: {
        status: "waiting_for_legacy_callers",
        workers: [
          { agent_id: "worker", phase: "original", error_code: "migration_failed", verification_pending: false },
        ],
      },
    };
    vi.mocked(fetchAppResource).mockResolvedValue(failed);
    render(<CLIDialog definition={definition} existing={failed} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Historical configuration migration needs repair.");
    expect(screen.getByRole("button", { name: "Connect Feishu" })).toBeEnabled();
  });
  it("hides an expired authorization error after a new authorization starts", async () => {
    const expired: AppInstallation = {
      ...resource,
      authorized: false,
      status: "authorization_required",
      last_error_code: "authorization_expired",
      operation: undefined,
    };
    vi.mocked(fetchAppResource).mockResolvedValue(expired);
    vi.mocked(startCLIOperation).mockResolvedValue(resource.operation!);
    render(<CLIDialog definition={definition} existing={expired} t={t} onClose={vi.fn()} onChanged={vi.fn()} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Authorization expired. Authorize again; the bot configuration is preserved.",
    );
    await userEvent
      .setup()
      .click(within(screen.getByRole("alert").parentElement!).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Connect Feishu" }));

    await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
    expect(screen.getByRole("link", { name: "Authorize account" })).toBeInTheDocument();
  });
});

it("creates and authorizes the selected CLI package", async () => {
  const custom: AppInstallation = {
    ...resource,
    app_id: "fixture",
    name: "Fixture CLI",
    status: "needs_configuration",
    operation: undefined,
  };
  vi.mocked(createAppResource).mockResolvedValue(custom);
  vi.mocked(fetchAppResource).mockResolvedValue(custom);
  vi.mocked(startCLIOperation).mockResolvedValue({
    ...resource.operation!,
    authorization_url: "https://example.com/authorize",
  });
  render(
    <CLIDialog
      definition={{ ...definition, app_id: "fixture", name: "Fixture CLI", description: "Use fixture commands." }}
      existing={null}
      t={t}
      onClose={vi.fn()}
      onChanged={vi.fn()}
    />,
  );
  expect(screen.getByRole("heading", { name: "Fixture CLI" })).toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "Connect" }));
  await waitFor(() =>
    expect(createAppResource).toHaveBeenCalledWith({
      app_id: "fixture",
      kind: "cli",
      name: "Fixture CLI",
      config: {},
      credentials: {},
      connect: false,
    }),
  );
  expect(startCLIOperation).toHaveBeenCalledWith(custom, "connect", expect.any(String));
});
