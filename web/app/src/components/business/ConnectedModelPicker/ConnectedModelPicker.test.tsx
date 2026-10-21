import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createTranslator } from "@/shared/i18n";
import { ConnectedModelPicker } from "./ConnectedModelPicker";
const t = createTranslator("en");
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
