import type { CSSProperties } from "react";
import type snapshot from "../figmaProgress.json";
import styles from "./ProgressFoundations.module.css";

const assets = import.meta.glob<string>("./assets/**/*.svg", { eager: true, query: "?url", import: "default" });
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
function ringImage(light: string, dark: string, style?: CSSProperties) {
  return (
    <>
      <img src={assets[`./assets/${light}`]} alt="" style={style} data-ring-theme="light" />
      <img src={assets[`./assets/dark/${dark}`]} alt="" style={style} data-ring-theme="dark" />
    </>
  );
}

export function ProgressCircleReference({ entry }: { entry: (typeof snapshot.circles)[number] }) {
  const half = entry.shape === "Half circle";
  const shape = half ? "half" : "circle";
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
          {ringImage(half ? "931d0.svg" : "e87a9.svg", `xxs-${shape}-background.svg`)}
          {ringImage(half ? "ca024.svg" : "30695.svg", `xxs-${shape}-line.svg`, {
            left: half ? 0 : 29,
            top: half ? 1.4186 : 0,
          })}
          <span className={styles.tinyNumber} style={{ left: half ? 32 : 31.5, top: half ? 15 : 22 }}>
            40%
          </span>
          <span className={styles.tinyLabel} style={{ top: half ? 35 : 68 }}>
            Users
          </span>
        </>
      ) : (
        <>
          {ringImage(rings[`${entry.size}-${entry.shape}`], `${entry.size}-${shape}-ring.svg`, {
            left: entry.size === "lg" && !half ? -1 : 0,
          })}
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
