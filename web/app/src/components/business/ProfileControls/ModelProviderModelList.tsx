import { useMemo, useState } from "react";
import { Search } from "lucide-react";
import { classNames } from "@/shared/lib/classNames";

export type ModelProviderModelListProps = {
  className?: string;
  emptyLabel: string;
  modelListLabel: string;
  models: string[];
  searchLabel: string;
};

export function ModelProviderModelList({
  className,
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
        <input
          value={filterText}
          onInput={(event) => setFilterText(event.currentTarget.value)}
          placeholder={searchLabel}
        />
      </label>
      <div className="model-provider-model-list" role="list" aria-label={modelListLabel}>
        {visibleModels.length ? (
          visibleModels.map(({ index, model }) => (
            <div className="model-provider-model-row" role="listitem" key={`${model}-${index}`}>
              <span className="model-provider-model-name">{model}</span>
            </div>
          ))
        ) : (
          <div className="model-provider-model-empty">{emptyLabel}</div>
        )}
      </div>
    </div>
  );
}
