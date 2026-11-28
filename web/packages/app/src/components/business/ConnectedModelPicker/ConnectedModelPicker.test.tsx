import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createTranslator } from "@/shared/i18n";
import { ConnectedModelPicker } from "./ConnectedModelPicker";
const t = createTranslator("en");

it("pins the three recommended Flash models and keeps the current OpenCSG model selectable", async () => {
  const user = userEvent.setup();
  const provider = {
    id: "opencsg",
    value: "opencsg",
    displayName: "OpenCSG",
    avatar: "",
    models: ["auto", "legacy-model", "glm-5.3-flash", "qwen3.8-flash", "deepseek-v4.1-flash-1017"],
  };
  render(
    <ConnectedModelPicker
      providers={[provider]}
      providerID="opencsg"
      modelID="legacy-model"
      onChoose={vi.fn()}
      t={t}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: "Model" });
  expect(trigger).toHaveTextContent("legacy-model");
  await user.click(trigger);
  const options = screen.getAllByRole("option");
  expect(options).toHaveLength(5);
  expect(options[0]).toHaveTextContent(/qwen3\.8-flash\s*\(Recommended\)/);
  expect(options[1]).toHaveTextContent(/deepseek-v4\.1-flash-1017\s*\(Recommended\)/);
  expect(options[2]).toHaveTextContent(/glm-5\.3-flash\s*\(Recommended\)/);
  expect(screen.getByRole("option", { name: /legacy-model/ })).toBeVisible();
});
const providers = [
  {
    id: "api",
    value: "api",
    displayName: "Team API",
    brandName: "智谱 GLM",
    serviceName: "GLM API",
    avatar: "model-providers/zhipu.svg",
    models: ["shared-model"],
  },
  {
    id: "plan",
    value: "plan",
    displayName: "Personal",
    brandName: "智谱 GLM",
    serviceName: "GLM Coding Plan",
    avatar: "model-providers/zhipu.svg",
    models: ["shared-model"],
  },
];
it("distinguishes identical model IDs by service and connection and searches both", async () => {
  const user = userEvent.setup(),
    choose = vi.fn();
  render(
    <ConnectedModelPicker providers={providers} providerID="api" modelID="shared-model" onChoose={choose} t={t} />,
  );
  await user.click(screen.getByRole("combobox", { name: "Model" }));
  expect(screen.getAllByRole("option")).toHaveLength(2);
  expect(screen.getByRole("group", { name: "智谱 GLM" })).toContainElement(screen.getAllByRole("option")[0]);
  expect(screen.getByRole("group", { name: "智谱 GLM" })).toContainElement(screen.getAllByRole("option")[1]);
  await user.type(screen.getByRole("combobox", { name: "Search models" }), "Coding");
  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getAllByRole("group", { name: "智谱 GLM" })).toHaveLength(1);
  await user.click(screen.getByRole("option", { name: /GLM Coding Plan/ }));
  expect(choose).toHaveBeenCalledWith("plan", "shared-model");
});
it("retains the saved model when a session update fails", async () => {
  const user = userEvent.setup(),
    choose = vi.fn().mockRejectedValue(new Error("Conversation is busy"));
  render(
    <ConnectedModelPicker providers={providers} providerID="api" modelID="shared-model" onChoose={choose} t={t} />,
  );
  await user.click(screen.getByRole("combobox", { name: "Model" }));
  await user.click(screen.getByRole("option", { name: /GLM Coding Plan/ }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Conversation is busy");
  expect(screen.getByRole("combobox", { name: "Model" })).toHaveTextContent("Team API");
});
it("shows connection names when brand, service and model are identical", async () => {
  const user = userEvent.setup();
  const choose = vi.fn();
  render(
    <ConnectedModelPicker
      providers={providers.map((provider) => ({ ...provider, serviceName: "GLM API" }))}
      providerID="api"
      modelID="shared-model"
      onChoose={choose}
      t={t}
    />,
  );
  await user.click(screen.getByRole("combobox", { name: "Model" }));
  expect(screen.getByRole("option", { name: /shared-model.*Team API/ })).toBeVisible();
  await user.click(screen.getByRole("option", { name: /shared-model.*Personal/ }));
  expect(choose).toHaveBeenCalledWith("plan", "shared-model");
});
it("blocks changes while the caller is working", async () => {
  render(
    <ConnectedModelPicker
      providers={providers}
      providerID="api"
      modelID="shared-model"
      disabled
      onChoose={vi.fn()}
      t={t}
    />,
  );
  expect(screen.getByRole("combobox", { name: "Model" })).toBeDisabled();
});

it("shows the followed model provider icon for the follow-agent option and selected value", async () => {
  const user = userEvent.setup();
  render(
    <ConnectedModelPicker
      providers={providers}
      providerID="api"
      modelID="shared-model"
      followDefault
      onChoose={vi.fn()}
      onFollowDefault={vi.fn()}
      t={t}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: "Model" });
  expect(trigger.querySelector("img.model-option-avatar")).toHaveAttribute("src", "model-providers/zhipu.svg");
  await user.click(trigger);
  expect(
    screen.getByRole("option", { name: /Follow the agent/ }).querySelector("img.model-option-avatar"),
  ).toHaveAttribute("src", "model-providers/zhipu.svg");
});

it("puts each account balance on its group header only, including after filtering", async () => {
  const user = userEvent.setup();
  render(
    <ConnectedModelPicker
      providers={providers.map((provider, index) => ({
        ...provider,
        models: ["shared-model", "search-model"],
        balance: { kind: "balance", currency: "CNY", remaining: index ? 10 : 0 },
      }))}
      providerID="api"
      modelID="shared-model"
      onChoose={vi.fn()}
      t={t}
    />,
  );

  await user.click(screen.getByRole("combobox", { name: "Model" }));
  const team = screen.getByRole("group", { name: /Team API/ });
  const personal = screen.getByRole("group", { name: /Personal/ });
  expect(team).toContainElement(screen.getByText("¥0.00"));
  expect(personal).toContainElement(screen.getByText("¥10.00"));
  for (const option of screen.getAllByRole("option")) expect(option).not.toHaveTextContent("¥");

  await user.type(screen.getByRole("combobox", { name: "Search models" }), "search-model");
  expect(screen.getAllByRole("option")).toHaveLength(2);
  expect(screen.getAllByText("¥0.00")).toHaveLength(1);
  expect(screen.getAllByText("¥10.00")).toHaveLength(1);
});
