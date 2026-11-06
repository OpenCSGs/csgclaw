import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { cloudAgentFixture } from "../../../../../tests/helpers/cloudAgentFixture";
import { AgentPlaza, AgentPlazaMetadata, CloudAgentDetail } from "./AgentPlaza";

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
    render(<CloudAgentDetail agent={agent} t={t} onNewTask={onNewTask} onRemove={onRemove} />);

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

it("shows only the agent display name and product publisher as metadata", () => {
  const agent = { ...cloudAgentFixture[0], publisher: "CSGClaw" };
  render(<AgentPlazaMetadata agent={agent} t={createTranslator("zh")} />);
  expect(screen.getByText(agent.name)).toBeVisible();
  expect(screen.getByText("CSGClaw")).toBeVisible();
  expect(screen.getAllByRole("term")).toHaveLength(2);
  for (const value of [agent.id, agent.relayURL, agent.sourceAgentID])
    expect(screen.queryByText(value)).not.toBeInTheDocument();
});

it("shows a private agent under Mine before it has been added", async () => {
  const agent = { ...cloudAgentFixture[0], visibility: "private" } as const;
  const onAdd = vi.fn();
  const user = userEvent.setup();
  render(<AgentPlaza agents={[agent]} addedIDs={[]} t={createTranslator("zh")} onAdd={onAdd} onOpen={vi.fn()} />);
  expect(screen.queryByRole("heading", { name: agent.name })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "我的" }));
  expect(screen.getByRole("heading", { name: agent.name })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "添加到我的智能体" }));
  expect(onAdd).toHaveBeenCalledWith(agent.id);
});
