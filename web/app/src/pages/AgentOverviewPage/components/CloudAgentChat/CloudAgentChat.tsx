import { useEffect, useRef, useState } from "react";
import { Send } from "lucide-react";
import { Button, TextArea } from "@/components/ui";
import type { CloudAgent } from "@/models/cloudAgents";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIErrorWithDiagnostics } from "@/shared/i18n";
import { useCloudAgentChat } from "./useCloudAgentChat";
import styles from "./CloudAgentChat.module.css";

export function CloudAgentChat({ agent, t }: { agent: CloudAgent; t: TranslateFn }) {
  const chat = useCloudAgentChat(agent);
  const [draft, setDraft] = useState("");
  const input = useRef<HTMLTextAreaElement>(null);
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => {
    input.current?.focus();
  }, []);
  useEffect(() => {
    end.current?.scrollIntoView({ block: "nearest" });
  }, [chat.state.messages.length, chat.busy]);
  const submit = () => {
    if (!draft.trim() || chat.busy || chat.state.pending || agent.status !== "online") return;
    chat.send(draft);
    setDraft("");
  };
  return (
    <section className={styles.chat} aria-label={t("cloudChatTitle")}>
      <header>
        <h2>{t("cloudChatTitle")}</h2>
        <Button variant="secondaryGray" size="sm" disabled={chat.busy} onClick={chat.reset}>
          {t("cloudChatNew")}
        </Button>
      </header>
      <div className={styles.messages} role="log" aria-label={t("cloudChatMessages")} aria-live="polite">
        {!chat.state.messages.length ? (
          <p className={styles.hint}>{t("cloudChatEmpty", { name: agent.name })}</p>
        ) : null}
        {chat.state.messages.map((message) => (
          <article key={message.id} className={styles.message} data-role={message.role}>
            <strong>{message.role === "user" ? t("cloudChatYou") : agent.name}</strong>
            <p>{message.text}</p>
          </article>
        ))}
        {chat.busy ? (
          <p role="status" className={styles.hint}>
            {t("cloudChatWaiting")}
          </p>
        ) : null}
        <div ref={end} />
      </div>
      {chat.error ? (
        <div role="alert" className={styles.error}>
          {localizeAPIErrorWithDiagnostics(chat.error, t, t("cloudChatFailed"))}
          {chat.state.pending ? (
            <Button size="sm" variant="secondaryGray" disabled={chat.busy} onClick={chat.retry}>
              {t("retry")}
            </Button>
          ) : null}
        </div>
      ) : null}
      {chat.storageError ? (
        <p role="alert" className={styles.hint}>
          {t("cloudChatStorageError")}
        </p>
      ) : null}
      {agent.status !== "online" ? (
        <p role="status" className={styles.hint}>
          {t("cloudChatOffline")}
        </p>
      ) : null}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <TextArea
          ref={input}
          aria-label={t("cloudChatInput")}
          placeholder={t("cloudChatPlaceholder")}
          value={draft}
          maxLength={64000}
          rows={3}
          disabled={chat.busy || Boolean(chat.state.pending) || agent.status !== "online"}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
              event.preventDefault();
              submit();
            }
          }}
        />
        <div className={styles.send}>
          <small>{t("cloudChatKeyboard")}</small>
          <Button
            type="submit"
            size="sm"
            variant="primary"
            loading={chat.busy}
            disabled={!draft.trim() || chat.busy || Boolean(chat.state.pending) || agent.status !== "online"}
          >
            <Send size={14} />
            {t("cloudChatSend")}
          </Button>
        </div>
      </form>
    </section>
  );
}
