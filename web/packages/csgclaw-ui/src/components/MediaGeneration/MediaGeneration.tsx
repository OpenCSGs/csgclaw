import type { CSSProperties, ReactNode } from "react";
import glyph from "../ConversationGlyph.module.css";
import styles from "./MediaGeneration.module.css";

const layouts = {
  square: { columns: 8, rows: 8, maxWidth: 420 },
  standard: { columns: 8, rows: 6, maxWidth: 420 },
  wide: { columns: 12, rows: 7, maxWidth: 420 },
  portrait: { columns: 6, rows: 10, maxWidth: 236 },
};

const aspectRatios = new Map([
  [1, "1 / 1"],
  [4 / 3, "4 / 3"],
  [16 / 9, "16 / 9"],
  [9 / 16, "9 / 16"],
]);

export interface MediaGenerationProps {
  kind: "image" | "video";
  state: "queued" | "pending" | "completed" | "failed";
  label: string;
  aspectRatio?: number;
  progress?: number;
  result?: ReactNode;
  prompt?: string;
  promptDisclosureLabel?: string;
  busy?: boolean;
  failureMessage?: string;
  details?: ReactNode;
  feedback?: ReactNode;
  actions?: ReactNode;
  variations?: readonly { label: string; onSelect?: () => void }[];
}

export function MediaGeneration({
  kind,
  state,
  label,
  aspectRatio = kind === "image" ? 1 : 16 / 9,
  progress,
  result,
  prompt,
  promptDisclosureLabel,
  busy,
  failureMessage,
  details,
  feedback,
  actions,
  variations,
}: MediaGenerationProps) {
  const ratio =
    Number.isFinite(aspectRatio) && aspectRatio > 0 ? aspectRatio : 16 / 9;
  const { columns, rows, maxWidth } =
    ratio < 1
      ? layouts.portrait
      : ratio === 1
        ? layouts.square
        : ratio <= 4 / 3
          ? layouts.standard
          : layouts.wide;
  const value =
    progress !== undefined && Number.isFinite(progress)
      ? Math.min(100, Math.max(0, progress))
      : undefined;
  const frames =
    state === "completed" && !result && variations?.length
      ? variations
      : [undefined];

  return (
    <div
      className={styles.card}
      data-state={state}
      style={
        {
          "--media-aspect-ratio": aspectRatios.get(ratio) ?? ratio,
          "--frame-width": `${maxWidth}px`,
        } as CSSProperties
      }
    >
      {prompt ? (
        <div className={styles.header}>
          <span className={`${glyph.glyph} ${styles.glyph}`} aria-hidden="true">
            ✦
          </span>
          {promptDisclosureLabel ? (
            <details className={styles.prompt}>
              <summary>{promptDisclosureLabel}</summary>
              <div>{prompt}</div>
            </details>
          ) : (
            <div className={styles.prompt}>{prompt}</div>
          )}
        </div>
      ) : null}
      <div
        className={styles.frames}
        data-variations={frames.length > 1 ? "" : undefined}
      >
        {frames.map((variation, index) => {
          const Frame = variation?.onSelect ? "button" : "div";
          return (
            <Frame
              className={styles.frame}
              data-result={result ? "" : undefined}
              key={index}
              type={Frame === "button" ? "button" : undefined}
              aria-label={variation?.label}
              onClick={variation?.onSelect}
            >
              {result ? (
                <div className={styles.result}>{result}</div>
              ) : (
                <div
                  aria-hidden="true"
                  className={styles.tiles}
                  style={{
                    gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
                    gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))`,
                  }}
                >
                  {Array.from({ length: columns * rows }, (_, tile) => (
                    <span
                      key={tile}
                      style={{
                        animationDelay: `${(Math.floor(tile / columns) + (tile % columns)) * 90}ms`,
                      }}
                    />
                  ))}
                </div>
              )}
              {state === "queued" && !result ? (
                <div className={styles.overlay}>
                  <span className={styles.dot} aria-hidden="true" />
                  {label}
                </div>
              ) : null}
              {state === "failed" && !result ? (
                <div className={`${styles.overlay} ${styles.failure}`}>
                  {failureMessage && failureMessage !== label ? (
                    <span>{failureMessage}</span>
                  ) : (
                    <span role="status" aria-busy={busy}>
                      {label}
                    </span>
                  )}
                  {actions}
                </div>
              ) : null}
              {variation ? (
                <span className={styles.variation} aria-hidden="true">
                  {index + 1}
                </span>
              ) : null}
            </Frame>
          );
        })}
      </div>
      <div className={styles.footer}>
        {state === "pending" ? (
          <div
            className={styles.progress}
            role="progressbar"
            aria-label={label}
            aria-valuenow={value === undefined ? undefined : Math.round(value)}
          >
            <span
              style={value === undefined ? undefined : { width: `${value}%` }}
            />
          </div>
        ) : null}
        <div className={styles.statusLine}>
          {state !== "failed" ||
          result ||
          (failureMessage && failureMessage !== label) ? (
            <span role="status" aria-busy={busy}>
              {label}
            </span>
          ) : null}
          {state === "pending" && value !== undefined ? (
            <span className={styles.percent}>{Math.round(value)}%</span>
          ) : null}
          {state === "completed" ? actions : null}
        </div>
        {state === "failed" && result ? (
          <>
            {failureMessage && failureMessage !== label ? (
              <div>{failureMessage}</div>
            ) : null}
            {actions}
          </>
        ) : null}
        {details}
        {feedback}
      </div>
    </div>
  );
}
