import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import profileControlsStyles from "./ProfileControls.module.css";
import { DEFAULT_NOTIFIER_POLL_INTERVAL, NOTIFIER_DELIVERY_OPTIONS } from "@/shared/constants/agents";
import { Field, TextInput, Select } from "@/components/ui";
import {
  ensureNotifierPullSubscriptionDraft,
  notifierPushWebhookNotifyURL,
  notifierRemoteTokenPlaceholderText,
  notifierThirdPartyRelayWebhookURL,
  type AgentDraft,
} from "@/models/agents";
import { ClipboardCopyButton } from "./ClipboardCopyButton";
import { FieldHelpTooltip } from "./FieldHelpTooltip";
import type { Translator } from "./types";

export type NotifierControlsProps = {
  agentID?: string;
  draft: AgentDraft;
  hideHeading?: boolean;
  onPatch: (patch: Partial<AgentDraft>) => void;
  /** Resolved from config.toml advertise_base_url (or listen_addr when empty); not user-editable. */
  webhookPublicOrigin: string;
  t: Translator;
};

export function NotifierControls({
  agentID,
  draft,
  hideHeading = false,
  onPatch,
  t,
  webhookPublicOrigin,
}: NotifierControlsProps) {
  const deliveryMode = draft.notifier_delivery_mode || "webhook";
  const publicWebhookURL = notifierPushWebhookNotifyURL(
    webhookPublicOrigin,
    agentID,
    t("notifierWebhookOriginPlaceholder"),
  );
  const relayPasteURL = notifierThirdPartyRelayWebhookURL(
    draft.notifier_remote_url,
    draft.notifier_remote_subscription_id,
  );

  function patch(next: Partial<AgentDraft>) {
    onPatch(next);
  }

  function setDeliveryMode(value: string) {
    const next = ensureNotifierPullSubscriptionDraft({ ...draft, notifier_delivery_mode: value }) as AgentDraft;
    onPatch(next);
  }

  return (
    <div className={`profile-section ${profileFormLayoutStyles["profile-section"]}`}>
      {!hideHeading ? (
        <div className="profile-section-heading">
          <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
            {t("profileNotifierSection")}
          </div>
          <p className="profile-section-description">{t("profileNotifierSectionDescription")}</p>
        </div>
      ) : null}
      <div className="agent-section-form">
        <div className="agent-section-form-content agent-notifier-form-content">
          <div
            className={`profile-grid ${profileFormLayoutStyles["profile-grid"]} profile-grid-compact ${profileFormLayoutStyles["profile-grid-compact"]}`}
          >
            <Field className={`span-2 ${profileFormLayoutStyles["span-2"]}`} label={t("notifierDeliveryMode")}>
              <Select
                value={deliveryMode}
                onValueChange={setDeliveryMode}
                options={NOTIFIER_DELIVERY_OPTIONS.map((mode) => ({
                  value: mode,
                  label: mode === "webhook" ? t("notifierDeliveryWebhook") : t("notifierDeliveryRemotePull"),
                }))}
              />
            </Field>

            {deliveryMode === "webhook" ? (
              <>
                <Field
                  className={`span-2 ${profileFormLayoutStyles["span-2"]}`}
                  required
                  label={t("notifierWebhookToken")}
                  labelAction={
                    <FieldHelpTooltip
                      summary={t("notifierWebhookTokenSummary")}
                      detail={t("notifierWebhookTokenHelp")}
                    />
                  }
                >
                  <div className={profileControlsStyles["notifier-copy-row"]}>
                    <TextInput
                      type="password"
                      autoComplete="new-password"
                      value={draft.notifier_webhook_token || ""}
                      placeholder={t("notifierWebhookTokenInputPlaceholder")}
                      onInput={(event) => patch({ notifier_webhook_token: event.currentTarget.value })}
                    />
                    <ClipboardCopyButton text={draft.notifier_webhook_token || ""} label={t("copyToClipboard")} />
                  </div>
                </Field>
                <Field
                  className={`span-2 ${profileFormLayoutStyles["span-2"]}`}
                  label={t("notifierThirdPartyCSGWebhookURL")}
                  labelAction={
                    <FieldHelpTooltip
                      summary={t("notifierThirdPartyCSGWebhookURLSummary")}
                      detail={t("notifierThirdPartyCSGWebhookURLHelp")}
                    />
                  }
                >
                  <div className={profileControlsStyles["notifier-copy-row"]}>
                    <TextInput readOnly value={publicWebhookURL} />
                    <ClipboardCopyButton text={publicWebhookURL} label={t("copyToClipboard")} />
                  </div>
                </Field>
              </>
            ) : null}

            {deliveryMode === "remote_pull" ? (
              <>
                <Field
                  className={`span-2 ${profileFormLayoutStyles["span-2"]}`}
                  required
                  label={t("notifierRemoteURL")}
                  labelAction={
                    <FieldHelpTooltip summary={t("notifierRemoteURLSummary")} detail={t("notifierRemoteURLHelp")} />
                  }
                >
                  <TextInput
                    value={draft.notifier_remote_url || ""}
                    placeholder={t("notifierRemoteURLPlaceholder")}
                    onInput={(event) => patch({ notifier_remote_url: event.currentTarget.value })}
                  />
                </Field>
                <Field
                  className={`span-2 ${profileFormLayoutStyles["span-2"]}`}
                  label={t("notifierRemoteToken")}
                  labelAction={
                    <FieldHelpTooltip summary={t("notifierRemoteTokenSummary")} detail={t("notifierRemoteTokenHelp")} />
                  }
                >
                  <TextInput
                    type="password"
                    autoComplete="new-password"
                    value={draft.notifier_remote_token || ""}
                    placeholder={notifierRemoteTokenPlaceholderText(draft, t)}
                    onInput={(event) => patch({ notifier_remote_token: event.currentTarget.value })}
                  />
                </Field>
                {relayPasteURL ? (
                  <Field
                    className={`span-2 ${profileFormLayoutStyles["span-2"]}`}
                    label={t("notifierThirdPartyWebhookPasteURL")}
                    labelAction={
                      <FieldHelpTooltip
                        summary={t("notifierThirdPartyWebhookPasteURLSummary")}
                        detail={t("notifierThirdPartyWebhookPasteURLHelp")}
                      />
                    }
                  >
                    <div className={profileControlsStyles["notifier-copy-row"]}>
                      <TextInput readOnly value={relayPasteURL} />
                      <ClipboardCopyButton text={relayPasteURL} label={t("copyToClipboard")} />
                    </div>
                  </Field>
                ) : null}
                <Field
                  label={t("notifierPollInterval")}
                  labelAction={
                    <FieldHelpTooltip
                      summary={t("notifierPollIntervalSummary")}
                      detail={t("notifierPollIntervalHelp")}
                    />
                  }
                >
                  <TextInput
                    value={draft.notifier_poll_interval || DEFAULT_NOTIFIER_POLL_INTERVAL}
                    placeholder={t("notifierPollIntervalPlaceholder")}
                    onInput={(event) => patch({ notifier_poll_interval: event.currentTarget.value })}
                  />
                </Field>
              </>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  );
}
