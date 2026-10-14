import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { KnowledgeBasesSection } from "@/pages/SettingsPage/sections/KnowledgeBasesSection";
import { emptyAuthStatus } from "@/models/auth";
import { createQueryWrapper } from "../helpers/queryClient";

const fetchConfig = vi.hoisted(() => vi.fn());
vi.mock("@/api/knowledgeBases", () => ({ fetchRemoteKnowledgeBaseMCPConfig: fetchConfig }));

function setup(connected = false) {
  const onCreateMCPInPlace = vi.fn().mockResolvedValue(true);
  const onUpdateMCP = vi.fn().mockResolvedValue(true);
  const item = {
    id: "kb-1",
    contentID: "content-1",
    name: "Product handbook",
    availability: "available",
    configuredMCPName: connected ? "persisted-kb-name" : "",
  };
  const controller = {
    t: (key: string) => key,
    sidebarProps: { authStatus: { ...emptyAuthStatus(), authenticated: true } },
    hubViewProps: {
      hub: {
        mcpServers: [],
        detailPaneProps: {
          onCreateMCPInPlace,
          onUpdateMCP,
          knowledgeBases: { items: connected ? [item] : [], discoveryItems: [item] },
        },
      },
    },
  } as unknown as WorkspaceController;
  const { client: queryClient } = createQueryWrapper();
  render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <WorkspaceControllerProvider controller={controller}>
          <KnowledgeBasesSection />
        </WorkspaceControllerProvider>
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return { onCreateMCPInPlace, onUpdateMCP, user: userEvent.setup() };
}

beforeEach(() => {
  fetchConfig
    .mockReset()
    .mockResolvedValue({ name: "remote-display-name", config: { url: "https://example.com/mcp" } });
});

it("updates a connected knowledge base using its persisted identity", async () => {
  const { user, onCreateMCPInPlace, onUpdateMCP } = setup(true);
  await user.click(screen.getByRole("tab", { name: "iaKnowledgeTabMarket" }));
  await user.click(screen.getByRole("button", { name: "iaKnowledgeUpdate" }));
  expect(onUpdateMCP).toHaveBeenCalledWith("persisted-kb-name", {
    name: "remote-display-name",
    config: { url: "https://example.com/mcp" },
  });
  expect(onCreateMCPInPlace).not.toHaveBeenCalled();
  expect(screen.getByRole("tab", { name: "iaKnowledgeTabInstalled" })).toHaveAttribute("aria-selected", "true");
});

it("creates an unconnected knowledge base", async () => {
  const { user, onCreateMCPInPlace, onUpdateMCP } = setup();
  await user.click(screen.getByRole("tab", { name: "iaKnowledgeTabMarket" }));
  await user.click(screen.getByRole("button", { name: "iaKnowledgeConnect" }));
  expect(onCreateMCPInPlace).toHaveBeenCalledOnce();
  expect(onUpdateMCP).not.toHaveBeenCalled();
});

it("shows a failed update inside the open detail dialog and permits retry", async () => {
  const { user, onUpdateMCP } = setup(true);
  onUpdateMCP.mockResolvedValueOnce(false);
  await user.click(screen.getByRole("tab", { name: "iaKnowledgeTabMarket" }));
  await user.click(screen.getByRole("button", { name: /Product handbook/ }));
  const dialog = screen.getByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "iaKnowledgeUpdate" }));
  expect(within(dialog).getByRole("alert")).toHaveTextContent("resourcesKnowledgeBaseConfigFailed");
  await user.click(within(dialog).getByRole("button", { name: "iaKnowledgeUpdate" }));
  expect(onUpdateMCP).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
