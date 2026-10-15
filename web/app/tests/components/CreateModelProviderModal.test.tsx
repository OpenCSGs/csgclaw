import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CreateModelProviderModal } from "@/pages/WorkspacePage/components/WorkspaceModals";
import { fetchModelProviderPresets } from "@/api/modelProviders";
import { createTranslator } from "@/shared/i18n";
import type { ModelServicePreset } from "@/models/modelProviderPresets";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";

vi.mock("@/api/modelProviders", () => ({ fetchModelProviderPresets: vi.fn() }));
const t = createTranslator("en");
const presets: ModelServicePreset[] = [
  {
    id: "test-api",
    brand: "test",
    brand_name: "Test AI",
    name: "Test API",
    avatar: "model-providers/kimi.png",
    aliases: ["Moon"],
    api_protocol: "chat_completions",
    docs_url: "https://docs.example.test",
    regions: [
      { id: "cn", name: "China", base_url: "https://cn.example.test/v1", key_url: "https://cn.example.test/keys" },
      {
        id: "global",
        name: "Global",
        base_url: "https://global.example.test/v1",
        key_url: "https://global.example.test/keys",
      },
    ],
    models: [],
  },
  {
    id: "test-plan",
    brand: "test",
    brand_name: "Test AI",
    name: "Test Coding Plan",
    avatar: "model-providers/kimi.png",
    aliases: ["Coding"],
    api_protocol: "responses",
    docs_url: "https://docs.example.test/plan",
    regions: [
      { id: "cn", name: "China", base_url: "https://plan.example.test/v1", key_url: "https://plan.example.test/keys" },
    ],
    models: [],
  },
];
beforeEach(() => {
  vi.mocked(fetchModelProviderPresets).mockResolvedValue({ presets });
});

async function setup(props: Partial<Parameters<typeof CreateModelProviderModal>[0]> = {}) {
  const onCreate = vi.fn();
  const view = render(<CreateModelProviderModal busy={false} onClose={vi.fn()} onCreate={onCreate} t={t} {...props} />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: /Test AI/ }));
  return { ...view, user, onCreate };
}

it("groups services by brand and only requires a Key", async () => {
  const { user, onCreate } = await setup();
  expect(screen.queryByLabelText(t("profileBaseURL"))).not.toBeInTheDocument();
  expect(screen.getByRole("link", { name: /Get Key/ })).toHaveAttribute("href", "https://cn.example.test/keys");
  expect(screen.getByLabelText(t("profileAPIKey"))).toHaveAttribute("type", "password");
  await user.type(screen.getByLabelText(t("profileAPIKey")), "test-key");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(onCreate).toHaveBeenCalledWith(
    expect.objectContaining({
      preset: "test-api",
      region: "cn",
      base_url: "https://cn.example.test/v1",
      api_key: "test-key",
      api_protocol: "chat_completions",
      connect: true,
      display_name: undefined,
    }),
  );
});

it("clears credentials and advanced transport overrides when changing service", async () => {
  const { user, onCreate } = await setup();
  await user.type(screen.getByLabelText(t("profileAPIKey")), "api-key");
  await user.click(screen.getByRole("button", { name: /Advanced settings/ }));
  await user.clear(screen.getByLabelText(t("profileBaseURL")));
  await user.type(screen.getByLabelText(t("profileBaseURL")), "https://override.test/v1");
  await user.click(screen.getByRole("combobox", { name: "Service type" }));
  await user.click(screen.getByRole("option", { name: "Test Coding Plan" }));
  expect(screen.getByLabelText(t("profileAPIKey"))).toHaveValue("");
  expect(screen.getByLabelText(t("profileBaseURL"))).toHaveValue("https://plan.example.test/v1");
  await user.type(screen.getByLabelText(t("profileAPIKey")), "plan-key");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(onCreate).toHaveBeenCalledWith(
    expect.objectContaining({ preset: "test-plan", api_protocol: "responses", api_key: "plan-key" }),
  );
});

it("matches region credentials and clears the previous Key", async () => {
  const { user, onCreate } = await setup();
  await user.type(screen.getByLabelText(t("profileAPIKey")), "cn-key");
  await user.click(screen.getByRole("combobox", { name: "Region" }));
  await user.click(screen.getByRole("option", { name: "Global" }));
  expect(screen.getByLabelText(t("profileAPIKey"))).toHaveValue("");
  expect(screen.getByRole("link", { name: /Get Key/ })).toHaveAttribute("href", "https://global.example.test/keys");
  await user.type(screen.getByLabelText(t("profileAPIKey")), "global-key");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(onCreate).toHaveBeenCalledWith(
    expect.objectContaining({ region: "global", base_url: "https://global.example.test/v1" }),
  );
});

it("validates explicit names and custom headers without discarding the Key", async () => {
  const { user, onCreate } = await setup({
    modelProviders: normalizeModelProviderCatalog({ providers: [{ id: "used", display_name: "Used", models: [] }] }),
  });
  await user.click(screen.getByRole("button", { name: /Advanced settings/ }));
  await user.type(screen.getByLabelText(t("profileAPIKey")), "test-key");
  await user.type(screen.getByLabelText(t("modelProviderDisplayName")), "Used");
  expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
  await user.clear(screen.getByLabelText(t("modelProviderDisplayName")));
  await user.type(screen.getByLabelText(t("profileHeaders")), "invalid");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(screen.getByRole("alert")).toHaveTextContent(t("modelConnectHeadersInvalid"));
  expect(screen.getByLabelText(t("profileAPIKey"))).toHaveValue("test-key");
  expect(onCreate).not.toHaveBeenCalled();
});

it("keeps connection failures in the form and allows retry", async () => {
  const { user } = await setup({ error: "Wrong Key for this region" });
  expect(screen.getByRole("alert")).toHaveTextContent("Wrong Key for this region");
  await user.type(screen.getByLabelText(t("profileAPIKey")), "retry-key");
  expect(screen.getByRole("button", { name: "Connect" })).toBeEnabled();
});

it("Escape closes a service menu without dismissing the connection form", async () => {
  const onClose = vi.fn();
  const { user } = await setup({ onClose });
  await user.click(screen.getByRole("combobox", { name: "Service type" }));
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(onClose).toHaveBeenCalledTimes(1);
});
