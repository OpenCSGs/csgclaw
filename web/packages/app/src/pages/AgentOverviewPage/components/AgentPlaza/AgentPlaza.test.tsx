import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { fetchCloudAgentTemplate, fetchRelayCatalog } from "@/api/relay";
import { createQueryWrapper } from "../../../../../tests/helpers/queryClient";
import { cloudAgentFixture } from "../../../../../tests/helpers/cloudAgentFixture";
import { AgentPlaza, AgentPlazaMetadata, CloudAgentDetail } from "./AgentPlaza";

vi.mock("@/api/relay", () => ({ fetchCloudAgentTemplate: vi.fn(), fetchRelayCatalog: vi.fn() }));

it("passes the authorized template repository to the shared profile renderer", async () => {
  vi.mocked(fetchCloudAgentTemplate).mockResolvedValue({ repo_path: "alice/writing-template" });
  const agent = { ...cloudAgentFixture[0], publisher: "Agentichub", modelID: "instance-model" };
  const renderTemplate = vi.fn((repoPath: string) => <h3>{repoPath}</h3>);
  render(
    <CloudAgentDetail
      agent={agent}
      t={createTranslator("zh")}
      onNewTask={vi.fn()}
      onRemove={vi.fn()}
      renderTemplate={renderTemplate}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
  expect(await screen.findByRole("heading", { name: "alice/writing-template" })).toBeVisible();
  expect(renderTemplate).toHaveBeenCalledWith("alice/writing-template", expect.anything());
  expect(fetchCloudAgentTemplate).toHaveBeenCalledWith(agent, expect.any(AbortSignal));
});

it("uses the catalog template reference without requesting it again", async () => {
  vi.mocked(fetchCloudAgentTemplate).mockClear();
  const agent = { ...cloudAgentFixture[0], publisher: "Agentichub", templateRepoPath: "alice/worker" };
  render(
    <CloudAgentDetail
      agent={agent}
      t={createTranslator("zh")}
      onNewTask={vi.fn()}
      onRemove={vi.fn()}
      renderTemplate={(path) => <h3>{path}</h3>}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
  expect(screen.getByRole("heading", { name: "alice/worker" })).toBeVisible();
  expect(fetchCloudAgentTemplate).not.toHaveBeenCalled();
});

describe("cloud directory availability", () => {
  it.each([
    ["online", "在线"],
    ["offline", "离线"],
  ] as const)("shows only the %s status and allows adding an offline listing", (status, label) => {
    render(
      <AgentPlaza
        agents={[{ ...cloudAgentFixture[0], status }]}
        addedIDs={[]}
        t={createTranslator("zh")}
        onAdd={vi.fn()}
        onOpen={vi.fn()}
      />,
      { wrapper: createQueryWrapper().wrapper },
    );
    expect(screen.getByText(label)).toBeVisible();
    expect(screen.queryByText(status === "online" ? "离线" : "在线")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "添加到我的智能体" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "浏览公共智能体" })).not.toBeInTheDocument();
  });

  it.each([
    ["zh", "我的", "公共", "浏览公共智能体", "搜索公共智能体", "添加到我的智能体"],
    ["en", "Mine", "Public", "Browse public agents", "Search public agents", "Add to My agents"],
  ] as const)(
    "returns to public agents from the %s empty state",
    async (locale, scope, publicScope, label, search, add) => {
      const onAdd = vi.fn();
      const user = userEvent.setup();
      render(
        <AgentPlaza
          agents={[...cloudAgentFixture]}
          addedIDs={[]}
          t={createTranslator(locale)}
          onAdd={onAdd}
          onOpen={vi.fn()}
        />,
        { wrapper: createQueryWrapper().wrapper },
      );
      await user.click(screen.getByRole("button", { name: scope }));
      await user.type(screen.getByRole("textbox", { name: search }), "not found");
      await user.click(screen.getByRole("button", { name: label }));
      expect(screen.getByRole("button", { name: publicScope })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("button", { name: scope })).toHaveAttribute("aria-pressed", "false");
      expect(screen.getByRole("textbox", { name: search })).toHaveValue("");
      expect(screen.getByRole("heading", { name: cloudAgentFixture[0].name })).toBeVisible();
      expect(screen.queryByRole("link")).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: add }));
      expect(onAdd).toHaveBeenCalledWith(cloudAgentFixture[0].id);
    },
  );
});

it.each(["zh", "en"] as const)(
  "keeps cloud profile actions available through the compact %s header",
  async (locale) => {
    const t = createTranslator(locale);
    const agent = { ...cloudAgentFixture[0], description: "Long description ".repeat(30).trim() };
    const onNewTask = vi.fn();
    const onRemove = vi.fn();
    const user = userEvent.setup();
    render(
      <CloudAgentDetail agent={agent} t={t} onNewTask={onNewTask} onRemove={onRemove} renderTemplate={() => null} />,
      { wrapper: createQueryWrapper().wrapper },
    );

    expect(screen.getByRole("heading", { name: agent.name })).toHaveAttribute("title", agent.name);
    expect(screen.getByTitle(agent.description)).toBeVisible();
    expect(screen.queryByRole("button", { name: t("agentPlazaRemove") })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: t("agentStartTask") }));
    expect(onNewTask).toHaveBeenCalledOnce();

    await user.tab();
    expect(screen.getByRole("button", { name: t("agentMoreActions") })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("menuitem", { name: t("agentPlazaRemove") })).toBeVisible();
    await user.keyboard("{Enter}");
    expect(onRemove).toHaveBeenCalledOnce();
  },
);

it("shows the creator and product publisher as metadata", () => {
  const agent = { ...cloudAgentFixture[0], creatorName: "Alice", publisher: "CSGClaw" };
  render(<AgentPlazaMetadata agent={agent} t={createTranslator("zh")} />);
  expect(screen.getByText("Alice")).toBeVisible();
  expect(screen.getByText("CSGClaw")).toBeVisible();
  expect(screen.getAllByRole("term")).toHaveLength(2);
  for (const value of [agent.name, agent.id, agent.relayURL, agent.sourceAgentID])
    expect(screen.queryByText(value)).not.toBeInTheDocument();
});

it("hides the source row when the creator is unavailable", () => {
  render(<AgentPlazaMetadata agent={cloudAgentFixture[0]} t={createTranslator("zh")} />);
  expect(screen.queryByText("来源")).not.toBeInTheDocument();
  expect(screen.getByText("发布端")).toBeVisible();
});

it.each(["private", "public"] as const)(
  "shows an owned %s agent under Mine before it has been added",
  async (visibility) => {
    const agent = { ...cloudAgentFixture[0], ownedByCurrentUser: true, visibility };
    const shared = {
      ...agent,
      id: "shared",
      name: "Shared private agent",
      ownedByCurrentUser: false,
      visibility: "private",
    } as const;
    const onAdd = vi.fn();
    const user = userEvent.setup();
    render(
      <AgentPlaza
        agents={[agent, shared]}
        addedIDs={[shared.id]}
        t={createTranslator("zh")}
        onAdd={onAdd}
        onOpen={vi.fn()}
      />,
      { wrapper: createQueryWrapper().wrapper },
    );
    await user.click(screen.getByRole("button", { name: "我的" }));
    expect(screen.getByRole("heading", { name: agent.name })).toBeVisible();
    expect(screen.queryByRole("heading", { name: shared.name })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "添加到我的智能体" }));
    expect(onAdd).toHaveBeenCalledWith(agent.id);
  },
);

it("probes only the first batch until the user loads more", async () => {
  const agents = Array.from({ length: 14 }, (_, index) => ({
    ...cloudAgentFixture[0],
    id: `agent-${index}`,
    name: `Agent ${index}`,
  }));
  vi.mocked(fetchRelayCatalog).mockResolvedValue([]);
  render(
    <AgentPlaza
      agents={agents}
      addedIDs={[]}
      probeAvailability
      t={createTranslator("en")}
      onAdd={vi.fn()}
      onOpen={vi.fn()}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
  await waitFor(() => expect(fetchRelayCatalog).toHaveBeenCalledTimes(1));
  expect(fetchRelayCatalog).toHaveBeenNthCalledWith(
    1,
    expect.any(AbortSignal),
    undefined,
    agents.slice(0, 12).map((agent) => agent.id),
  );
  expect(screen.queryByRole("heading", { name: "Agent 12" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Load more" }));
  await waitFor(() => expect(fetchRelayCatalog).toHaveBeenCalledTimes(2));
  expect(fetchRelayCatalog).toHaveBeenNthCalledWith(2, expect.any(AbortSignal), undefined, ["agent-12", "agent-13"]);
});
