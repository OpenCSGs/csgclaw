import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import type { ReactNode } from "react";
import { Field, TextInput } from "@/components/ui";
import { formatRuntimeKindLabel } from "@/models/agents";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import { formatHubDateTime, type HubTemplate } from "@/models/hubWorkspace";
import { templateSourceLabelKey } from "@/models/templateGroups";
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
