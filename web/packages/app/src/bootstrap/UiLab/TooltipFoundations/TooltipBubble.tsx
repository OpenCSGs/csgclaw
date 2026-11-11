import { arrowAssets, defaultTooltipTitle, defaultTooltipDescription } from "./tooltipData";
import styles from "./TooltipFoundations.module.css";

export function TooltipBody({
  supporting,
  title = defaultTooltipTitle,
  description = defaultTooltipDescription,
}: {
  supporting: boolean;
  title?: string;
  description?: string;
}) {
  return (
    <div className={styles.body} data-supporting={supporting}>
      <span className={styles.title}>{title || defaultTooltipTitle}</span>
      {supporting && <span className={styles.description}>{description || defaultTooltipDescription}</span>}
    </div>
  );
}

export function TooltipBubble({
  arrow,
  supporting,
  width,
  height,
}: {
  arrow: string;
  supporting: boolean;
  width: number;
  height: number;
}) {
  const slot =
    arrow !== "None" ? (
      <span className={styles.arrowSlot}>
        <span className={styles.arrowRotation}>
          <img src={arrowAssets[arrow]} alt="" />
        </span>
      </span>
    ) : null;
  return (
    <div className={styles.bubble} data-arrow={arrow} style={{ width, height }}>
      {(arrow === "Top center" || arrow === "Left") && slot}
      <TooltipBody supporting={supporting} />
      {arrow !== "Top center" && arrow !== "Left" && slot}
    </div>
  );
}
