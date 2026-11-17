import agentQuestionComposerStyles from "./AgentQuestionComposer.module.css";
import { useEffect, useState, type ReactNode } from "react";
import { CircleHelp, Clock3, LockKeyhole, Pencil, X } from "lucide-react";
import { InlineError, TextInput, Button, IconButton, QuestionCard } from "@/components/ui";
import { questionOptions } from "@/models/agentActivity";
import { resolveUserByLocalIdentity, type TranslateFn, type UsersById } from "@/models/conversations";
import type { QuestionAnswerMode } from "./useQuestionAnswerMode";

const recommendedSuffix = " (Recommended)";

export function AgentQuestionComposer({
  mode,
  t,
  usersById,
  statusSlot,
}: {
  mode: QuestionAnswerMode;
  t: TranslateFn;
  usersById?: UsersById;
  statusSlot?: ReactNode;
}) {
  if (mode.pending.length === 0) {
    return null;
  }
  if (!mode.selected) {
    return (
      <section
        className={`${agentQuestionComposerStyles["question-composer"]} question-chooser`}
        aria-label={t("questionChooseRequest")}
      >
        {statusSlot}
        <div className={agentQuestionComposerStyles["question-composer-heading"]}>
          <CircleHelp aria-hidden="true" size={18} />
          <strong>{t("questionSeveralWaiting", { count: mode.pending.length })}</strong>
        </div>
        <div className={agentQuestionComposerStyles["question-request-options"]}>
          {mode.pending.map((activity) => {
            const request = activity.content.question!;
            const first = request.questions[0];
            const askingAgent =
              (usersById ? resolveUserByLocalIdentity(activity.sender, usersById)?.name : "") || activity.sender;
            return (
              <Button key={request.id} variant="secondaryColor" onClick={() => mode.select(request.id)}>
                {askingAgent || t("questionAgent")}: {first?.header || t("questionRequest")}
              </Button>
            );
          })}
        </div>
        <p className={agentQuestionComposerStyles["question-composer-hint"]}>{t("questionSelectHint")}</p>
      </section>
    );
  }

  const request = mode.selected.content.question!;
  const question = request.questions[mode.questionIndex];
  const options = questionOptions(question);
  const selectedOption = mode.answers[question.id]?.optionIndex;
  const finalQuestion = mode.questionIndex === request.questions.length - 1;
  const showFreeform = question.is_other || options.length === 0;
  const hasText = mode.text.trim().length > 0;

  function advanceFreeform() {
    if (!hasText) {
      mode.skipQuestion();
      return;
    }
    if (finalQuestion) {
      mode.submitAnswers();
    } else {
      mode.nextQuestion();
    }
  }

  return (
    <div className={agentQuestionComposerStyles["question-composer-card"]}>
      {statusSlot}
      <QuestionCard
        fullWidth
        title={question.header || t("questionRequest")}
        accessibleLabel={t("questionAnswerMode")}
        questions={request.questions.map((item) => ({
          id: item.id,
          prompt: item.question,
          options: questionOptions(item).map(({ label, description }) => {
            const recommended = label.endsWith(recommendedSuffix);
            return {
              label: recommended ? label.slice(0, -recommendedSuffix.length) : label,
              description,
              badge: recommended ? t("questionRecommended") : undefined,
            };
          }),
        }))}
        labels={{
          answer: t("questionAnswer"),
          navigation: t("questionNavigation"),
          previous: t("questionPrevious"),
          next: t("questionNext"),
          progress: (current, total) => t("questionProgressCompact", { current, total }),
        }}
        pagination={{
          index: mode.questionIndex,
          onChange: (index) => (index < mode.questionIndex ? mode.previousQuestion() : mode.nextQuestion()),
        }}
        selection={selectedOption === undefined ? null : { questionId: question.id, optionIndex: selectedOption - 1 }}
        disabled={mode.busy}
        optionMode="radio"
        showAnswer={false}
        letterShortcuts={false}
        onAnswer={(_, index) => {
          if (index !== undefined) mode.chooseOption(index + 1);
        }}
        footerMeta={<QuestionExpirationCountdown autoResolveAt={request.auto_resolve_at} t={t} />}
        footerActions={
          <IconButton
            disabled={mode.busy}
            icon={<X size={16} />}
            label={t("questionClose")}
            size="sm"
            variant="tertiaryColor"
            onClick={mode.closeRequest}
          />
        }
      >
        {showFreeform ? (
          <div className={agentQuestionComposerStyles["question-composer-input-row"]}>
            <span className={agentQuestionComposerStyles["question-input-icon"]}>
              {question.is_secret ? (
                <LockKeyhole aria-hidden="true" size={18} />
              ) : (
                <Pencil aria-hidden="true" size={18} />
              )}
            </span>
            <TextInput
              size="sm"
              key={question.id}
              autoFocus
              type={question.is_secret ? "password" : "text"}
              autoComplete="off"
              value={mode.text}
              disabled={mode.busy}
              aria-label={question.is_secret ? t("questionSecretAnswer") : t("questionFreeformAnswer")}
              placeholder={question.is_secret ? t("questionSecretPlaceholder") : t("questionTypeAnswer")}
              onChange={(event) => mode.setText(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.nativeEvent.isComposing) {
                  event.preventDefault();
                  advanceFreeform();
                }
              }}
            />
            <Button disabled={mode.busy} variant={hasText ? "primary" : "tertiaryGray"} onClick={advanceFreeform}>
              {hasText ? t("questionNext") : t("questionSkip")}
            </Button>
          </div>
        ) : null}

        {mode.error ? (
          <InlineError className={`${agentQuestionComposerStyles["question-composer-error"]}`} role="alert">
            {mode.error}
          </InlineError>
        ) : null}
      </QuestionCard>
    </div>
  );
}

function QuestionExpirationCountdown({ autoResolveAt, t }: { autoResolveAt?: string; t: TranslateFn }) {
  const [remainingSeconds, setRemainingSeconds] = useState<number | null>(() =>
    secondsUntil(autoResolveAt, Date.now()),
  );

  useEffect(() => {
    const deadline = parseDeadline(autoResolveAt);
    if (deadline === null) {
      setRemainingSeconds(null);
      return;
    }

    let timer: number | undefined;
    const tick = () => {
      const next = Math.max(0, Math.ceil((deadline - Date.now()) / 1_000));
      setRemainingSeconds(next);
      if (next > 0) {
        timer = window.setTimeout(tick, Math.min(1_000, Math.max(1, deadline - Date.now())));
      }
    };
    tick();
    return () => {
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
    };
  }, [autoResolveAt]);

  if (remainingSeconds === null) {
    return null;
  }
  const time = formatCountdown(remainingSeconds);
  const label = t("questionExpiresIn", { time });
  return (
    <span
      className={`${agentQuestionComposerStyles["question-composer-countdown"]} ${remainingSeconds === 0 ? "expired" : ""}`}
      role="timer"
      aria-label={label}
      title={label}
    >
      <Clock3 aria-hidden="true" size={14} />
      <span>{time}</span>
    </span>
  );
}

function secondsUntil(autoResolveAt: string | undefined, now: number): number | null {
  const deadline = parseDeadline(autoResolveAt);
  return deadline === null ? null : Math.max(0, Math.ceil((deadline - now) / 1_000));
}

function parseDeadline(autoResolveAt: string | undefined): number | null {
  const deadline = Date.parse(autoResolveAt || "");
  return Number.isFinite(deadline) ? deadline : null;
}

function formatCountdown(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}
