import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { HubPage } from "@/pages/HubPage/HubPage";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { createTranslator } from "@/shared/i18n";
import type { HubTemplate } from "@/models/hubWorkspace";

const controllerMock = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/workspace", () => ({ useWorkspaceControllerContext: controllerMock }));
afterEach(() => vi.unstubAllGlobals());

it("keeps the template library detail inline when switching versions", async () => {
  const user = userEvent.setup();
  const template: HubTemplate = {
    id: "local.Reviewer",
    name: "Reviewer",
    uid: "identity",
    revision: 2,
    latest_revision: 2,
    runtime_kind: "codex",
    source: { kind: "local" },
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(workspaceQueryKeys.hubTemplates(), [template]);
  const t = createTranslator("en");
  controllerMock.mockImplementation(function useController() {
    const { data } = useQuery({
      queryKey: workspaceQueryKeys.hubTemplate(template.id),
      initialData: template,
      queryFn: async () => template,
      enabled: false,
    });
    return {
      ready: true,
      hubViewProps: {
        locale: "en",
        t,
        hub: {
          detailPaneProps: {
            loaded: true,
            skills: [],
            mcpServers: [],
            remoteMCPServers: [],
            workspaceEntries: [],
            selectedResourceType: "template",
            selectedTemplate: data,
            selectedTemplateId: data.id,
            templates: [data],
          },
        },
      },
    };
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/restore")) return Response.json({ ...template, revision: 1 });
      if (url.endsWith("/versions"))
        return Response.json([
          { revision: 1, created_at: "2026-10-29T00:00:00Z" },
          { revision: 2, created_at: "2026-10-29T01:00:00Z" },
        ]);
      return Response.json({ kind: "dir", entries: [] });
    }),
  );
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <HubPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  const details = view.container.querySelector("details")!;
  details.open = true;
  fireEvent(details, new Event("toggle"));
  await user.click(await screen.findByRole("button", { name: "V1" }));
  await user.click(screen.getByRole("button", { name: "Switch version" }));
  await user.click(screen.getByRole("button", { name: "Confirm switch to V1" }));
  expect((await screen.findAllByText("Active V1")).length).toBeGreaterThan(0);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Reviewer" })).toBeInTheDocument();
});
