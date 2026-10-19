import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fetchModelProviderPresets } from "@/api/modelProviders";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";
import { ModelServiceSection } from "@/pages/SettingsPage/sections/ModelServiceSection";
import { createTranslator } from "@/shared/i18n";

vi.mock("@/api/modelProviders", () => ({ fetchModelProviderPresets: vi.fn() }));

it("offers provider creation through the directory without a duplicate settings header action", async () => {
  const t = createTranslator("en");
  const onCreateModelProvider = vi.fn();
  vi.mocked(fetchModelProviderPresets).mockResolvedValue({
    presets: ["openai", "custom"].map((id) => ({
      id,
      brand: id,
      brand_name: id === "openai" ? "OpenAI" : "Custom",
      name: id === "openai" ? "OpenAI API" : "Custom API",
      avatar: "model-providers/custom.svg",
      aliases: [],
      api_protocol: "chat_completions",
      docs_url: "",
      regions: [],
      models: [],
    })),
  });
  render(
    <WorkspaceControllerProvider
      controller={
        {
          ready: true,
          activePane: { type: "settings", id: "" },
          modelProviders: normalizeModelProviderCatalog({
            providers: [{ id: "deepseek", display_name: "DeepSeek", models: [] }],
          }),
          sidebarProps: { onCreateModelProvider },
          refreshWorkspaceModelProviders: vi.fn(),
          t,
        } as never
      }
    >
      <ModelServiceSection />
    </WorkspaceControllerProvider>,
  );
  const heading = screen.getByRole("heading", { name: /Model services/ });
  expect(within(heading.parentElement!.parentElement!).queryByRole("button")).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: /OpenAI API/ }));
  expect(onCreateModelProvider).toHaveBeenLastCalledWith({ preset: "openai" });
  await user.click(screen.getByRole("button", { name: /Custom API/ }));
  expect(onCreateModelProvider).toHaveBeenLastCalledWith({ preset: "custom" });
});
