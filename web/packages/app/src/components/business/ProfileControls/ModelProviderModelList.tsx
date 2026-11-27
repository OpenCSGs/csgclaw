import { TextInput } from "csgclaw-ui";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { Search } from "lucide-react";
import { classNames } from "@/shared/lib/classNames";
import styles from "./ProfileControls.module.css";

export type ModelProviderModelListProps = {
  qualifierForModel?: (model: string) => string | undefined;
  renderDetails?: (model: string) => ReactNode;
  className?: string;
  emptyLabel: string;
  modelListLabel: string;
  models: string[];
  searchLabel: string;
};

export function ModelProviderModelList({
  className,
  qualifierForModel,
  renderDetails,
  emptyLabel,
  modelListLabel,
  models,
  searchLabel,
}: ModelProviderModelListProps) {
  const [filterText, setFilterText] = useState("");
  const visibleModels = useMemo(() => {
    const query = filterText.trim().toLowerCase();
    const indexedModels = models.map((model, index) => ({ index, model }));
    if (!query) {
      return indexedModels;
    }
    return indexedModels.filter(({ model }) => model.toLowerCase().includes(query));
  }, [filterText, models]);

  return (
    <div className={classNames("model-provider-model-list-view", className)}>
      <label className="model-provider-model-search">
        <Search size={16} aria-hidden="true" />
        <span className="sr-only">{searchLabel}</span>
        <TextInput
          variant="plain"
          value={filterText}
          onInput={(event) => setFilterText(event.currentTarget.value)}
          placeholder={searchLabel}
        />
      </label>
      {visibleModels.length ? (
        <div className="model-provider-model-grid" role="list" aria-label={modelListLabel}>
          {visibleModels.map(({ index, model }) => {
            const qualifier = qualifierForModel?.(model);
            return (
              <div className="model-provider-model-card" role="listitem" key={`${model}-${index}`}>
                <span className={styles["model-list-label"]}>
                  <span className="model-provider-model-name" title={model}>
                    {model}
                  </span>
                  {qualifier ? <span className={styles["model-qualifier"]}>({qualifier})</span> : null}
                </span>
                {renderDetails?.(model)}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="model-provider-model-empty">{emptyLabel}</div>
      )}
    </div>
  );
}
