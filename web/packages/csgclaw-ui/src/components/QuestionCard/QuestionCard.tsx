import { ChevronLeft, ChevronRight, Clock3, LockKeyhole } from "lucide-react";
import { useState } from "react";
import glyph from "../ConversationGlyph.module.css";
import styles from "./QuestionCard.module.css";

export interface QuestionCardItem {
  id: string;
  heading?: string;
  prompt: string;
  privateAnswerLabel?: string;
  options: readonly { label: string; description?: string }[];
  resolution?: string;
}

export interface QuestionCardProps {
  title: string;
  status: string;
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
  status,
  deadline,
  selection,
  questions,
  labels,
  onAnswer,
}: QuestionCardProps) {
  const [index, setIndex] = useState(0);
  const currentIndex = Math.min(index, questions.length - 1);
  const current = questions[currentIndex];
  const options = current?.options ?? [];

  function choose(optionIndex: number) {
    if (!current || !onAnswer) return;
    onAnswer(current.id, optionIndex);
  }

  return (
    <section
      className={styles.card}
      data-question-card=""
      aria-label={title}
      onKeyDown={(event) => {
        if (
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
        <span className={styles.status} role="status">
          {status}
        </span>
      </header>
      {current ? (
        <div className={styles.body}>
          <div className={styles.prompt}>{current.prompt}</div>
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
              role="group"
              aria-label={current.prompt}
            >
              {options.map((option, optionIndex) => (
                <button
                  type="button"
                  className={styles.option}
                  key={optionIndex}
                  aria-pressed={
                    selection !== undefined
                      ? selection?.questionId === current.id &&
                        selection.optionIndex === optionIndex
                      : undefined
                  }
                  disabled={!onAnswer}
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
                  </span>
                  <span className={styles.indicator} aria-hidden="true" />
                </button>
              ))}
            </div>
          ) : null}
          {current.resolution ? (
            <div className={styles.resolution}>{current.resolution}</div>
          ) : null}
        </div>
      ) : null}
      {current || deadline ? (
        <footer className={styles.footer}>
          <div className={styles.meta}>
            {current ? (
              <span aria-live="polite">
                {labels.progress(currentIndex + 1, questions.length)}
              </span>
            ) : null}
            {deadline ? (
              <span className={styles.deadline}>
                <Clock3 size={12} aria-hidden="true" />
                {deadline}
              </span>
            ) : null}
          </div>
          {questions.length > 1 ? (
            <nav className={styles.navigation} aria-label={labels.navigation}>
              <button
                type="button"
                className={styles.navButton}
                aria-label={labels.previous}
                title={labels.previous}
                disabled={currentIndex === 0}
                onClick={() => setIndex(currentIndex - 1)}
              >
                <ChevronLeft size={14} aria-hidden="true" />
              </button>
              <button
                type="button"
                className={styles.navButton}
                aria-label={labels.next}
                title={labels.next}
                disabled={currentIndex === questions.length - 1}
                onClick={() => setIndex(currentIndex + 1)}
              >
                <ChevronRight size={14} aria-hidden="true" />
              </button>
            </nav>
          ) : null}
          {current && onAnswer ? (
            <button
              type="button"
              className={styles.answer}
              onClick={() => onAnswer(current.id)}
            >
              {labels.answer}
            </button>
          ) : null}
        </footer>
      ) : null}
    </section>
  );
}
