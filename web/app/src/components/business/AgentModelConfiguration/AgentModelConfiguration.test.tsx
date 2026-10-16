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
