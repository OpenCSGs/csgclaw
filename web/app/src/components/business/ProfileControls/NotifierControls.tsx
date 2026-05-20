import { DEFAULT_NOTIFIER_POLL_INTERVAL, NOTIFIER_DELIVERY_OPTIONS } from "@/shared/constants/agents";
import type { ReactNode } from "react";
import {
  ensureNotifierPullSubscriptionDraft,
  notifierPushWebhookNotifyURL,
  notifierRemoteTokenPlaceholderText,
  notifierThirdPartyRelayWebhookURL,
  type AgentDraft,
} from "@/models/agents";
import { ClipboardCopyButton } from "./ClipboardCopyButton";
import { FieldHelpTooltip } from "./FieldHelpTooltip";
import { requiredFieldLabel } from "./requiredFieldLabel";
import type { Translator } from "./types";

export type NotifierControlsProps = {
  agentID?: string;
  draft: AgentDraft;
  onPatch: (patch: Partial<AgentDraft>) => void;
  /** Resolved from config.toml advertise_base_url (or listen_addr when empty); not user-editable. */
  webhookPublicOrigin: string;
  t: Translator;
};

function FieldLabelWithHelp({ children, detail, summary }: { children: ReactNode; detail?: string; summary?: string }) {
  return (
    <div className="field-label-with-help">
      {children}
      <FieldHelpTooltip summary={summary} detail={detail} />
    </div>
  );
}

export function NotifierControls({
  agentID,
  draft,
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
    <section className="profile-section">
      <div className="profile-section-title">{t("profileNotifierSection")}</div>
      <div className="profile-grid profile-grid-compact">
        <label className="field span-2">
          <span>{t("notifierDeliveryMode")}</span>
          <select value={deliveryMode} onChange={(event) => setDeliveryMode(event.currentTarget.value)}>
            {NOTIFIER_DELIVERY_OPTIONS.map((mode) => (
              <option key={mode} value={mode}>
                {mode === "webhook" ? t("notifierDeliveryWebhook") : t("notifierDeliveryRemotePull")}
              </option>
            ))}
          </select>
        </label>

        {deliveryMode === "webhook" ? (
          <>
            <label className="field span-2">
              <FieldLabelWithHelp summary={t("notifierWebhookTokenSummary")} detail={t("notifierWebhookTokenHelp")}>
                {requiredFieldLabel(t("notifierWebhookToken"))}
              </FieldLabelWithHelp>
              <div className="notifier-copy-row">
                <input
                  type="password"
                  autoComplete="new-password"
                  value={draft.notifier_webhook_token || ""}
                  placeholder={t("notifierWebhookTokenInputPlaceholder")}
                  onInput={(event) => patch({ notifier_webhook_token: event.currentTarget.value })}
                />
                <ClipboardCopyButton text={draft.notifier_webhook_token || ""} label={t("copyToClipboard")} />
              </div>
            </label>
            <label className="field span-2">
              <FieldLabelWithHelp
                summary={t("notifierThirdPartyCSGWebhookURLSummary")}
                detail={t("notifierThirdPartyCSGWebhookURLHelp")}
              >
                <span>{t("notifierThirdPartyCSGWebhookURL")}</span>
              </FieldLabelWithHelp>
              <div className="notifier-copy-row">
                <input readOnly value={publicWebhookURL} />
                <ClipboardCopyButton text={publicWebhookURL} label={t("copyToClipboard")} />
              </div>
            </label>
          </>
        ) : null}

        {deliveryMode === "remote_pull" ? (
          <>
            <label className="field span-2">
              <FieldLabelWithHelp summary={t("notifierRemoteURLSummary")} detail={t("notifierRemoteURLHelp")}>
                {requiredFieldLabel(t("notifierRemoteURL"))}
              </FieldLabelWithHelp>
              <input
                value={draft.notifier_remote_url || ""}
                placeholder={t("notifierRemoteURLPlaceholder")}
                onInput={(event) => patch({ notifier_remote_url: event.currentTarget.value })}
              />
            </label>
            <label className="field span-2">
              <FieldLabelWithHelp summary={t("notifierRemoteTokenSummary")} detail={t("notifierRemoteTokenHelp")}>
                <span>{t("notifierRemoteToken")}</span>
              </FieldLabelWithHelp>
              <input
                type="password"
                autoComplete="new-password"
                value={draft.notifier_remote_token || ""}
                placeholder={notifierRemoteTokenPlaceholderText(draft, t)}
                onInput={(event) => patch({ notifier_remote_token: event.currentTarget.value })}
              />
            </label>
            {relayPasteURL ? (
              <label className="field span-2">
                <FieldLabelWithHelp
                  summary={t("notifierThirdPartyWebhookPasteURLSummary")}
                  detail={t("notifierThirdPartyWebhookPasteURLHelp")}
                >
                  <span>{t("notifierThirdPartyWebhookPasteURL")}</span>
                </FieldLabelWithHelp>
                <div className="notifier-copy-row">
                  <input readOnly value={relayPasteURL} />
                  <ClipboardCopyButton text={relayPasteURL} label={t("copyToClipboard")} />
                </div>
              </label>
            ) : null}
            <label className="field">
              <FieldLabelWithHelp summary={t("notifierPollIntervalSummary")} detail={t("notifierPollIntervalHelp")}>
                <span>{t("notifierPollInterval")}</span>
              </FieldLabelWithHelp>
              <input
                value={draft.notifier_poll_interval || DEFAULT_NOTIFIER_POLL_INTERVAL}
                placeholder={t("notifierPollIntervalPlaceholder")}
                onInput={(event) => patch({ notifier_poll_interval: event.currentTarget.value })}
              />
            </label>
          </>
        ) : null}
      </div>
    </section>
  );
}
