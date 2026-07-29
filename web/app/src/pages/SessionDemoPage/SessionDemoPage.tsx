import { Bot, Check, Copy, MessageSquareText, Plus, RotateCcw, Send, ShieldCheck, Square } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button, Field, Select, TextArea, TextInput } from "@/components/ui";
import { createTranslator, detectInitialLocale } from "@/shared/i18n";
import { detectInitialTheme, resolveThemeMode } from "@/shared/theme/theme";
import { useSessionDemo } from "@/pages/SessionDemoPage/useSessionDemo";
import styles from "./SessionDemoPage.module.css";

export function SessionDemoPage() {
  const locale = useMemo(detectInitialLocale, []);
  const t = useMemo(() => createTranslator(locale), [locale]);
  const session = useSessionDemo(t);
  const [copied, setCopied] = useState("");
  const messagesRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const theme = detectInitialTheme();
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme = resolveThemeMode(theme);
    };
    apply();
    if (theme === "system") media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, []);

  useEffect(() => {
    if (session.messages.length === 0 && !session.pendingInput) return;
    const messages = messagesRef.current;
    if (!messages) return;
    if (typeof messages.scrollTo === "function") {
      messages.scrollTo({ top: messages.scrollHeight, behavior: "smooth" });
    } else {
      messages.scrollTop = messages.scrollHeight;
    }
  }, [session.messages.length, session.pendingInput]);

  async function copyValue(name: string, value: string) {
    if (!value) return;
    await navigator.clipboard.writeText(value);
    setCopied(name);
    window.setTimeout(() => setCopied((current) => (current === name ? "" : current)), 1400);
  }

  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <header className={styles.hero}>
          <div className={styles.brandMark} aria-hidden="true">
            <MessageSquareText size={24} />
          </div>
          <div className={styles.heroCopy}>
            <div className={styles.eyebrow}>{t("sessionDemoEyebrow")}</div>
            <h1>{t("sessionDemoTitle")}</h1>
            <p>{t("sessionDemoSubtitle")}</p>
          </div>
          <div className={styles.liveBadge}>
            <span aria-hidden="true" />
            {t("sessionDemoLiveAPI")}
          </div>
        </header>

        <div className={styles.workspace}>
          <aside className={styles.setupCard} aria-label={t("sessionDemoSetup")}>
            <div className={styles.cardHeading}>
              <div>
                <span className={styles.stepLabel}>01</span>
                <h2>{t("sessionDemoSetup")}</h2>
              </div>
              <Bot size={20} aria-hidden="true" />
            </div>

            <Field label={t("sessionDemoAgent")} hint={session.agentLocked ? t("sessionDemoAgentLocked") : undefined}>
              <Select
                aria-label={t("sessionDemoAgent")}
                disabled={session.agentLocked || session.agentsLoading}
                value={session.agentId}
                onValueChange={session.selectAgent}
                placeholder={session.agentsLoading ? t("sessionDemoAgentsLoading") : t("sessionDemoSelectAgent")}
                options={session.agents.map((agent) => ({
                  value: agent.id,
                  label: agent.name,
                  description: agent.id,
                }))}
                searchable
                searchPlaceholder={t("sessionDemoSearchAgents")}
              />
            </Field>
            {session.agentsError ? <div className={styles.inlineError}>{session.agentsError}</div> : null}

            <Field label={t("sessionDemoSessionID")} error={session.sessionError || undefined}>
              <div className={styles.sessionInputRow}>
                <TextInput
                  aria-label={t("sessionDemoSessionID")}
                  value={session.sessionDraft}
                  disabled={session.busy}
                  spellCheck={false}
                  onChange={(event) => session.setSessionDraft(event.currentTarget.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") session.reuseSession();
                  }}
                />
                <Button
                  iconOnly
                  aria-label={t("sessionDemoCopySessionID")}
                  title={t("sessionDemoCopySessionID")}
                  onClick={() => copyValue("session", session.sessionId)}
                >
                  {copied === "session" ? <Check size={16} /> : <Copy size={16} />}
                </Button>
              </div>
            </Field>

            <div className={styles.sessionActions}>
              <Button variant="secondaryGray" onClick={session.reuseSession} disabled={session.busy}>
                <RotateCcw size={15} aria-hidden="true" />
                {t("sessionDemoReuseSession")}
              </Button>
              <Button variant="secondaryColor" onClick={session.newSession} disabled={session.busy}>
                <Plus size={15} aria-hidden="true" />
                {t("sessionDemoNewSession")}
              </Button>
            </div>

            <div className={styles.auditBlock}>
              <div className={styles.auditHeader}>
                <span>{t("sessionDemoRoomName")}</span>
                <Button
                  iconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t("sessionDemoCopyRoomName")}
                  title={t("sessionDemoCopyRoomName")}
                  disabled={!session.roomTitle}
                  onClick={() => copyValue("room", session.roomTitle)}
                >
                  {copied === "room" ? <Check size={14} /> : <Copy size={14} />}
                </Button>
              </div>
              <code>{session.roomTitle || t("sessionDemoSelectAgentFirst")}</code>
              {session.roomId ? <small>{t("sessionDemoRoomCreated", { roomId: session.roomId })}</small> : null}
            </div>

            <div className={styles.identityNotice}>
              <ShieldCheck size={18} aria-hidden="true" />
              <div>
                <strong>{t("sessionDemoAnonymousAsAdmin")}</strong>
                <span>{t("sessionDemoAnonymousAsAdminHint")}</span>
              </div>
            </div>
          </aside>

          <section className={styles.chatCard} aria-label={t("sessionDemoConversation")}>
            <header className={styles.chatHeader}>
              <div className={styles.agentAvatar}>{session.selectedAgent?.name.slice(0, 1).toUpperCase() || "A"}</div>
              <div>
                <h2>{session.selectedAgent?.name || t("sessionDemoNoAgent")}</h2>
                <span>{session.sessionId}</span>
              </div>
              <div className={styles.chatState} data-busy={session.busy || undefined}>
                <span aria-hidden="true" />
                {session.busy ? t("sessionDemoWaiting") : t("sessionDemoReady")}
              </div>
            </header>

            <div ref={messagesRef} className={styles.messages} aria-live="polite" aria-busy={session.busy}>
              {session.messages.length === 0 && !session.pendingInput ? (
                <div className={styles.emptyState}>
                  <div className={styles.emptyIcon}>
                    <MessageSquareText size={26} aria-hidden="true" />
                  </div>
                  <h3>{t("sessionDemoEmptyTitle")}</h3>
                  <p>{t("sessionDemoEmptyBody")}</p>
                </div>
              ) : null}
              {session.messages.map((message) => (
                <article key={message.id} className={styles.message} data-role={message.role}>
                  <span>{message.role === "user" ? t("sessionDemoAdminLabel") : session.selectedAgent?.name}</span>
                  <div>{message.content}</div>
                </article>
              ))}
              {session.pendingInput ? (
                <article className={styles.message} data-role="user" data-pending="true">
                  <span>{t("sessionDemoAdminLabel")}</span>
                  <div>{session.pendingInput}</div>
                </article>
              ) : null}
              {session.busy ? (
                <div className={styles.thinking} role="status">
                  <span />
                  <span />
                  <span />
                  {t("sessionDemoAgentWorking")}
                </div>
              ) : null}
            </div>

            <div className={styles.composerArea}>
              {session.requestError ? (
                <div className={styles.requestError} role="alert">
                  <span>{session.requestError}</span>
                  {!session.busy && session.draft.trim() ? (
                    <Button size="sm" variant="tertiaryDanger" onClick={session.send}>
                      {t("retry")}
                    </Button>
                  ) : null}
                </div>
              ) : null}
              <form
                className={styles.composer}
                onSubmit={(event) => {
                  event.preventDefault();
                  session.send();
                }}
              >
                <TextArea
                  aria-label={t("sessionDemoMessage")}
                  placeholder={t("sessionDemoMessagePlaceholder")}
                  value={session.draft}
                  disabled={session.busy || !session.selectedAgent}
                  rows={3}
                  onChange={(event) => session.setDraft(event.currentTarget.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && !event.shiftKey) {
                      event.preventDefault();
                      session.send();
                    }
                  }}
                />
                <div className={styles.composerFooter}>
                  <span>{t("sessionDemoComposerHint")}</span>
                  {session.busy ? (
                    <Button variant="outlineDanger" onClick={session.cancel}>
                      <Square size={13} fill="currentColor" aria-hidden="true" />
                      {t("sessionDemoCancel")}
                    </Button>
                  ) : (
                    <Button type="submit" variant="primary" disabled={!session.draft.trim() || !session.selectedAgent}>
                      <Send size={15} aria-hidden="true" />
                      {t("sessionDemoSend")}
                    </Button>
                  )}
                </div>
              </form>
            </div>
          </section>
        </div>
      </div>
    </main>
  );
}
