import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useDescriptionAccess } from "@/hooks/useDescriptionAccess";
import { interpretHubTemplate } from "@/api/hub";
import type { HubTemplate } from "@/models/hubWorkspace";
import { TemplateProfilePanel } from "./TemplateProfilePanel";

vi.mock("@/hooks/useDescriptionAccess", () => ({
  useDescriptionAccess: vi.fn(() => ({ status: "allowed", retry: vi.fn() })),
}));
vi.mock("@/api/hub", () => ({ interpretHubTemplate: vi.fn() }));

const template: HubTemplate = { id: "Reviewer", name: "Reviewer", description: "**原说明**" };
const t = (key: string) => key;

it.each([
  { id: "local.Reviewer", source: { kind: "local" } },
  { id: "alice/Reviewer", source: { kind: "remote" } },
])("shows a read-only introduction for $id", (source) => {
  render(
    <TemplateProfilePanel selectedTemplate={{ ...template, ...source }} templateUsername="alice" t={t} locale="zh" />,
  );
  const card = within(screen.getByRole("region", { name: "templateIntroduction" }));
  expect(card.getByText("原说明").tagName).toBe("STRONG");
  expect(card.queryByRole("button")).not.toBeInTheDocument();
  expect(card.queryByRole("textbox")).not.toBeInTheDocument();
});

it("shows an official template interpretation in its introduction card", async () => {
  const user = userEvent.setup();
  vi.mocked(interpretHubTemplate).mockResolvedValue({ description: "- **文档审阅**" });
  render(
    <TemplateProfilePanel
      selectedTemplate={{ ...template, id: "agentichub/Reviewer", source: { kind: "remote" } }}
      t={t}
      locale="zh"
    />,
  );
  const card = within(screen.getByRole("region", { name: "templateIntroduction" }));
  await user.click(card.getByRole("button", { name: "templateInterpret" }));
  expect(await card.findByRole("listitem")).toHaveTextContent("文档审阅");
  expect(card.getByText("原说明")).toBeVisible();
  expect(interpretHubTemplate).toHaveBeenCalledWith("agentichub/Reviewer", "zh", expect.any(AbortSignal));
});

it("explains when the Manager model is unavailable and allows retry", async () => {
  const user = userEvent.setup();
  vi.mocked(interpretHubTemplate).mockRejectedValueOnce({ code: "template_interpretation_model_unavailable" });
  render(
    <TemplateProfilePanel
      selectedTemplate={{ ...template, id: "agentichub/Reviewer", source: { kind: "remote" } }}
      t={t}
      locale="zh"
    />,
  );
  const card = within(screen.getByRole("region", { name: "templateIntroduction" }));
  const interpret = card.getByRole("button", { name: "templateInterpret" });
  await user.click(interpret);
  expect(await card.findByRole("alert")).toHaveTextContent("templateInterpretationModelUnavailable");
  expect(card.getByText("原说明")).toBeVisible();
  expect(interpret).toBeEnabled();
});

it("keeps the official template readable when AI access is unavailable", () => {
  vi.mocked(useDescriptionAccess).mockReturnValue({ status: "denied", retry: vi.fn() });
  try {
    const { container } = render(
      <TemplateProfilePanel
        selectedTemplate={{
          ...template,
          id: "agentichub/Reviewer",
          source: { kind: "remote" },
          description:
            "## 内容\n\n- **审阅**\n\n[参考](https://example.com/docs)\n\n![示例](https://example.com/image.png)\n\n<video src='https://example.com/video.mp4'></video>\n\n<span style='background-image:url(https://example.com/image.png)'>说明</span>",
        }}
        t={t}
        locale="zh"
      />,
    );
    const card = within(screen.getByRole("region", { name: "templateIntroduction" }));
    expect(card.getByText("descriptionAccessUnavailable")).toBeVisible();
    expect(card.queryByRole("button", { name: "templateInterpret" })).not.toBeInTheDocument();
    expect(card.getByRole("heading", { name: "内容" })).toBeVisible();
    expect(card.getByRole("listitem")).toHaveTextContent("审阅");
    expect(card.getByRole("link", { name: "参考" })).toHaveAttribute("href", "https://example.com/docs");
    expect(card.getByRole("link", { name: "参考" })).toHaveAttribute("target", "_blank");
    expect(card.getByRole("link", { name: "参考" })).toHaveAttribute("rel", "noopener noreferrer");
    expect(container.querySelector("img, video, [style*='background-image']")).toBeNull();
  } finally {
    vi.mocked(useDescriptionAccess).mockReturnValue({ status: "allowed", retry: vi.fn() });
  }
});

it("offers retry when checking AI access fails", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  vi.mocked(useDescriptionAccess).mockReturnValue({ status: "error", retry });
  try {
    render(
      <TemplateProfilePanel
        selectedTemplate={{ ...template, id: "agentichub/Reviewer", source: { kind: "remote" } }}
        t={t}
        locale="zh"
      />,
    );
    const card = within(screen.getByRole("region", { name: "templateIntroduction" }));
    expect(card.getByRole("alert")).toHaveTextContent("descriptionAccessCheckFailed");
    expect(card.queryByText("descriptionAccessUnavailable")).not.toBeInTheDocument();
    await user.click(card.getByRole("button", { name: "retry" }));
    expect(retry).toHaveBeenCalledOnce();
  } finally {
    vi.mocked(useDescriptionAccess).mockReturnValue({ status: "allowed", retry: vi.fn() });
  }
});
