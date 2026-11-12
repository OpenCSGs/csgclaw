import { useEffect, useState } from "react";
import { fetchModelProviderPresets } from "@/api/modelProviders";
import type { ModelServicePreset } from "@/models/modelProviderPresets";

export function useModelProviderPresets(enabled = true) {
  const [data, setData] = useState<{ presets: ModelServicePreset[] }>();
  const [error, setError] = useState(false);
  const [version, setVersion] = useState(0);
  useEffect(() => {
    if (!enabled) return;
    const abort = new AbortController();
    void Promise.resolve()
      .then(() => fetchModelProviderPresets(abort.signal))
      .then((value) => {
        if (!abort.signal.aborted) {
          setData(value);
          setError(false);
        }
      })
      .catch(() => {
        if (!abort.signal.aborted) setError(true);
      });
    return () => abort.abort();
  }, [version, enabled]);
  return {
    data,
    isError: error,
    isPending: !data && !error,
    refetch: () => {
      setError(false);
      setVersion((v) => v + 1);
    },
  };
}
