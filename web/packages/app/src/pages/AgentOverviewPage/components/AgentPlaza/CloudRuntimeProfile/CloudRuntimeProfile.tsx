import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchCloudProfile } from "@/api/relay";
import { Button, Field, InlineError, TextArea, TextInput } from "@/components/ui";
import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import type { CloudAgent } from "@/models/cloudAgents";
import { formatProviderLabel, formatRuntimeKindLabel } from "@/models/agents";
import type { ResourceType } from "@/models/conversationResources";
import { cloudResourceScope } from "@/models/cloudResourceScope";
import type { TranslateFn } from "@/models/conversations";
import { useConversationResources } from "@/hooks/workspace/useConversationResources";
import { localizeAPIError } from "@/shared/i18n";
import styles from "./CloudRuntimeProfile.module.css";

import { cloudRuntimeProfileSections, type CloudProfileSection } from "./sections";

export function CloudRuntimeProfile({
  agent,
  t,
  section = "profile",
}: {
  agent: CloudAgent;
  t: TranslateFn;
  section?: CloudProfileSection;
}) {
  const profile = useQuery({
    queryKey: ["cloud-runtime-profile", agent.connectionID, agent.id],
    queryFn: ({ signal }) => fetchCloudProfile(agent, signal),
    retry: false,
    enabled: section === "profile" || section === "instructions",
  });
  const scope = cloudResourceScope(agent.id, agent.connectionID);
  const resourceType =
    section === "skills"
      ? "skill"
      : section === "apps"
        ? "connector"
        : section === "knowledge"
          ? "knowledge_base"
          : null;
  return (
    <div className="profile-editor-shell agent-page-editor">
      <div id={`agent-profile-${section}`} className={styles.panel}>
        {resourceType ? (
          <CloudResourceList
            key={`${scope}:${resourceType}`}
            scope={scope}
            type={resourceType}
            label={t(cloudRuntimeProfileSections.find((item) => item.id === section)!.label)}
            t={t}
          />
        ) : (
          <>
            {profile.isError ? (
              <InlineError role="alert" className={styles.feedback}>
                <span>{localizeAPIError(profile.error, t, t("errors.resource_service_unavailable"))}</span>
                <Button
                  variant="secondaryGray"
                  size="sm"
                  loading={profile.isFetching}
                  onClick={() => void profile.refetch()}
                >
                  {t("retry")}
                </Button>
              </InlineError>
            ) : null}
            {profile.isPending ? (
              <p role="status" className={styles.notice}>
                {t("resourceLoading")}
              </p>
            ) : null}
            {section === "profile" ? (
              <>
                <ProfileSection title={t("profileRuntimeSection")} description={t("cloudRuntimeSectionDescription")}>
                  <Field label={t("profileRuntimeKind")}>
                    <TextInput
                      value={profile.data?.runtime ? formatRuntimeKindLabel(profile.data.runtime, t) : "—"}
                      readOnly
                      disabled
                    />
                  </Field>
                </ProfileSection>
                <ProfileSection title={t("profileModelSection")} description={t("cloudModelSectionDescription")}>
                  <fieldset className={styles.modelGroup}>
                    <legend>{t("profileConversationModelGroup")}</legend>
                    <div className={styles.modelFields}>
                      <Field label={t("profileProvider")}>
                        <TextInput
                          value={profile.data?.provider ? formatProviderLabel(profile.data.provider) : "—"}
                          readOnly
                          disabled
                        />
                      </Field>
                      <Field label={t("profileModel")}>
                        <TextInput value={profile.data?.model || agent.modelID || "—"} readOnly disabled />
                      </Field>
                    </div>
                  </fieldset>
                </ProfileSection>
                <p className={styles.notice}>{t("cloudProfileReadonly")}</p>
              </>
            ) : (
              <ProfileSection title={t("agentInstructions")} description={t("cloudProfileReadonly")}>
                <Field label={t("agentInstructions")}>
                  <TextArea value={profile.data?.instructions || "—"} readOnly rows={12} />
                </Field>
              </ProfileSection>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function ProfileSection({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <section className={`profile-section ${profileFormLayoutStyles["profile-section"]}`} aria-label={title}>
      <div className="profile-section-heading">
        <h3 className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>{title}</h3>
        <p className="profile-section-description">{description}</p>
      </div>
      <div className="agent-section-form">
        <div className="agent-page-form-content">{children}</div>
      </div>
    </section>
  );
}

function CloudResourceList({
  scope,
  type,
  label,
  t,
}: {
  scope: string;
  type: ResourceType;
  label: string;
  t: TranslateFn;
}) {
  const resources = useConversationResources(scope, type, "");
  return (
    <ProfileSection title={label} description={t("cloudProfileReadonly")}>
      {resources.loading ? (
        <p role="status" className={styles.notice}>
          {t("resourceLoading")}
        </p>
      ) : null}
      {resources.items.map((item) => (
        <div className={styles.resource} key={`${item.source}:${item.id}`}>
          <strong>{item.name}</strong>
          {item.description ? <p>{item.description}</p> : null}
          {!item.available ? (
            <small>{t(item.enabled ? "errors.resource_unavailable" : "errors.resource_disabled")}</small>
          ) : null}
        </div>
      ))}
      {!resources.loading && !resources.continuation.failed && !resources.items.length ? (
        <p className={styles.notice}>{t("composerResourceEmpty", { kind: label })}</p>
      ) : null}
      {resources.continuation.failed ? (
        <InlineError role="alert" className={styles.feedback}>
          <span>{localizeAPIError(resources.query.error, t, t("errors.resource_service_unavailable"))}</span>
          <Button
            variant="secondaryGray"
            size="sm"
            loading={resources.continuation.loading}
            onClick={resources.continuation.retry}
          >
            {t("retry")}
          </Button>
        </InlineError>
      ) : null}
      {resources.continuation.hasMore ? (
        <Button
          variant="tertiaryGray"
          size="sm"
          disabled={resources.continuation.loading}
          onClick={() => void resources.continuation.loadMore()}
        >
          {t("taskEntryLoadMore")}
        </Button>
      ) : null}
    </ProfileSection>
  );
}
