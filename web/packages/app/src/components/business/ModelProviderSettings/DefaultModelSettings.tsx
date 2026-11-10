import { useEffect, useState } from "react";
import { Bot, Image, Video } from "lucide-react";
import { fetchAgentProfileDefaults, updateAgentProfileDefaults } from "@/api/agents";
import { localizeAPIError } from "@/shared/i18n";
import { Button, Field, Select } from "@/components/ui";
import { ModelOptionLabel } from "@/components/business/ProfileControls";
import type { AgentProfileLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import {
  modelProviderAvatarPath,
  modelIDsForProviderSelection,
  type ModelProviderCatalog,
} from "@/models/modelProviders";
import styles from "./DefaultModelSettings.module.css";

type Defaults = Pick<AgentProfileLike, "model_provider_id" | "model_id" | "image_generation" | "video_generation">;
const kinds = ["agent", "image", "video"] as const;
const icons = { agent: Bot, image: Image, video: Video };
const labels = { agent: "modelDefaultAgent", image: "modelDefaultImage", video: "modelDefaultVideo" } as const;
const encode = (provider: string, model: string) => JSON.stringify([provider, model]);

export function DefaultModelSettings({ catalog, t }: { catalog: ModelProviderCatalog | null; t: TranslateFn }) {
  const [defaults, setDefaults] = useState<Defaults | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const busy = loading || saving;
  const [error, setError] = useState("");
  const [loadError, setLoadError] = useState("");
  const displayedError = error || loadError;
  const [reload, setReload] = useState(0);
  useEffect(() => {
    if (saving) return;
    let active = true;
    setLoading(true);
    void fetchAgentProfileDefaults()
      .then((result) => {
        if (active) {
          setDefaults(result);
          setLoadError("");
        }
      })
      .catch((cause) => {
        if (active) setLoadError(localizeAPIError(cause, t, t("modelDefaultsLoadFailed")));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [catalog, reload, saving, t]);

  async function choose(kind: (typeof kinds)[number], value: string) {
    if (!defaults) return;
    const [providerID, modelID] = value ? (JSON.parse(value) as [string, string]) : ["", ""];
    setSaving(true);
    setError("");
    try {
      setDefaults(await updateAgentProfileDefaults({ kind, provider_id: providerID, model_id: modelID }));
    } catch (cause) {
      setError(localizeAPIError(cause, t, t("modelDefaultsSaveFailed")));
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className={styles.panel} aria-label={t("modelDefaultsTitle")} aria-busy={busy}>
      <div className={styles.heading}>
        <h2>{t("modelDefaultsTitle")}</h2>
        <p>{t("modelDefaultsHelp")}</p>
      </div>
      {kinds.map((kind) => {
        const Icon = icons[kind];
        const ref =
          kind === "agent"
            ? { provider_id: defaults?.model_provider_id, model_id: defaults?.model_id }
            : defaults?.[kind === "image" ? "image_generation" : "video_generation"];
        const value = ref?.provider_id && ref.model_id ? encode(ref.provider_id, ref.model_id) : "";
        const options = (catalog?.providers ?? [])
          .filter((provider) => provider.status === "connected")
          .flatMap((provider) => {
            const models =
              kind === "agent"
                ? modelIDsForProviderSelection(
                    provider.id,
                    provider.models.filter(
                      (model) => !provider.imageModels?.includes(model) && !provider.videoModels?.includes(model),
                    ),
                  )
                : (provider[kind === "image" ? "imageModels" : "videoModels"] ?? []);
            return models.map((model) => ({
              value: encode(provider.id, model),
              label: (
                <ModelOptionLabel
                  avatar={modelProviderAvatarPath(provider)}
                  model={model}
                  provider={provider.display_name}
                />
              ),
              textValue: `${provider.display_name} / ${model}`,
            }));
          });
        return (
          <Field
            key={kind}
            className={styles.row}
            label={
              <span className={styles.label}>
                <Icon size={16} aria-hidden="true" />
                {t(labels[kind])}
              </span>
            }
          >
            <Select
              value={options.some((option) => option.value === value) ? value : ""}
              disabled={busy || !defaults}
              options={[{ value: "", label: t("modelDefaultNone") }, ...options]}
              triggerProps={{ "aria-label": t(labels[kind]) }}
              onValueChange={(value) => void choose(kind, value)}
            />
          </Field>
        );
      })}
      {displayedError ? (
        <div className={styles.error} role="alert">
          {displayedError}
          <Button
            size="sm"
            variant="tertiaryGray"
            disabled={busy}
            onClick={() => {
              setError("");
              setReload((value) => value + 1);
            }}
          >
            {t("retry")}
          </Button>
        </div>
      ) : null}
    </section>
  );
}
