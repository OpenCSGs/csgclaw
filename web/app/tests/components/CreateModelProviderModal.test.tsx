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
    id: "custom",
    brand: "custom",
    brand_name: "Custom",
    name: "Custom API",
    avatar: "model-providers/custom.svg",
    aliases: [],
    api_protocol: "chat_completions",
    docs_url: "",
    regions: [{ id: "custom", name: "Custom", base_url: "", key_url: "" }],
    models: [],
  },
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

it("groups services by brand and connects with a display name and Key", async () => {
  const { user, onCreate } = await setup();
  expect(screen.getByRole("textbox", { name: t("profileBaseURL") })).toBeVisible();
  expect(screen.getByRole("link", { name: /Get Key/ })).toHaveAttribute("href", "https://cn.example.test/keys");
  expect(screen.getByLabelText(t("profileAPIKey"))).toHaveAttribute("type", "password");
  await user.clear(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }));
  await user.type(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }), "Test connection");
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
      display_name: "Test connection",
    }),
  );
});

it("clears credentials and transport overrides when changing service", async () => {
  const { user, onCreate } = await setup();
  await user.type(screen.getByLabelText(t("profileAPIKey")), "api-key");
  await user.click(screen.getByRole("button", { name: /Advanced settings/ }));
  await user.clear(screen.getByRole("textbox", { name: t("profileBaseURL") }));
  await user.type(screen.getByRole("textbox", { name: t("profileBaseURL") }), "https://override.test/v1");
  await user.click(screen.getByRole("combobox", { name: "Service type" }));
  await user.click(screen.getByRole("option", { name: "Test Coding Plan" }));
  expect(screen.getByLabelText(t("profileAPIKey"))).toHaveValue("");
  expect(screen.getByRole("textbox", { name: t("modelProviderDisplayName") })).toHaveValue("Test Coding Plan");
  expect(screen.getByRole("textbox", { name: t("profileBaseURL") })).toHaveValue("https://plan.example.test/v1");
  await user.clear(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }));
  await user.type(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }), "Test plan");
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
  await user.clear(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }));
  await user.type(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }), "Global connection");
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
  await user.clear(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }));
  await user.type(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }), "Used");
  expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
  await user.clear(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }));
  await user.type(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }), "Unique");
  await user.type(screen.getByLabelText(t("profileHeaders")), "invalid");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(screen.getByRole("alert")).toHaveTextContent(t("modelConnectHeadersInvalid"));
  expect(screen.getByLabelText(t("profileAPIKey"))).toHaveValue("test-key");
  expect(onCreate).not.toHaveBeenCalled();
});

it("keeps connection failures in the form and allows retry", async () => {
  const { user } = await setup({ error: "Wrong Key for this region" });
  expect(screen.getByRole("alert")).toHaveTextContent("Wrong Key for this region");
  await user.clear(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }));
  await user.type(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }), "Retry connection");
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

it.each(["test-api", "custom"])(
  "keeps required connection fields visible with optional settings collapsed for %s",
  async (initialPreset) => {
    const onCreate = vi.fn();
    render(
      <CreateModelProviderModal
        busy={false}
        initialPreset={initialPreset}
        onClose={vi.fn()}
        onCreate={onCreate}
        t={t}
      />,
    );
    const user = userEvent.setup();
    const toggle = await screen.findByRole("button", { name: /Advanced settings/ });
    const name = screen.getByRole("textbox", { name: t("modelProviderDisplayName") });
    const service = screen.getByRole("combobox", { name: t("modelConnectService") });
    const protocol = screen.getByRole("combobox", { name: t("modelConnectProtocol") });
    const baseURL = screen.getByRole("textbox", { name: t("profileBaseURL") });
    const key = screen.getByLabelText(t("profileAPIKey"));
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(baseURL).toBeVisible();
    expect(baseURL).toBeRequired();
    expect(protocol).toBeVisible();
    expect(protocol).toHaveAttribute("aria-required", "true");
    for (const [before, after] of [
      [name, service],
      [service, protocol],
      [protocol, baseURL],
      [baseURL, key],
    ]) {
      expect(before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }
    expect(screen.queryByRole("textbox", { name: t("profileHeaders") })).not.toBeInTheDocument();
    await user.clear(baseURL);
    await user.type(baseURL, "https://custom.example.test/v1");
    await user.clear(name);
    await user.type(name, "My model service");
    await user.type(key, "test-key");
    await user.click(toggle);
    await user.click(screen.getByRole("textbox", { name: t("profileHeaders") }));
    await user.paste('{"X-Project":"test"}');
    await user.click(toggle);
    expect(screen.queryByRole("textbox", { name: t("profileHeaders") })).not.toBeInTheDocument();
    expect(baseURL).toBeVisible();
    expect(protocol).toBeVisible();
    await user.click(toggle);
    expect(screen.getByRole("textbox", { name: t("profileHeaders") })).toHaveValue('{"X-Project":"test"}');
    await user.click(toggle);
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        preset: initialPreset,
        base_url: "https://custom.example.test/v1",
        api_protocol: "chat_completions",
        display_name: "My model service",
        api_key: "test-key",
        headers: { "X-Project": "test" },
      }),
    );
  },
);

it.each(["test-api", "test-plan", "custom"])("requires display name first in the %s form", async (initialPreset) => {
  render(
    <CreateModelProviderModal busy={false} initialPreset={initialPreset} onClose={vi.fn()} onCreate={vi.fn()} t={t} />,
  );
  const user = userEvent.setup();
  const name = await screen.findByRole("textbox", { name: t("modelProviderDisplayName") });
  const service = screen.getByRole("combobox", { name: t("modelConnectService") });
  expect(name).toBeRequired();
  expect(name.compareDocumentPosition(service) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(screen.getByLabelText(t("profileAPIKey"))).toBeRequired();
  expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
  await user.type(screen.getByLabelText(t("profileAPIKey")), "test-key");
  if (initialPreset === "custom") {
    expect(screen.getByRole("textbox", { name: t("profileBaseURL") })).toBeRequired();
    await user.type(screen.getByRole("textbox", { name: t("profileBaseURL") }), "https://custom.example.test/v1");
  }
  await user.clear(name);
  await user.type(name, "   ");
  expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
  await user.clear(name);
  await user.type(name, "My provider");
  expect(screen.getByRole("button", { name: "Connect" })).toBeEnabled();
  expect(name).toBeVisible();
});

it("changes protocol and connects without opening advanced settings", async () => {
  const { user, onCreate } = await setup();
  await user.clear(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }));
  await user.type(screen.getByRole("textbox", { name: t("modelProviderDisplayName") }), "Responses service");
  await user.type(screen.getByLabelText(t("profileAPIKey")), "test-key");
  await user.click(screen.getByRole("combobox", { name: t("modelConnectProtocol") }));
  await user.click(screen.getByRole("option", { name: "OpenAI Responses" }));
  expect(screen.getByRole("button", { name: /Advanced settings/ })).toHaveAttribute("aria-expanded", "false");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  expect(onCreate).toHaveBeenCalledWith(
    expect.objectContaining({ api_protocol: "responses", base_url: "https://cn.example.test/v1" }),
  );
});

it.each(["test-api", "test-plan", "custom"])(
  "prefills and submits the default name for %s without typing it",
  async (initialPreset) => {
    const onCreate = vi.fn();
    render(
      <CreateModelProviderModal
        busy={false}
        initialPreset={initialPreset}
        onClose={vi.fn()}
        onCreate={onCreate}
        t={t}
      />,
    );
    const user = userEvent.setup();
    const expectedName = presets.find((p) => p.id === initialPreset)!.name;
    expect(await screen.findByRole("textbox", { name: t("modelProviderDisplayName") })).toHaveValue(expectedName);
    const baseURL = screen.getByRole("textbox", { name: t("profileBaseURL") });
    if (initialPreset === "custom") await user.type(baseURL, "https://custom.example.test/v1");
    await user.type(screen.getByLabelText(t("profileAPIKey")), "test-key");
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({ display_name: expectedName }));
  },
);

it("prefills a free numbered name and preserves an explicit name if the catalog refreshes", async () => {
  const catalog = normalizeModelProviderCatalog({
    providers: [
      { id: "used", display_name: "test api", models: [] },
      { id: "used-2", display_name: "Test API 2", models: [] },
    ],
  });
  const props = { busy: false, initialPreset: "test-api", onClose: vi.fn(), onCreate: vi.fn(), t };
  const view = render(<CreateModelProviderModal {...props} modelProviders={catalog} />);
  const user = userEvent.setup();
  const name = await screen.findByRole("textbox", { name: t("modelProviderDisplayName") });
  expect(name).toHaveValue("Test API 3");
  await user.clear(name);
  await user.type(name, "My connection");
  view.rerender(
    <CreateModelProviderModal {...props} modelProviders={normalizeModelProviderCatalog({ providers: [] })} />,
  );
  expect(name).toHaveValue("My connection");
});

it("closes a pristine prefilled form without asking to discard changes", async () => {
  const onClose = vi.fn();
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  try {
    render(
      <CreateModelProviderModal busy={false} initialPreset="test-api" onClose={onClose} onCreate={vi.fn()} t={t} />,
    );
    expect(await screen.findByRole("textbox", { name: t("modelProviderDisplayName") })).toHaveValue("Test API");
    await userEvent.setup().click(screen.getByRole("button", { name: "Cancel" }));
    expect(confirm).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledOnce();
  } finally {
    confirm.mockRestore();
  }
});
