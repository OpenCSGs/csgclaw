import { apiErrorCode, errorMessage } from "@/api/client";
import { useRef, useState } from "react";
import { createModelProvider, type ModelProviderPayload } from "@/api/modelProviders";
import type { ModelProvider, ModelProviderCatalog } from "@/models/modelProviders";
import type { LocaleCode, TranslateFn } from "@/models/conversations";

export type ModelProviderCreationOptions = { preset?: string; onConnected?: (provider: ModelProvider) => void };

export type ModelProviderCreationProps = {
  initialPreset?: string;
  busy: boolean;
  locale: LocaleCode;
  error: string;
  modelProviders: ModelProviderCatalog | null;
  onClose: () => void;
  onCreate: (payload: ModelProviderPayload) => Promise<void>;
  onClearError: () => void;
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
  const options = useRef<ModelProviderCreationOptions>({});
  async function create(payload: ModelProviderPayload) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const created = await createModelProvider(payload);
      const refreshed = await refresh();
      const connected = refreshed?.providers.find((provider) => provider.id === created.id) ?? created;
      if (options.current.onConnected) options.current.onConnected(connected);
      else if (!keepCurrentPage) onSelect(connected);
      setOpen(false);
    } catch (cause) {
      const code = apiErrorCode(cause);
      const message = (
        {
          invalid_api_key: "modelConnectErrorKey",
          permission_denied: "modelConnectErrorPermission",
          insufficient_quota: "modelConnectErrorQuota",
          rate_limit: "modelConnectErrorRate",
          timeout: "modelConnectErrorNetwork",
          network_error: "modelConnectErrorNetwork",
          model_not_found: "modelConnectErrorModel",
          models_required: "modelConnectManualModels",
        } as const
      )[code as "invalid_api_key"];
      setError(message ? t(message) : errorMessage(cause, t("modelProviderCreateFailed")));
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  const modalProps: ModelProviderCreationProps | null = open
    ? {
        busy,
        initialPreset: options.current.preset,
        locale,
        error,
        modelProviders: catalog,
        onCreate: create,
        t,
        onClose: () => {
          if (!pending.current) setOpen(false);
        },
        onClearError: () => setError(""),
      }
    : null;
  return {
    open: (request: ModelProviderCreationOptions = {}) => {
      options.current = request;
      setError("");
      setOpen(true);
    },
    modalProps,
  };
}
