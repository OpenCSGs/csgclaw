import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createTranslator } from "@/shared/i18n";
import { ConnectedModelPicker } from "./ConnectedModelPicker";
const t = createTranslator("en");
const providers = [
  { id: "api", value: "api", displayName: "Team API", serviceName: "GLM API", avatar: "", models: ["shared-model"] },
  {
    id: "plan",
    value: "plan",
    displayName: "Personal",
    serviceName: "GLM Coding Plan",
    avatar: "",
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
  await user.type(screen.getByRole("combobox", { name: "Search models" }), "Coding");
  expect(screen.getAllByRole("option")).toHaveLength(1);
  await user.click(screen.getByRole("option", { name: /GLM Coding Plan.*Personal/ }));
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
