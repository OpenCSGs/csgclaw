import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fetchAgentProfileDefaults, updateAgentProfileDefaults } from "@/api/agents";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";
import { createTranslator } from "@/shared/i18n";
import { DefaultModelSettings } from "./DefaultModelSettings";

vi.mock("@/api/agents", () => ({ fetchAgentProfileDefaults: vi.fn(), updateAgentProfileDefaults: vi.fn() }));
const t = createTranslator("zh");
const defaults = {
  model_provider_id: "opencsg",
  model_id: "auto",
  image_generation: { provider_id: "opencsg", model_id: "image-first" },
  video_generation: null,
};
const catalog = normalizeModelProviderCatalog({
  providers: [
    {
      id: "opencsg",
      display_name: "OpenCSG",
      status: "connected",
      models: ["chat", "auto", "image-first", "video-first"],
      image_models: ["image-first", "image-second"],
      video_models: ["video-first"],
    },
    { id: "failed", display_name: "Failed", status: "failed", models: ["stale"], image_models: ["stale-image"] },
    { id: "unknown", display_name: "Unknown", status: "unknown", models: ["cached"] },
  ],
});

beforeEach(() => {
  let saved: Awaited<ReturnType<typeof fetchAgentProfileDefaults>> = defaults;
  vi.mocked(fetchAgentProfileDefaults).mockImplementation(async () => saved);
  vi.mocked(updateAgentProfileDefaults).mockImplementation(async ({ kind, provider_id, model_id }) => {
    saved = {
      ...saved,
      ...(kind === "agent"
        ? { model_provider_id: provider_id, model_id }
        : {
            [kind === "image" ? "image_generation" : "video_generation"]: model_id ? { provider_id, model_id } : null,
          }),
    };
    return saved;
  });
});

test("lists connected models by capability and saves an explicit default", async () => {
  const user = userEvent.setup();
  render(<DefaultModelSettings catalog={catalog} t={t} />);
  const agent = screen.getByRole("combobox", { name: "智能体模型" });
  await waitFor(() => expect(agent).toBeEnabled());
  expect(agent).toHaveTextContent("auto");
  await user.click(agent);
  expect(screen.getAllByRole("option")).toHaveLength(3);
  expect(screen.queryByRole("option", { name: /stale|cached|image-first|video-first/ })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("combobox", { name: "图片生成" }));
  expect(screen.getAllByRole("option")).toHaveLength(3);
  await user.click(screen.getByRole("option", { name: /image-second/ }));
  await waitFor(() =>
    expect(updateAgentProfileDefaults).toHaveBeenCalledWith({
      kind: "image",
      provider_id: "opencsg",
      model_id: "image-second",
    }),
  );
  expect(screen.getByRole("combobox", { name: "图片生成" })).toHaveTextContent("image-second");
});

test("shows unspecified for every category when all services are unavailable", async () => {
  vi.mocked(fetchAgentProfileDefaults).mockResolvedValue({});
  const user = userEvent.setup();
  render(
    <DefaultModelSettings
      catalog={normalizeModelProviderCatalog({
        providers: [
          { id: "failed", status: "failed", models: ["stale"], image_models: ["image"], video_models: ["video"] },
        ],
      })}
      t={t}
    />,
  );
  for (const control of screen.getAllByRole("combobox")) {
    await waitFor(() => expect(control).toBeEnabled());
    expect(control).toHaveTextContent("不指定");
    await user.click(control);
    expect(screen.getAllByRole("option")).toHaveLength(1);
    await user.keyboard("{Escape}");
  }
});

test("keeps the previous choice after a failed save", async () => {
  vi.mocked(updateAgentProfileDefaults).mockRejectedValue(new Error("save failed"));
  const user = userEvent.setup();
  render(<DefaultModelSettings catalog={catalog} t={t} />);
  const agent = screen.getByRole("combobox", { name: "智能体模型" });
  await waitFor(() => expect(agent).toBeEnabled());
  await user.click(agent);
  await user.click(screen.getByRole("option", { name: /chat/ }));
  await screen.findByRole("alert");
  expect(agent).toHaveTextContent("auto");
});
