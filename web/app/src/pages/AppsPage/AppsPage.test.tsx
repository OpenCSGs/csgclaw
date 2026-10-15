import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AppDefinition } from "@/api/apps";
import type { HubDetailPaneProps } from "@/components/business/HubDetailPane";
import { HubPage } from "@/pages/HubPage";
import { createTranslator } from "@/shared/i18n";
import { AppsPage } from "./AppsPage";

vi.mock("@/hooks/workspace", () => ({
  useWorkspaceControllerContext: () => ({ ready: true, hubViewProps: fixtureProps }),
}));
const t = createTranslator("en");
const fixtureProps: HubDetailPaneProps = {
  t,
  hub: {
    detailPaneProps: {
      selectedResourceType: "mcp",
      error: "",
      loaded: true,
      onRetry: vi.fn(),
      onSelectWorkspaceFile: vi.fn(),
      selectedSkill: null,
      selectedSkillPath: "",
      selectedTemplate: null,
      selectedTemplateId: "",
      selectedWorkspacePath: "",
      skillFile: null,
      skillFileError: "",
      skillFileLoading: false,
      skills: [],
      skillTree: null,
      skillTreeError: "",
      skillTreeLoading: false,
      templates: [],
      workspaceFile: null,
      workspaceFileError: "",
      workspaceFileLoading: false,
      workspaceEntries: [],
    },
  },
};

const definition: AppDefinition = {
  app_id: "feishu",
  name: "Feishu",
  description: "",
  version: "1",
  interface: {},
  config_schema: {},
  auth_methods: ["feishu"],
  oauth_supported: false,
};

afterEach(() => vi.unstubAllGlobals());

function ConnectorRouteFixture() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <output aria-label="Current route">{location.pathname}</output>
      <button onClick={() => void navigate(-1)}>Previous page</button>
      <button onClick={() => void navigate("/connectors/new-resource")}>New resource link</button>
      <Routes>
        <Route path="/resources" element={<HubPage />} />
        <Route path="/before" element={<p>Previous destination</p>} />
        <Route path="/connectors" element={<AppsPage />} />
        <Route path="/connectors/:resourceId" element={<AppsPage />} />
      </Routes>
    </>
  );
}

describe("Unavailable connector links", () => {
  it("replaces an unavailable URL with the list and retains the existing dismissible message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => Response.json({ items: String(url).endsWith("catalog") ? [definition] : [] })),
    );
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/before", "/connectors/removed"]} initialIndex={1}>
        <ConnectorRouteFixture />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByLabelText("Current route")).toHaveTextContent(/^\/connectors$/));
    expect(screen.getByRole("alert")).toHaveTextContent(t("appNotFound"));
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Previous page" }));
    expect(screen.getByText("Previous destination")).toBeInTheDocument();
  });

  it("waits for a successful resource response before replacing the URL", async () => {
    let release!: () => void;
    const pending = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        if (String(url).endsWith("catalog")) return Response.json({ items: [definition] });
        await pending;
        return Response.json({ items: [] });
      }),
    );
    render(
      <MemoryRouter initialEntries={["/connectors/removed"]}>
        <ConnectorRouteFixture />
      </MemoryRouter>,
    );
    expect(screen.getByLabelText("Current route")).toHaveTextContent("/connectors/removed");
    expect(screen.queryByText(t("appNotFound"))).not.toBeInTheDocument();
    await act(async () => {
      release();
    });
    await waitFor(() => expect(screen.getByLabelText("Current route")).toHaveTextContent(/^\/connectors$/));
  });

  it("keeps the current URL when resource loading fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) =>
        String(url).endsWith("catalog")
          ? Response.json({ items: [definition] })
          : Response.json(
              { error: { code: "fixture_unavailable", message: "Resource service unavailable" } },
              { status: 503 },
            ),
      ),
    );
    render(
      <MemoryRouter initialEntries={["/connectors/removed"]}>
        <ConnectorRouteFixture />
      </MemoryRouter>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("Resource service unavailable");
    expect(screen.getByLabelText("Current route")).toHaveTextContent("/connectors/removed");
    expect(screen.queryByText(t("appNotFound"))).not.toBeInTheDocument();
  });

  it("loads a newly selected resource before deciding whether its URL is unavailable", async () => {
    const current = {
      installation_id: "new-resource",
      app_id: "feishu",
      name: "New connector",
      enabled: true,
      status: "configured",
      config: {},
      credentials_set: {},
      tools: [],
      bindings: [],
    };
    let resources: (typeof current)[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) =>
        Response.json({ items: String(url).endsWith("catalog") ? [definition] : resources }),
      ),
    );
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/connectors"]}>
        <ConnectorRouteFixture />
      </MemoryRouter>,
    );
    await screen.findByText(t("appEmptyTitle"));
    resources = [current];
    await user.click(screen.getByRole("button", { name: "New resource link" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("New connector");
    expect(screen.getByLabelText("Current route")).toHaveTextContent("/connectors/new-resource");
    expect(screen.queryByText(t("appNotFound"))).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByLabelText("Current route")).toHaveTextContent(/^\/connectors$/);
  });
});

it.each(["/connectors", "/resources"])("preserves local dialog navigation on %s", async (path) => {
  const resource = {
    installation_id: "local-resource",
    app_id: "feishu",
    name: "Local connector",
    enabled: true,
    status: "configured",
    config: {},
    credentials_set: {},
    tools: [],
    bindings: [],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => Response.json({ items: String(url).endsWith("catalog") ? [definition] : [resource] })),
  );
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={[path]}>
      <ConnectorRouteFixture />
    </MemoryRouter>,
  );
  await user.click(await screen.findByRole("button", { name: /Local connector/ }));
  expect(await screen.findByRole("dialog")).toHaveTextContent("Local connector");
  expect(screen.getByLabelText("Current route")).toHaveTextContent(path);
  await user.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByLabelText("Current route")).toHaveTextContent(path);
});

it("refreshes cancelled authorization through the production AppsPage", async () => {
  const operation = {
    operation_id: "connect-1",
    session_id: "session-1",
    resource_id: "global-cli",
    resource_revision: 1,
    action: "connect",
    stage: "user_authorization",
    status: "running",
    authorization_url: "https://example.test/authorize",
  };
  let resource = {
    installation_id: "global-cli",
    app_id: "feishu",
    kind: "cli",
    name: "Feishu",
    revision: 1,
    enabled: true,
    status: "connecting",
    config: {},
    tools: [],
    credentials_set: {},
    bindings: [],
    operation,
  };
  const definition = {
    app_id: "feishu",
    kind: "cli",
    name: "Feishu",
    description: "",
    version: "1",
    interface: {},
    config_schema: {},
    auth_methods: ["command"],
    oauth_supported: true,
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "DELETE") {
        resource = {
          ...resource,
          status: "needs_configuration",
          operation: { ...operation, status: "cancelled", authorization_url: "" },
        };
        return Response.json(operation);
      }
      if (String(url).endsWith("/catalog")) return Response.json({ items: [definition] });
      if (String(url).endsWith("/global-cli")) return Response.json(resource);
      return Response.json({ items: [resource] });
    }),
  );
  render(
    <MemoryRouter initialEntries={["/connectors"]}>
      <ConnectorRouteFixture />
    </MemoryRouter>,
  );
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: /Feishu/ }));
  expect(await screen.findByRole("link", { name: "Authorize account" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(await screen.findByRole("button", { name: "Connect Feishu" })).toBeEnabled();
  expect(screen.queryByRole("link", { name: "Authorize account" })).not.toBeInTheDocument();
  expect(screen.queryByText("In progress")).not.toBeInTheDocument();
});
