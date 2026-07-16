import { ChevronLeft, ChevronRight, CircleHelp, LockKeyhole } from "lucide-react";
import { Button, IconButton } from "@/components/ui";
import { questionOptions } from "@/models/agentActivity";
import { resolveUserByLocalIdentity, type TranslateFn, type UsersById } from "@/models/conversations";
import type { QuestionAnswerMode } from "./useQuestionAnswerMode";

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

  return (
    <section className="question-composer" aria-label={t("questionAnswerMode")}>
      <div className="question-composer-heading">
        <CircleHelp aria-hidden="true" size={18} />
        <div>
          <strong>{question.header}</strong>
          <span role="status" aria-live="polite">
            {t("questionProgress", { current: mode.questionIndex + 1, total: request.questions.length })}
          </span>
        </div>
        {request.questions.length > 1 ? (
          <nav className="question-composer-navigation" aria-label={t("questionNavigation")}>
            <IconButton
              disabled={mode.busy || mode.questionIndex === 0}
              icon={<ChevronLeft size={18} />}
              label={t("questionPrevious")}
              size="sm"
              variant="tertiaryColor"
              onClick={mode.previousQuestion}
            />
            <IconButton
              disabled={mode.busy || finalQuestion}
              icon={<ChevronRight size={18} />}
              label={t("questionNext")}
              size="sm"
              variant="tertiaryColor"
              onClick={mode.nextQuestion}
            />
          </nav>
        ) : null}
      </div>
      <p className="question-composer-prompt">{question.question}</p>
      {options.length > 0 ? (
        <div className="question-composer-options" role="radiogroup" aria-label={question.question}>
          {options.map((option, index) => {
            const optionIndex = index + 1;
            const selected = currentAnswer.option_index === optionIndex;
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
                <span>
                  <strong>
                    {question.is_other && optionIndex === options.length ? t("questionOther") : option.label}
                  </strong>
                  {option.description ? <small>{option.description}</small> : null}
                </span>
              </button>
            );
          })}
        </div>
      ) : null}
      <label className="question-composer-input-label">
        <span>
          {question.is_secret ? <LockKeyhole aria-hidden="true" size={15} /> : null}
          {question.is_secret ? t("questionSecretAnswer") : t("questionFreeformAnswer")}
        </span>
        <input
          key={question.id}
          autoFocus
          type={question.is_secret ? "password" : "text"}
          autoComplete="off"
          value={mode.text}
          disabled={mode.busy}
          placeholder={options.length > 0 ? t("questionOptionalNote") : t("questionTypeAnswer")}
          onChange={(event) => mode.setText(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.nativeEvent.isComposing) {
              event.preventDefault();
              if (finalQuestion) {
                mode.submitAnswers();
              } else {
                mode.nextQuestion();
              }
            }
          }}
        />
      </label>
      {mode.error ? (
        <div className="form-error question-composer-error" role="alert">
          {mode.error}
        </div>
      ) : null}
      <div className="question-composer-actions">
        <Button disabled={mode.busy} variant="tertiaryGray" onClick={mode.skipQuestion}>
          {t("questionSkipThis")}
        </Button>
        <div className="question-composer-primary-actions">
          {mode.skipConfirmation ? (
            <>
              <span role="alert">{t("questionSkipAllConfirm")}</span>
              <Button disabled={mode.busy} variant="outlineDanger" onClick={mode.continueWithoutAnswering}>
                {t("questionConfirmContinue")}
              </Button>
              <Button disabled={mode.busy} variant="tertiaryGray" onClick={() => mode.setSkipConfirmation(false)}>
                {t("cancel")}
              </Button>
            </>
          ) : (
            <>
              <Button disabled={mode.busy} variant="tertiaryGray" onClick={() => mode.setSkipConfirmation(true)}>
                {t("questionContinueWithout")}
              </Button>
              <Button disabled={mode.busy} variant="primary" onClick={mode.submitAnswers}>
                {mode.busy ? t("loading") : t("questionSubmit")}
              </Button>
            </>
          )}
        </div>
      </div>
    </section>
  );
}
