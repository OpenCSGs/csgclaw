import { useRef, useState } from "react";
import { checkModelProvider, createModelProvider, type ModelProviderPayload } from "@/api/modelProviders";
import type { ModelProvider, ModelProviderCatalog } from "@/models/modelProviders";
import type { LocaleCode, TranslateFn } from "@/models/conversations";

export type ModelProviderCreationProps = {
  busy: boolean;
  locale: LocaleCode;
  error: string;
  modelProviders: ModelProviderCatalog | null;
  onClose: () => void;
  onCreate: (payload: ModelProviderPayload) => Promise<void>;
  onCheckAccess: (payload: ModelProviderPayload) => ReturnType<typeof checkModelProvider>;
  t: TranslateFn;
};

export function useModelProviderCreation({
  catalog,
  locale,
  refresh,
  onSelect,
  keepCurrentPage,
  t,
}: {
  catalog: ModelProviderCatalog | null;
  locale: LocaleCode;
  refresh: () => Promise<ModelProviderCatalog | null>;
  onSelect: (provider: ModelProvider) => void;
  keepCurrentPage: boolean;
  t: TranslateFn;
}) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef(false);
  async function create(payload: ModelProviderPayload) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const created = await createModelProvider(payload);
      if (created.id && payload.base_url && (payload.api_key || created.api_key_set)) {
        try {
          await checkModelProvider(created.id, payload);
        } catch {
          // A saved provider remains usable; its detail view can retry the access check.
        }
      }
      const refreshed = await refresh();
      if (!keepCurrentPage) onSelect(refreshed?.providers.find((provider) => provider.id === created.id) ?? created);
      setOpen(false);
    } catch {
      setError(t("modelProviderCreateFailed"));
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  const modalProps: ModelProviderCreationProps | null = open
    ? {
        busy,
        locale,
        error,
        modelProviders: catalog,
        onCreate: create,
        t,
        onClose: () => {
          if (!pending.current) setOpen(false);
        },
        onCheckAccess: (payload) => checkModelProvider("openai-draft", payload),
      }
    : null;
  return {
    open: () => {
      setError("");
      setOpen(true);
    },
    modalProps,
  };
}
