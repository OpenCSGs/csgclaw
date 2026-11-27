import { useDescriptionAccess } from "@/hooks/useDescriptionAccess";
import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Sparkles } from "lucide-react";
import { apiErrorCode } from "@/api/client";
import { interpretHubTemplate } from "@/api/hub";
import { IntroductionCard, IntroductionMarkdown } from "@/components/business/IntroductionCard";
import { Button, Field, InlineError, TextInput } from "@/components/ui";
import { formatRuntimeKindLabel } from "@/models/agents";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import { formatHubDateTime, type HubTemplate } from "@/models/hubWorkspace";
import { isOfficialTemplate, templateSourceLabelKey } from "@/models/templateGroups";
import { classNames } from "@/shared/lib/classNames";
import styles from "../HubDetailPane.module.css";
import templateResourceStyles from "../TemplateResource.module.css";

export function TemplateProfilePanel({
  selectedTemplate,
  templateUsername,
  locale,
  t,
  showHeading = false,
  intro,
  instanceModel,
  templateModel,
}: {
  selectedTemplate: HubTemplate;
  templateUsername?: string;
  locale: LocaleCode;
  t: TranslateFn;
  showHeading?: boolean;
  intro?: ReactNode;
  instanceModel?: string;
  templateModel?: string;
}) {
  const templateImageEnv = selectedTemplate.image_env ?? [];
  return (
    <div className={classNames("profile-editor-shell", styles["hub-template-profile-shell"])}>
      <TemplateIntroduction
        key={`${selectedTemplate.id}:${selectedTemplate.revision ?? ""}`}
        template={selectedTemplate}
        locale={locale}
        t={t}
      />
      {intro}
      <section className={`profile-section ${profileFormLayoutStyles["profile-section"]}`}>
        {showHeading ? (
          <div className="profile-section-heading">
            <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
              {t("profileRuntimeSection")}
            </div>
            <p className="profile-section-description">{t("profileRuntimeSectionDescription")}</p>
          </div>
        ) : null}
        <div
          className={classNames(
            `profile-grid-compact ${profileFormLayoutStyles["profile-grid-compact"]}`,
            templateResourceStyles["hub-template-profile-grid"],
          )}
        >
          <Field label={t("resourcesRuntimeLabel")}>
            <TextInput
              value={selectedTemplate.runtime_kind ? formatRuntimeKindLabel(selectedTemplate.runtime_kind, t) : "-"}
              readOnly
              disabled
            />
          </Field>
          <Field label={t("resourcesSourceLabel")}>
            <TextInput value={t(templateSourceLabelKey(selectedTemplate, templateUsername))} readOnly disabled />
          </Field>
          <Field label={t("resourcesImageLabel")}>
            <TextInput value={selectedTemplate.image || "-"} readOnly disabled />
          </Field>
          <Field label={t("resourcesUpdatedAtLabel")}>
            <TextInput value={formatHubDateTime(selectedTemplate.updated_at, locale)} readOnly disabled />
          </Field>
          {instanceModel ? (
            <Field label={t("agentPlazaInstanceModel")}>
              <TextInput value={instanceModel} readOnly disabled />
            </Field>
          ) : null}
          {templateModel && templateModel !== instanceModel ? (
            <Field label={t("agentPlazaTemplateModel")}>
              <TextInput value={templateModel} readOnly disabled />
            </Field>
          ) : null}
          <div
            className={classNames(
              `span-2 ${profileFormLayoutStyles["span-2"]}`,
              templateResourceStyles["hub-template-env-field"],
            )}
          >
            <div className={templateResourceStyles["hub-template-env-heading"]}>
              <span>{t("resourcesTemplateEnvLabel")}</span>
              <span className={templateResourceStyles["hub-template-env-count"]}>
                {t("resourcesTemplateEnvCount", { count: templateImageEnv.length })}
              </span>
            </div>
            {templateImageEnv.length ? (
              <div className={templateResourceStyles["hub-template-env-list"]} role="list">
                {templateImageEnv.map((item) => (
                  <div className={templateResourceStyles["hub-template-env-chip"]} role="listitem" key={item.name}>
                    <code className={templateResourceStyles["hub-template-env-name"]}>{item.name}</code>
                    <span
                      className={classNames(
                        templateResourceStyles["hub-template-env-status"],
                        item.required ? "required" : "optional",
                      )}
                    >
                      <span className={templateResourceStyles["hub-template-env-status-dot"]} aria-hidden="true"></span>
                      {item.required ? t("resourcesTemplateEnvRequiredBadge") : t("resourcesTemplateEnvOptional")}
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <div className={templateResourceStyles["hub-template-env-empty"]}>
                {t("resourcesTemplateEnvNotRequired")}
              </div>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}

function TemplateIntroduction({ template, locale, t }: { template: HubTemplate; locale: LocaleCode; t: TranslateFn }) {
  const official = isOfficialTemplate(template);
  const descriptionAccess = useDescriptionAccess(official);
  const [interpretation, setInterpretation] = useState("");
  const [pending, setPending] = useState(false);
  const [errorCode, setErrorCode] = useState("");
  const requestRef = useRef<AbortController | null>(null);
  useEffect(() => () => requestRef.current?.abort(), []);

  async function interpret() {
    const templateID = template.id;
    if (!templateID || requestRef.current) return;
    const controller = new AbortController();
    requestRef.current = controller;
    setPending(true);
    setErrorCode("");
    try {
      const result = await interpretHubTemplate(templateID, locale, controller.signal);
      if (!controller.signal.aborted) setInterpretation(result.description);
    } catch (error) {
      if (!controller.signal.aborted) setErrorCode(apiErrorCode(error) || "template_interpretation_failed");
    } finally {
      requestRef.current = null;
      if (!controller.signal.aborted) setPending(false);
    }
  }

  return (
    <IntroductionCard
      title={t("templateIntroduction")}
      hint={official && descriptionAccess.status === "denied" ? t("descriptionAccessUnavailable") : undefined}
      actions={
        official && descriptionAccess.status === "allowed" ? (
          <Button variant="secondaryGray" size="sm" disabled={pending || !template.id} onClick={interpret}>
            <Sparkles size={14} aria-hidden="true" />
            {t(pending ? "templateInterpreting" : "templateInterpret")}
          </Button>
        ) : undefined
      }
    >
      <IntroductionMarkdown text={template.description} empty={t("agentIntroductionEmpty")} />
      {descriptionAccess.status === "error" ? (
        <InlineError role="alert">
          {t("descriptionAccessCheckFailed")}{" "}
          <Button variant="tertiaryGray" size="sm" onClick={descriptionAccess.retry}>
            {t("retry")}
          </Button>
        </InlineError>
      ) : null}
      {errorCode ? (
        <InlineError role="alert">
          {t(
            errorCode === "template_interpretation_model_unavailable"
              ? "templateInterpretationModelUnavailable"
              : errorCode === "template_interpretation_timeout"
                ? "templateInterpretationTimeout"
                : "templateInterpretationFailed",
          )}
        </InlineError>
      ) : null}
      {interpretation ? (
        <div className={templateResourceStyles["hub-template-interpretation"]} role="status">
          <h3>{t("templateInterpretationTitle")}</h3>
          <IntroductionMarkdown text={interpretation} />
        </div>
      ) : null}
    </IntroductionCard>
  );
}
