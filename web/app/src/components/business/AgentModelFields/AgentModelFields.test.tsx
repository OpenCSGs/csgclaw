import { useState } from "react";
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

test("model choices follow the selected provider when switching in both directions", async () => {
  const user = userEvent.setup();
  const onPatch = vi.fn();
  function Harness() {
    const [selection, setSelection] = useState({ model_provider_id: "custom", model_id: "first" });
    return (
      <AgentModelFields
        providerOptions={providers}
        providerID={selection.model_provider_id}
        modelID={selection.model_id}
        models={providers.find((provider) => provider.id === selection.model_provider_id)?.models ?? []}
        onPatch={(patch) => {
          onPatch(patch);
          setSelection((current) => ({ ...current, ...patch }));
        }}
        onRequireOpenCSGAuth={() => true}
        t={t}
      />
    );
  }
  render(<Harness />);
  const model = screen.getByRole("combobox", { name: t("profileModel") });
  const provider = screen.getByRole("combobox", { name: t("profileModelProvider") });
  await user.click(model);
  expect(screen.getAllByRole("option")).toHaveLength(2);
  expect(screen.queryByRole("option", { name: /cloud-model/ })).not.toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: /second/ }));
  expect(onPatch).toHaveBeenLastCalledWith({ provider: "api", model_provider_id: "custom", model_id: "second" });
  await user.click(provider);
  await user.click(screen.getByRole("option", { name: /OpenCSG/ }));
  expect(model).toHaveTextContent("cloud-model");
  await user.click(model);
  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getByRole("option", { name: /cloud-model/ })).toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /first|second/ })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  await user.click(provider);
  await user.click(screen.getByRole("option", { name: /Custom/ }));
  expect(model).toHaveTextContent("first");
  await user.click(model);
  expect(screen.getAllByRole("option")).toHaveLength(2);
  expect(screen.queryByRole("option", { name: /cloud-model/ })).not.toBeInTheDocument();
});

test.each([
  { providerID: "custom", models: [], disabled: false },
  { providerID: "custom", models: ["first"], disabled: true },
  { providerID: "missing", models: ["saved"], disabled: false },
])("disables model selection for an unavailable provider: %j", ({ providerID, models, disabled }) => {
  render(
    <AgentModelFields
      providerOptions={providers.map((provider) => ({ ...provider, disabled: provider.id === providerID && disabled }))}
      providerID={providerID}
      modelID="saved"
      models={models}
      onPatch={vi.fn()}
      onRequireOpenCSGAuth={() => true}
      t={t}
    />,
  );
  expect(screen.getByRole("combobox", { name: t("profileModel") })).toBeDisabled();
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
