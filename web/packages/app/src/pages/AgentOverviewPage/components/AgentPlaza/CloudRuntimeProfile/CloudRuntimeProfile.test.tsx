import type { ComponentProps } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, vi, it, expect } from "vitest";
import { fetchCloudProfile } from "@/api/relay";
import { fetchConversationResourceBatch } from "@/api/agentResources";
import { createQueryWrapper } from "../../../../../../tests/helpers/queryClient";
import { cloudAgentFixture } from "../../../../../../tests/helpers/cloudAgentFixture";
import { createTranslator } from "@/shared/i18n";
import { CloudAgentDetail } from "../AgentPlaza";

vi.mock("@/api/relay", () => ({
  fetchCloudProfile: vi.fn(),
  fetchCloudAgentTemplate: vi.fn(),
  fetchRelayCatalog: vi.fn(),
}));
vi.mock("@/api/agentResources", () => ({ fetchConversationResourceBatch: vi.fn() }));
const profile = {
  id: "worker",
  name: "Runtime worker",
  description: "Live description",
  instructions: "Live instructions",
  runtime: "codex",
  model: "model",
  provider: "opencsg",
};
const agent = { ...cloudAgentFixture[0], publisher: "Agentichub", type: "csgclaw" };
const empty = { items: [], total: 0, list_revision: "1", has_more: false };
function renderProfile(props: Partial<ComponentProps<typeof CloudAgentDetail>> = {}, locale: "zh" | "en" = "zh") {
  return render(
    <CloudAgentDetail
      agent={agent}
      t={createTranslator(locale)}
      onNewTask={vi.fn()}
      onRemove={vi.fn()}
      renderTemplate={() => null}
      {...props}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
}
beforeEach(() => {
  vi.mocked(fetchCloudProfile).mockReset().mockResolvedValue(profile);
  vi.mocked(fetchConversationResourceBatch).mockReset().mockResolvedValue(empty);
});
it("keeps the local profile sections and read-only fields in the default tab", async () => {
  renderProfile();
  expect(screen.getByRole("region", { name: "运行环境" })).toBeVisible();
  expect(screen.getByRole("region", { name: "模型配置" })).toBeVisible();
  expect(await screen.findByDisplayValue("model")).toBeVisible();
  for (const input of screen.getAllByRole("textbox")) {
    expect(input).toHaveAttribute("readonly");
    expect(input).toBeDisabled();
  }
  expect(screen.queryByDisplayValue("Live instructions")).not.toBeInTheDocument();
  expect(fetchConversationResourceBatch).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: /保存|编辑|安装|删除/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
});
it("preserves the full layout and navigation when profile loading fails", async () => {
  vi.mocked(fetchCloudProfile).mockRejectedValue({ code: "cloud_endpoint_unavailable", status: 409 });
  renderProfile();
  expect(await screen.findByRole("alert")).toHaveTextContent("只读接口不可用");
  expect(screen.getByRole("region", { name: "运行环境" })).toBeVisible();
  expect(screen.getByRole("region", { name: "模型配置" })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "运行时" })).toHaveValue("—");
  expect(screen.getByRole("textbox", { name: "模型提供方" })).toHaveValue("—");
  const nav = screen.getByRole("navigation", { name: "配置档页面分区" });
  expect(nav.querySelectorAll("button")).toHaveLength(5);
  expect(fetchConversationResourceBatch).not.toHaveBeenCalled();
  await userEvent.setup().click(screen.getByRole("button", { name: "指令" }));
  expect(screen.getByRole("textbox", { name: "指令" })).toHaveAttribute("readonly");
  expect(screen.getAllByRole("alert")).toHaveLength(1);
});
it("keeps section cards visible during the initial load", () => {
  vi.mocked(fetchCloudProfile).mockReturnValue(new Promise(() => {}));
  renderProfile();
  expect(screen.getByRole("status")).toHaveTextContent("加载中");
  expect(screen.getByRole("region", { name: "运行环境" })).toBeVisible();
  expect(screen.getByRole("region", { name: "模型配置" })).toBeVisible();
});
it("loads only the selected resource tab and keeps instructions separate", async () => {
  vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
    ...empty,
    items: [{ source: "skill", id: "review", name: "Runtime review", type: "skill", enabled: true, available: true }],
    total: 1,
  });
  const onProfileTabChange = vi.fn();
  renderProfile({ onProfileTabChange });
  await screen.findByDisplayValue("model");
  await userEvent.setup().click(screen.getByRole("button", { name: "技能" }));
  expect(await screen.findByText("Runtime review")).toBeVisible();
  expect(onProfileTabChange).toHaveBeenCalledWith("skills");
  expect(vi.mocked(fetchConversationResourceBatch).mock.calls.every((call) => call[1] === "skill")).toBe(true);
  expect(screen.queryByRole("region", { name: "模型配置" })).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "指令" }));
  expect(screen.getByRole("textbox", { name: "指令" })).toHaveValue("Live instructions");
  expect(screen.queryByText("Runtime review")).not.toBeInTheDocument();
});
it("reports a read error once and retries without replacing the profile sections", async () => {
  vi.mocked(fetchCloudProfile)
    .mockRejectedValueOnce({ code: "cloud_resource_access_denied", status: 403 })
    .mockResolvedValue(profile);
  renderProfile({}, "en");
  expect(await screen.findByRole("alert")).toHaveTextContent("You do not have access");
  expect(screen.getByRole("region", { name: "Runtime environment" })).toBeVisible();
  await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
  expect(await screen.findByDisplayValue("model")).toBeVisible();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
it("uses the requested section from navigation rather than resetting to profile", async () => {
  renderProfile({ requestedProfileTab: "knowledge" });
  expect(screen.getByRole("button", { name: "知识库" })).toHaveAttribute("aria-current", "location");
  await screen.findByText(/还没有启用/);
  expect(fetchCloudProfile).not.toHaveBeenCalled();
  expect(fetchConversationResourceBatch).toHaveBeenCalledWith(
    expect.any(String),
    "knowledge_base",
    "",
    { search: "" },
    expect.any(AbortSignal),
  );
});
