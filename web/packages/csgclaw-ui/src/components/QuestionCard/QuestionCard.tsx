import { ChevronLeft, ChevronRight, Clock3, LockKeyhole } from "lucide-react";
import { useState, type ReactNode } from "react";
import glyph from "../ConversationGlyph.module.css";
import styles from "./QuestionCard.module.css";

export interface QuestionCardItem {
  id: string;
  heading?: string;
  prompt: string;
  privateAnswerLabel?: string;
  options: readonly { label: string; description?: string; badge?: string }[];
  resolution?: string;
}

export interface QuestionCardProps {
  title: string;
  fullWidth?: boolean;
  status?: string;
  accessibleLabel?: string;
  children?: ReactNode;
  footerMeta?: ReactNode;
  footerActions?: ReactNode;
  pagination?: { index: number; onChange: (index: number) => void };
  disabled?: boolean;
  optionMode?: "button" | "radio";
  showAnswer?: boolean;
  letterShortcuts?: boolean;
  deadline?: string;
  selection?: { questionId: string; optionIndex: number } | null;
  questions: readonly QuestionCardItem[];
  labels: {
    answer: string;
    navigation: string;
    previous: string;
    next: string;
    progress: (current: number, total: number) => string;
  };
  /** The option index is zero-based. An omitted index opens the answer flow for the current question. */
  onAnswer?: (questionId: string, optionIndex?: number) => void;
}

export function QuestionCard({
  title,
  fullWidth = false,
  status,
  accessibleLabel = title,
  children,
  footerMeta,
  footerActions,
  pagination,
  disabled = false,
  optionMode = "button",
  showAnswer = true,
  letterShortcuts = true,
  deadline,
  selection,
  questions,
  labels,
  onAnswer,
}: QuestionCardProps) {
  const [index, setIndex] = useState(0);
  const currentIndex = Math.min(
    pagination?.index ?? index,
    questions.length - 1,
  );
  const current = questions[currentIndex];
  const options = current?.options ?? [];
  const radioOptions = optionMode === "radio";
  const navigate = pagination?.onChange ?? setIndex;
  const answerVisible = current && onAnswer && showAnswer;

  function choose(optionIndex: number) {
    if (disabled || !current || !onAnswer) return;
    onAnswer(current.id, optionIndex);
  }

  return (
    <section
      className={fullWidth ? `${styles.card} ${styles.fullWidth}` : styles.card}
      data-question-card=""
      aria-label={accessibleLabel}
      onKeyDown={(event) => {
        if (
          !letterShortcuts ||
          (event.target instanceof HTMLElement &&
            (event.target.isContentEditable ||
              event.target.closest("input, textarea, select"))) ||
          event.altKey ||
          event.ctrlKey ||
          event.metaKey ||
          event.shiftKey ||
          event.nativeEvent.isComposing
        )
          return;
        if (!onAnswer || !/^[a-z]$/i.test(event.key)) return;
        const optionIndex = event.key.toUpperCase().charCodeAt(0) - 65;
        if (optionIndex >= options.length) return;
        event.preventDefault();
        choose(optionIndex);
      }}
    >
      <header className={styles.header}>
        <span className={`${glyph.glyph} ${styles.glyph}`} aria-hidden="true">
          ✦
        </span>
        <span className={styles.title}>{title}</span>
        {status ? (
          <span className={styles.status} role="status">
            {status}
          </span>
        ) : null}
      </header>
      {current ? (
        <div className={styles.body}>
          <h2 className={styles.prompt}>{current.prompt}</h2>
          {current.heading || current.privateAnswerLabel ? (
            <div className={styles.context}>
              {current.privateAnswerLabel ? (
                <LockKeyhole
                  size={12}
                  role="img"
                  aria-label={current.privateAnswerLabel}
                />
              ) : null}
              {current.heading}
            </div>
          ) : null}
          {options.length ? (
            <div
              className={styles.options}
              role={radioOptions ? "radiogroup" : "group"}
              aria-label={current.prompt}
            >
              {options.map((option, optionIndex) => {
                const selected =
                  selection?.questionId === current.id &&
                  selection.optionIndex === optionIndex;
                return (
                  <button
                    type="button"
                    className={styles.option}
                    key={optionIndex}
                    role={radioOptions ? "radio" : undefined}
                    aria-checked={radioOptions ? selected : undefined}
                    aria-pressed={
                      !radioOptions && selection !== undefined
                        ? selected
                        : undefined
                    }
                    disabled={disabled || !onAnswer}
                    onClick={() => choose(optionIndex)}
                  >
                    <kbd className={styles.marker} aria-hidden="true">
                      {optionIndex < 26
                        ? String.fromCharCode(65 + optionIndex)
                        : optionIndex + 1}
                    </kbd>
                    <span className={styles.optionCopy}>
                      <span className={styles.optionLabel}>{option.label}</span>
                      {option.description ? (
                        <span className={styles.description}>
                          {option.description}
                        </span>
                      ) : null}
                      {option.badge ? (
                        <span className={styles.badge}>{option.badge}</span>
                      ) : null}
                    </span>
                    <span className={styles.indicator} aria-hidden="true" />
                  </button>
                );
              })}
            </div>
          ) : null}
          {current.resolution ? (
            <div className={styles.resolution}>{current.resolution}</div>
          ) : null}
        </div>
      ) : null}
      {children}
      {current || deadline || footerMeta || footerActions ? (
        <footer className={styles.footer}>
          <div className={styles.meta}>
            {current ? (
              <span role="status">
                {labels.progress(currentIndex + 1, questions.length)}
              </span>
            ) : null}
            {deadline ? (
              <span className={styles.deadline}>
                <Clock3 size={12} aria-hidden="true" />
                {deadline}
              </span>
            ) : null}
            {footerMeta}
          </div>
          {questions.length > 1 || answerVisible || footerActions ? (
            <div className={styles.actions}>
              {questions.length > 1 ? (
                <nav
                  className={styles.navigation}
                  aria-label={labels.navigation}
                >
                  <button
                    type="button"
                    className={styles.navButton}
                    aria-label={labels.previous}
                    title={labels.previous}
                    disabled={disabled || currentIndex === 0}
                    onClick={() => navigate(currentIndex - 1)}
                  >
                    <ChevronLeft size={14} aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    className={styles.navButton}
                    aria-label={labels.next}
                    title={labels.next}
                    disabled={disabled || currentIndex === questions.length - 1}
                    onClick={() => navigate(currentIndex + 1)}
                  >
                    <ChevronRight size={14} aria-hidden="true" />
                  </button>
                </nav>
              ) : null}
              {answerVisible ? (
                <button
                  type="button"
                  className={styles.answer}
                  disabled={disabled}
                  onClick={() => onAnswer(current.id)}
                >
                  {labels.answer}
                </button>
              ) : null}
              {footerActions}
            </div>
          ) : null}
        </footer>
      ) : null}
    </section>
  );
}
