import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createTranslator } from "@/shared/i18n";
import { AgentModelFields } from "./AgentModelFields";
import { ModelPicker } from "./ModelPicker";

const t = createTranslator("en");
const providers = [
  { id: "custom", value: "custom", displayName: "Custom", avatar: "", models: ["first", "second"] },
  { id: "opencsg", value: "opencsg", displayName: "OpenCSG", avatar: "", models: ["cloud-model"] },
];

test("provider changes choose its first model and OpenCSG authentication can block the change", async () => {
  const user = userEvent.setup();
  const onPatch = vi.fn();
  const authenticate = vi.fn().mockReturnValue(false);
  render(
    <AgentModelFields
      providerOptions={providers}
      providerID=""
      modelID=""
      models={[]}
      onPatch={onPatch}
      onRequireOpenCSGAuth={authenticate}
      t={t}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: t("profileModelProvider") });
  await user.click(trigger);
  await user.click(screen.getByRole("option", { name: /OpenCSG/ }));
  expect(authenticate).toHaveBeenCalledTimes(1);
  expect(onPatch).not.toHaveBeenCalled();
  await user.click(trigger);
  await user.click(screen.getByRole("option", { name: /Custom/ }));
  expect(onPatch).toHaveBeenCalledWith({ provider: "api", model_provider_id: "custom", model_id: "first" });
});

test("a removed saved model is retained with an explanation and cannot be selected again", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(
    <ModelPicker value="removed" models={["current", "current"]} placeholder="Choose" onChange={onChange} t={t} />,
  );
  expect(screen.getByRole("combobox")).toHaveTextContent("removed");
  await user.click(screen.getByRole("combobox"));
  const saved = screen.getByRole("option", { name: /removed/ });
  expect(saved).toHaveAttribute("aria-disabled", "true");
  expect(saved).toHaveTextContent(t("profileModelCurrentSelectionRetained"));
  await user.click(saved);
  expect(onChange).not.toHaveBeenCalled();
  expect(screen.getAllByRole("option", { name: "current" })).toHaveLength(1);
  expect(screen.queryByRole("option", { name: "Choose" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: "current" }));
  expect(onChange).toHaveBeenCalledWith("current");
});
