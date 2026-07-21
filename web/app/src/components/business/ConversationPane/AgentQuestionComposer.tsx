import { useEffect, useState } from "react";
import { ArrowRight, ChevronLeft, ChevronRight, CircleHelp, Clock3, LockKeyhole, Pencil, X } from "lucide-react";
import { Button, IconButton } from "@/components/ui";
import { questionOptions } from "@/models/agentActivity";
import { resolveUserByLocalIdentity, type TranslateFn, type UsersById } from "@/models/conversations";
import type { QuestionAnswerMode } from "./useQuestionAnswerMode";

const recommendedSuffix = " (Recommended)";

export function AgentQuestionComposer({
  mode,
  t,
  usersById,
}: {
  mode: QuestionAnswerMode;
  t: TranslateFn;
  usersById?: UsersById;
}) {
  if (mode.pending.length === 0) {
    return null;
  }
  if (!mode.selected) {
    return (
      <section className="question-composer question-chooser" aria-label={t("questionChooseRequest")}>
        <div className="question-composer-heading">
          <CircleHelp aria-hidden="true" size={18} />
          <strong>{t("questionSeveralWaiting", { count: mode.pending.length })}</strong>
        </div>
        <div className="question-request-options">
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
        <p className="question-composer-hint">{t("questionSelectHint")}</p>
      </section>
    );
  }

  const request = mode.selected.content.question!;
  const question = request.questions[mode.questionIndex];
  const options = questionOptions(question);
  const currentAnswer = mode.answers[question.id] ?? {};
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
    <section className="question-composer" aria-label={t("questionAnswerMode")}>
      <header className="question-composer-header">
        <h2>{question.question}</h2>
        <nav className="question-composer-navigation" aria-label={t("questionNavigation")}>
          <QuestionExpirationCountdown autoResolveAt={request.auto_resolve_at} t={t} />
          <IconButton
            disabled={mode.busy || mode.questionIndex === 0}
            icon={<ChevronLeft size={18} />}
            label={t("questionPrevious")}
            size="sm"
            variant="tertiaryColor"
            onClick={mode.previousQuestion}
          />
          <span role="status" aria-live="polite">
            {t("questionProgressCompact", { current: mode.questionIndex + 1, total: request.questions.length })}
          </span>
          <IconButton
            disabled={mode.busy || finalQuestion}
            icon={<ChevronRight size={18} />}
            label={t("questionNext")}
            size="sm"
            variant="tertiaryColor"
            onClick={mode.nextQuestion}
          />
          <IconButton
            disabled={mode.busy}
            icon={<X size={19} />}
            label={t("questionClose")}
            size="sm"
            variant="tertiaryColor"
            onClick={mode.closeRequest}
          />
        </nav>
      </header>

      {options.length > 0 ? (
        <div className="question-composer-options" role="radiogroup" aria-label={question.question}>
          {options.map((option, index) => {
            const optionIndex = index + 1;
            const selected = currentAnswer.optionIndex === optionIndex;
            const recommended = option.label.endsWith(recommendedSuffix);
            const visibleLabel = recommended ? option.label.slice(0, -recommendedSuffix.length) : option.label;
            return (
              <button
                key={`${optionIndex}:${option.label}`}
                type="button"
                role="radio"
                aria-checked={selected}
                className={`question-composer-option ${selected ? "selected" : ""}`}
                disabled={mode.busy}
                onClick={() => mode.chooseOption(optionIndex)}
              >
                <span className="question-option-number">{optionIndex}</span>
                <span className="question-option-copy">
                  <span className="question-option-title">
                    <strong>{visibleLabel}</strong>
                    {recommended ? (
                      <span className="question-recommended-badge">{t("questionRecommended")}</span>
                    ) : null}
                  </span>
                  {option.description ? <small>{option.description}</small> : null}
                </span>
                <ArrowRight className="question-option-arrow" aria-hidden="true" size={20} />
              </button>
            );
          })}
        </div>
      ) : null}

      {showFreeform ? (
        <div className="question-composer-input-row">
          <span className="question-input-icon">
            {question.is_secret ? (
              <LockKeyhole aria-hidden="true" size={18} />
            ) : (
              <Pencil aria-hidden="true" size={18} />
            )}
          </span>
          <input
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
        <div className="form-error question-composer-error" role="alert">
          {mode.error}
        </div>
      ) : null}
    </section>
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
      className={`question-composer-countdown ${remainingSeconds === 0 ? "expired" : ""}`}
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
