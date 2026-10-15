import { useState } from "react";
import { ExternalLink } from "lucide-react";
import type { ModelProviderPayload } from "@/api/modelProviders";
import {
  Button,
  Select,
  DialogRoot,
  DialogContent,
  DialogTitle,
  DialogDescription,
  TextInput,
  TextArea,
  Field,
} from "@/components/ui";
import { useModelProviderPresets } from "@/hooks/useModelProviderPresets";
import {
  modelProviderDisplayNameExists,
  parseModelProviderModelsText,
  type ModelProviderCatalog,
} from "@/models/modelProviders";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import { ModalCloseButton } from "./ModalCloseButton";
import styles from "./CreateModelProviderModal.module.css";

export type CreateModelProviderPayload = ModelProviderPayload;
export type CreateModelProviderModalProps = {
  busy: boolean;
  error?: string;
  locale?: LocaleCode;
  initialPreset?: string;
  modelProviders?: ModelProviderCatalog | null;
  onClearError?: () => void;
  onClose: () => void;
  onCreate: (payload: CreateModelProviderPayload) => Promise<void> | void;
  t: TranslateFn;
};

export function CreateModelProviderModal({
  busy,
  error = "",
  locale = "en",
  initialPreset = "",
  modelProviders = null,
  onClose,
  onClearError,
  onCreate,
  t,
}: CreateModelProviderModalProps) {
  const directory = useModelProviderPresets();
  const presets = directory.data?.presets ?? [];
  const [selected, setSelected] = useState(initialPreset);
  const [brand, setBrand] = useState("");
  const [regionID, setRegion] = useState("");
  const [query, setQuery] = useState("");
  const [apiKey, setKey] = useState("");
  const [visible, setVisible] = useState(false);
  const [advanced, setAdvanced] = useState(false);
  const [openSelect, setOpenSelect] = useState("");
  const [name, setName] = useState("");
  const [baseURL, setBaseURL] = useState<string>();
  const [protocol, setProtocol] = useState("");
  const [headers, setHeaders] = useState("");
  const [models, setModels] = useState("");
  const [localError, setLocalError] = useState("");
  const preset = presets.find((item) => item.id === selected);
  const activeBrand = preset?.brand || brand;
  const region = preset?.regions.find((item) => item.id === regionID) ?? preset?.regions[0];
  const brandPresets = presets.filter((item) => item.brand === activeBrand);
  const brands = presets.filter((item, index) => presets.findIndex((other) => other.brand === item.brand) === index);
  const duplicateName = Boolean(name.trim()) && modelProviderDisplayNameExists(modelProviders, name);
  const cn = locale === "zh";
  const reset = () => {
    onClearError?.();
    setKey("");
    setVisible(false);
    setName("");
    setBaseURL(undefined);
    setProtocol("");
    setHeaders("");
    setModels("");
    setLocalError("");
  };
  const selectService = (id: string) => {
    setSelected(id);
    setRegion("");
    reset();
  };
  const close = () => {
    if (busy) return;
    if ((apiKey || name || headers || models) && !window.confirm(t("modelProviderDiscardConfirm"))) return;
    onClose();
  };
  function connect() {
    if (busy || !preset || duplicateName) return;
    setLocalError("");
    let customHeaders: Record<string, string> | undefined;
    try {
      if (headers.trim()) {
        const parsed: unknown = JSON.parse(headers);
        if (
          !parsed ||
          typeof parsed !== "object" ||
          Array.isArray(parsed) ||
          Object.values(parsed).some((value) => typeof value !== "string")
        )
          throw new Error();
        customHeaders = parsed as Record<string, string>;
      }
    } catch {
      setLocalError(t("modelConnectHeadersInvalid"));
      return;
    }
    void onCreate({
      preset: preset.id,
      region: region?.id,
      api_protocol: protocol || preset.api_protocol,
      api_key: apiKey.trim(),
      base_url: (baseURL ?? region?.base_url ?? "").trim(),
      display_name: name.trim() || undefined,
      headers: customHeaders,
      models: parseModelProviderModelsText(models),
      connect: true,
    });
  }
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        className={styles.dialog}
        onEscapeKeyDown={(event) => {
          // Keep a fast Escape in a newly opened Select from dismissing its parent dialog.
          if (openSelect) {
            event.preventDefault();
            setOpenSelect("");
          }
        }}
      >
        <header className={styles.header}>
          <div>
            <DialogTitle>{t("modelProviderCreateTitle")}</DialogTitle>
            <DialogDescription>{t("modelConnectSubtitle")}</DialogDescription>
          </div>
          <ModalCloseButton label={t("close")} onClose={close} />
        </header>
        <div className={styles.body}>
          {!preset ? (
            <>
              <TextInput
                type="search"
                autoFocus
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                aria-label={t("modelConnectBrandSearch")}
                placeholder={t("modelConnectBrandSearch")}
              />
              <div className={styles.brands}>
                {brands
                  .filter((item) =>
                    presets
                      .filter((p) => p.brand === item.brand)
                      .some((p) =>
                        `${p.brand_name} ${p.name} ${p.aliases.join(" ")}`.toLowerCase().includes(query.toLowerCase()),
                      ),
                  )
                  .map((item) => (
                    <button
                      type="button"
                      className={styles.brand}
                      key={item.brand}
                      onClick={() => {
                        setBrand(item.brand);
                        selectService(item.id);
                      }}
                    >
                      <img src={item.avatar} alt="" />
                      <span>
                        {cn
                          ? item.brand_name
                          : item.brand === "aliyun"
                            ? "Alibaba Cloud"
                            : item.brand === "volcengine"
                              ? "Volcengine Ark"
                              : item.brand === "zhipu"
                                ? "GLM / Z.ai"
                                : item.brand === "mimo"
                                  ? "Xiaomi MiMo"
                                  : item.brand === "custom"
                                    ? t("modelProviderPresetCustom")
                                    : item.brand_name}
                      </span>
                      <small>
                        {presets
                          .filter((p) => p.brand === item.brand)
                          .map((p) => p.name)
                          .join(" · ")}
                      </small>
                    </button>
                  ))}
              </div>
              {directory.isPending ? <p role="status">{t("loading")}</p> : null}
              {directory.isError ? (
                <div role="alert">
                  {t("modelConnectDirectoryError")}{" "}
                  <Button variant="secondaryGray" onClick={directory.refetch}>
                    {t("retry")}
                  </Button>
                </div>
              ) : null}
            </>
          ) : (
            <>
              <Button
                variant="tertiaryGray"
                className={styles.back}
                size="sm"
                disabled={busy}
                onClick={() => {
                  setSelected("");
                  setBrand("");
                  reset();
                }}
              >
                {t("modelConnectChooseBrand")}
              </Button>
              <div className={styles.identity}>
                <img src={preset.avatar} alt="" />
                <div>
                  <strong>{preset.brand_name}</strong>
                  <p>{preset.name}</p>
                </div>
              </div>
              <Field label={t("modelConnectService")}>
                <Select
                  open={openSelect === "service"}
                  onOpenChange={(open) => setOpenSelect(open ? "service" : "")}
                  value={preset.id}
                  disabled={busy}
                  onValueChange={selectService}
                  options={brandPresets.map((p) => ({ value: p.id, label: p.name }))}
                  triggerProps={{ "aria-label": t("modelConnectService") }}
                />
              </Field>
              {preset.regions.length > 1 ? (
                <Field label={t("modelConnectRegion")}>
                  <Select
                    open={openSelect === "region"}
                    onOpenChange={(open) => setOpenSelect(open ? "region" : "")}
                    value={region?.id}
                    disabled={busy}
                    onValueChange={(id) => {
                      setRegion(id);
                      reset();
                    }}
                    options={preset.regions.map((r) => ({ value: r.id, label: r.name }))}
                    triggerProps={{ "aria-label": t("modelConnectRegion") }}
                  />
                </Field>
              ) : null}
              <div className={styles.keyHeading}>
                <span>{t("profileAPIKey")} *</span>
                <span className={styles.keyLinks}>
                  {region?.key_url ? (
                    <a href={region.key_url} target="_blank" rel="noreferrer">
                      {t("modelConnectGetKey")}
                      <ExternalLink size={13} aria-hidden="true" />
                    </a>
                  ) : null}
                  {preset.docs_url ? (
                    <a href={preset.docs_url} target="_blank" rel="noreferrer">
                      {t("modelConnectDocs")}
                      <ExternalLink size={13} aria-hidden="true" />
                    </a>
                  ) : null}
                </span>
              </div>
              <div className={styles.secret}>
                <TextInput
                  autoFocus
                  type={visible ? "text" : "password"}
                  autoComplete="off"
                  value={apiKey}
                  disabled={busy}
                  onChange={(event) => {
                    setKey(event.target.value);
                    onClearError?.();
                  }}
                  aria-label={t("profileAPIKey")}
                  placeholder={t("profileAPIKeyNewPlaceholder")}
                />
                <Button
                  variant="tertiaryGray"
                  size="sm"
                  aria-pressed={visible}
                  aria-label={t(visible ? "modelProviderHideSecret" : "modelProviderShowSecret")}
                  onClick={() => setVisible(!visible)}
                >
                  {t(visible ? "modelProviderHideSecret" : "modelProviderShowSecret")}
                </Button>
              </div>
              <p className={styles.hint}>
                {t(
                  preset.id.includes("plan") || preset.id.includes("coding") || preset.id.includes("agent")
                    ? "modelConnectPlanHint"
                    : "modelProviderAPIKeyHint",
                )}
              </p>
              <button
                type="button"
                className={styles.advancedToggle}
                aria-expanded={advanced}
                onClick={() => setAdvanced(!advanced)}
              >
                {advanced ? "▾" : "▸"} {t("modelConnectAdvanced")}
              </button>
              {advanced || preset.id === "custom" ? (
                <div className={styles.advanced}>
                  <Field
                    label={t("modelProviderDisplayName")}
                    error={duplicateName ? t("modelProviderDuplicateDisplayName") : undefined}
                  >
                    <TextInput
                      value={name}
                      disabled={busy}
                      onChange={(event) => setName(event.target.value)}
                      placeholder={preset.name}
                    />
                  </Field>
                  <Field label={t("profileBaseURL")}>
                    <TextInput
                      value={baseURL ?? region?.base_url ?? ""}
                      disabled={busy}
                      onChange={(event) => setBaseURL(event.target.value)}
                    />
                  </Field>
                  <Field label={t("modelConnectProtocol")}>
                    <Select
                      open={openSelect === "protocol"}
                      onOpenChange={(open) => setOpenSelect(open ? "protocol" : "")}
                      value={protocol || preset.api_protocol}
                      disabled={busy}
                      onValueChange={setProtocol}
                      options={[
                        { value: "responses", label: "OpenAI Responses" },
                        { value: "chat_completions", label: "OpenAI Chat Completions" },
                        { value: "anthropic_messages", label: "Anthropic Messages" },
                      ]}
                      triggerProps={{ "aria-label": t("modelConnectProtocol") }}
                    />
                  </Field>
                  <Field label={t("profileHeaders")}>
                    <TextArea
                      value={headers}
                      disabled={busy}
                      onChange={(event) => setHeaders(event.target.value)}
                      placeholder={'{"X-Project": "project-id"}'}
                    />
                  </Field>
                  <Field label={t("modelProviderModels")} hint={t("modelConnectManualModels")}>
                    <TextArea
                      value={models}
                      disabled={busy}
                      onChange={(event) => setModels(event.target.value)}
                      placeholder="model-id"
                    />
                  </Field>
                </div>
              ) : null}
            </>
          )}
          {error || localError ? (
            <p className={styles.error} role="alert">
              {error || localError}
            </p>
          ) : null}
        </div>
        <footer className={styles.footer}>
          <Button variant="secondaryGray" disabled={busy} onClick={close}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!preset || !apiKey.trim() || !(baseURL ?? region?.base_url) || duplicateName}
            onClick={connect}
          >
            {t("modelConnectAction")}
          </Button>
        </footer>
      </DialogContent>
    </DialogRoot>
  );
}
