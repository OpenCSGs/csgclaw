import type { CSSProperties } from "react";
import type snapshot from "../figmaProgress.json";
import styles from "./ProgressFoundations.module.css";

const assets = import.meta.glob<string>("./assets/*.svg", { eager: true, query: "?url", import: "default" });
const rings: Record<string, string> = {
  "md-Circle": "6e790.svg",
  "lg-Circle": "9ba14.svg",
  "sm-Circle": "39931.svg",
  "xs-Circle": "8ec24.svg",
  "md-Half circle": "207c6.svg",
  "lg-Half circle": "7a70c.svg",
  "sm-Half circle": "45f47.svg",
  "xs-Half circle": "7d043.svg",
};
const metrics: Record<
  string,
  { font: number; line: number; labelFont: number; labelLine: number; halfCenter: number }
> = {
  xs: { font: 24, line: 32, labelFont: 12, labelLine: 18, halfCenter: 61 },
  sm: { font: 30, line: 38, labelFont: 12, labelLine: 18, halfCenter: 78 },
  md: { font: 36, line: 44, labelFont: 14, labelLine: 20, halfCenter: 96 },
  lg: { font: 48, line: 60, labelFont: 14, labelLine: 20, halfCenter: 110 },
};
const image = (file: string, style?: CSSProperties) => <img src={assets[`./assets/${file}`]} alt="" style={style} />;

export function ProgressCircleReference({ entry }: { entry: (typeof snapshot.circles)[number] }) {
  const half = entry.shape === "Half circle";
  const tiny = entry.size === "xxs";
  const metric = metrics[entry.size];
  return (
    <div
      className={styles.circle}
      data-size={entry.size}
      data-shape={entry.shape}
      style={{ width: entry.width, height: entry.height }}
      aria-hidden="true"
    >
      {tiny ? (
        <>
          {image(half ? "931d0.svg" : "e87a9.svg")}
          {image(half ? "ca024.svg" : "30695.svg", { left: half ? 0 : 29, top: half ? 1.4186 : 0 })}
          <span className={styles.tinyNumber} style={{ left: half ? 32 : 31.5, top: half ? 15 : 22 }}>
            40%
          </span>
          <span className={styles.tinyLabel} style={{ top: half ? 35 : 68 }}>
            Users
          </span>
        </>
      ) : (
        <>
          {image(rings[`${entry.size}-${entry.shape}`], { left: entry.size === "lg" && !half ? -1 : 0 })}
          <div className={styles.circleCopy} style={{ top: half ? metric.halfCenter : entry.height / 2 }}>
            <span
              className={styles.circleLabel}
              style={{ fontSize: metric.labelFont, lineHeight: `${metric.labelLine}px` }}
            >
              Active users
            </span>
            <span className={styles.circleNumber} style={{ fontSize: metric.font, lineHeight: `${metric.line}px` }}>
              40%
            </span>
          </div>
        </>
      )}
    </div>
  );
}
