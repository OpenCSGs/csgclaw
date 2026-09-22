import { useEffect, useState } from "react";
import { Check, Pencil, RotateCcw, RefreshCw } from "lucide-react";
import { Button, PopoverRoot, PopoverTrigger, PopoverContent, Select, TextInput, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import {
  DEFAULT_CONTEXT_TOKENS,
  contextUnit,
  contextInputValue,
  formatContextSize,
  parseContextSize,
  type ContextUnit,
  type ModelMetadata,
  type ModelOverride,
} from "@/models/modelMetadata";

export function ModelMetadataFields({
  model,
  metadata,
  automatic,
  value,
  onChange,
  onRefresh,
  t,
  changed = false,
  disabled = false,
}: {
  model: string;
  metadata?: ModelMetadata;
  automatic?: ModelMetadata;
  value?: ModelOverride;
  onChange: (value: ModelOverride | undefined) => void;
  onRefresh: () => Promise<void>;
  t: TranslateFn;
  changed?: boolean;
  disabled?: boolean;
}) {
  const resolved = automatic || metadata;
  const capacity = value?.context_window || resolved?.context_window || DEFAULT_CONTEXT_TOKENS;
  const source = value?.context_window ? "user" : resolved?.context_source || "default";
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [unit, setUnit] = useState<ContextUnit>("K");
  const [invalid, setInvalid] = useState(false);
  const [feedback, setFeedback] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState(false);
  useEffect(() => {
    if (!feedback || refreshing) return;
    const timer = window.setTimeout(() => setFeedback(""), 2500);
    return () => window.clearTimeout(timer);
  }, [feedback, refreshing]);
  const tokens = parseContextSize(draft, unit);
  function openEditor(open: boolean) {
    if (open) {
      setUnit(contextUnit(capacity));
      setDraft(contextInputValue(capacity));
      setInvalid(false);
    }
    setEditing(open);
  }
  return (
    <div className={`model-context-cell${changed ? " is-changed" : ""}`}>
      <PopoverRoot open={editing} onOpenChange={openEditor}>
        <PopoverTrigger asChild>
          <button
            type="button"
            className="model-context-edit"
            disabled={disabled || refreshing}
            aria-label={`${model} ${t("modelMetadataContext")}`}
          >
            <span>{formatContextSize(capacity)}</span>
            <Pencil size={12} aria-hidden="true" />
          </button>
        </PopoverTrigger>
        <PopoverContent className="model-context-popover" align="start">
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (tokens === undefined) {
                setInvalid(true);
                return;
              }
              onChange({ context_window: tokens });
              setRefreshError(false);
              setFeedback(t("modelContextUpdated"));
              setEditing(false);
            }}
          >
            <strong>{t("modelMetadataContext")}</strong>
            <span className="model-context-editor-model">{model}</span>
            <div className="model-context-inputs">
              <TextInput
                autoFocus
                inputMode="decimal"
                value={draft}
                aria-label={`${model} ${t("modelMetadataContext")}`}
                aria-invalid={invalid}
                onFocus={(event) => event.target.select()}
                onChange={(event) => {
                  setDraft(event.target.value);
                  setInvalid(false);
                }}
              />
              <Select
                value={unit}
                options={[
                  { value: "K", label: "K tokens" },
                  { value: "M", label: "M tokens" },
                ]}
                triggerProps={{ "aria-label": t("modelContextUnit") }}
                onValueChange={(next) => {
                  const u = next as ContextUnit;
                  if (tokens !== undefined) setDraft(contextInputValue(tokens, u));
                  setUnit(u);
                }}
              />
            </div>
            <p className={invalid ? "model-context-error" : "model-context-hint"} role={invalid ? "alert" : undefined}>
              {invalid
                ? t("modelContextInvalid")
                : tokens === undefined
                  ? t("modelContextConversion")
                  : t("modelContextTokenCount", { count: tokens.toLocaleString() })}
            </p>
            <div className="model-context-editor-actions">
              <Button
                size="sm"
                variant="ghost"
                disabled={!value?.context_window}
                onClick={() => {
                  setEditing(false);
                  onChange(undefined);
                  setRefreshError(false);
                  setFeedback(t("modelContextResetDone"));
                }}
              >
                <RotateCcw size={14} aria-hidden="true" />
                {t("modelMetadataReset")}
              </Button>
              <Button size="sm" onClick={() => setEditing(false)}>
                {t("cancel")}
              </Button>
              <Button size="sm" type="submit" variant="primary">
                <Check size={14} aria-hidden="true" />
                {t("modelContextApply")}
              </Button>
            </div>
          </form>
        </PopoverContent>
      </PopoverRoot>
      <span
        className={`model-context-source${refreshError ? " is-error" : changed ? " is-pending" : source === "user" ? " is-user" : ""}`}
        role="status"
      >
        {refreshing
          ? t("modelContextRefreshing")
          : feedback || (changed ? t("modelContextPending") : t(`modelMetadataSource_${source}`))}
      </span>
      <Tooltip content={t("modelContextRefresh")}>
        <Button
          variant="ghost"
          size="sm"
          iconOnly
          className="model-context-reset"
          disabled={disabled}
          loading={refreshing}
          loadingLabel={t("modelContextRefreshing")}
          aria-label={`${model} ${t("modelContextRefresh")}`}
          onClick={async () => {
            setRefreshing(true);
            setRefreshError(false);
            setFeedback("");
            try {
              await onRefresh();
              setFeedback(t("modelContextRefreshed"));
            } catch {
              setRefreshError(true);
              setFeedback(t("modelContextRefreshFailed"));
            } finally {
              setRefreshing(false);
            }
          }}
        >
          <RefreshCw size={14} aria-hidden="true" />
        </Button>
      </Tooltip>
    </div>
  );
}
