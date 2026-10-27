import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { agentToDraft } from "@/models/agents";
import { createTranslator } from "@/shared/i18n";
import { AgentModelConfiguration } from "./AgentModelConfiguration";

const t = createTranslator("en");

test("conversation settings are grouped separately and media changes only patch their own model", async () => {
  const user = userEvent.setup();
  const onPatch = vi.fn();
  const draft = agentToDraft({ id: "worker", name: "Worker", model_id: "chat", model_provider_id: "custom" });
  render(
    <AgentModelConfiguration
      draft={{ ...draft, image_generation: { provider_id: "custom", model_id: "image" } }}
      providerOptions={[
        {
          id: "custom",
          value: "custom",
          displayName: "Custom",
          avatar: "",
          models: ["chat"],
          imageModels: ["image", "image-2"],
          videoModels: ["video"],
        },
      ]}
      providerID="custom"
      modelID="chat"
      models={["chat"]}
      onPatch={onPatch}
      onRequireOpenCSGAuth={() => true}
      t={t}
    />,
  );
  const conversation = within(screen.getByRole("group", { name: t("profileConversationModelGroup") }));
  expect(conversation.getByRole("combobox", { name: t("profileReasoningEffort") })).toBeInTheDocument();
  await user.click(conversation.getByRole("checkbox", { name: t("profileFastMode") }));
  expect(onPatch).toHaveBeenLastCalledWith({ enable_fast_mode: !draft.enable_fast_mode });
  const image = within(screen.getByRole("group", { name: t("profileImageModelGroup") }));
  expect(image.queryByRole("checkbox")).not.toBeInTheDocument();
  await user.click(image.getByRole("combobox", { name: t("profileImageModel") }));
  await user.click(screen.getByRole("option", { name: "image-2" }));
  expect(onPatch).toHaveBeenLastCalledWith({ image_generation: { provider_id: "custom", model_id: "image-2" } });
  const video = within(screen.getByRole("group", { name: t("profileVideoModelGroup") }));
  expect(video.queryByRole("checkbox")).not.toBeInTheDocument();
  expect(video.getByRole("combobox", { name: t("profileVideoModel") })).toBeDisabled();
});

test("conversation reasoning follows the displayed model and changing models clears the old effort", async () => {
  const user = userEvent.setup();
  const onPatch = vi.fn();
  const models = ["gpt-5.6-luna", "gpt-6-astra"];
  const props = {
    draft: { ...agentToDraft({ id: "worker", name: "Worker" }), reasoning_effort: "high" },
    providerID: "codex",
    modelID: models[0],
    models,
    providerOptions: [
      {
        id: "codex",
        value: "codex",
        displayName: "Codex",
        avatar: "",
        models,
        reasoningEfforts: {
          "gpt-5.6-luna": ["none", "low", "medium", "high", "xhigh", "max"],
          "gpt-6-astra": ["low", "medium", "high", "xhigh", "max"],
        },
      },
    ],
    onPatch,
    onRequireOpenCSGAuth: () => true,
    t,
  };
  const { rerender } = render(<AgentModelConfiguration {...props} />);
  await user.click(screen.getByRole("combobox", { name: t("profileReasoningEffort") }));
  expect(screen.queryByRole("option", { name: t("profileReasoningMinimal") })).not.toBeInTheDocument();
  expect(screen.getByRole("option", { name: t("profileReasoningDisabled") })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: t("profileReasoningMax") })).toBeInTheDocument();
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("combobox", { name: t("profileModel") }));
  await user.click(screen.getByRole("option", { name: /^gpt-6-astra/ }));
  expect(onPatch).toHaveBeenLastCalledWith(
    expect.objectContaining({ model_id: "gpt-6-astra", reasoning_effort: "auto" }),
  );
  rerender(<AgentModelConfiguration {...props} modelID="gpt-6-astra" />);
  await user.click(screen.getByRole("combobox", { name: t("profileReasoningEffort") }));
  expect(screen.queryByRole("option", { name: t("profileReasoningDisabled") })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: t("profileReasoningMinimal") })).not.toBeInTheDocument();
});
