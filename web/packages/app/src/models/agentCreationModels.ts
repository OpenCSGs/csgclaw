import type { AgentDraft } from "@/models/agents";
import {
  modelIDsForProviderSelection,
  providerNameForProviderID,
  type ModelProviderCatalog,
} from "@/models/modelProviders";

type ModelChoice = { provider_id: string; model_id: string };

function selectAvailable(
  preferred: ModelChoice | null | undefined,
  choices: readonly ModelChoice[],
  autoSelectMissing: boolean,
): ModelChoice | null {
  if (!autoSelectMissing && !preferred?.model_id) return null;
  return (
    choices.find(
      (choice) => choice.provider_id === preferred?.provider_id && choice.model_id === preferred?.model_id,
    ) ??
    choices[0] ??
    null
  );
}

// Resolve defaults against the current catalog, preserving explicit empty choices.
export function agentDraftWithAvailableModels(
  draft: AgentDraft,
  catalog: ModelProviderCatalog | null,
  openCSGAuthenticated = true,
  autoSelectMissing = true,
): AgentDraft {
  const providers = (catalog?.providers ?? []).filter(
    (provider) => provider.status === "connected" && (provider.id !== "opencsg" || openCSGAuthenticated),
  );
  providers.sort((a, b) => Number(b.id === "opencsg") - Number(a.id === "opencsg"));
  const text = providers.flatMap((provider) =>
    modelIDsForProviderSelection(
      provider.id,
      provider.models.filter(
        (model) => !provider.imageModels?.includes(model) && !provider.videoModels?.includes(model),
      ),
    ).map((model) => ({ provider_id: provider.id, model_id: model })),
  );
  const mediaChoices = (kind: "imageModels" | "videoModels") =>
    providers.flatMap((provider) =>
      (provider[kind] ?? []).map((model) => ({ provider_id: provider.id, model_id: model })),
    );
  const selected = selectAvailable(
    { provider_id: draft.model_provider_id || "", model_id: draft.model_id || "" },
    text,
    autoSelectMissing,
  );
  return {
    ...draft,
    ...(selected
      ? {
          provider: providerNameForProviderID(selected.provider_id),
          model_provider_id: selected.provider_id,
          model_id: selected.model_id,
        }
      : { model_id: "", model_provider_id: "" }),
    image_generation: selectAvailable(draft.image_generation, mediaChoices("imageModels"), autoSelectMissing),
    video_generation: selectAvailable(draft.video_generation, mediaChoices("videoModels"), autoSelectMissing),
  };
}
