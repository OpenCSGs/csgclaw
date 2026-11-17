import type { CSSProperties } from "react";
import { ThemeGlyph } from "../ThemeGlyph";
import { applicationAsset } from "./assets";
import type { EmptyIconVariant, EmptySize } from "./EmptyDemo";
import styles from "./EmptyDemo.module.css";

type Illustration = {
  width: number;
  height: number;
  badge: { top: number; size: number; glyph: string };
  cloud: readonly [asset: string, x: number, y: number];
  layers: readonly (readonly [asset: string, x: number, y: number])[];
};

const illustrations: Record<EmptySize, Illustration> = {
  sm: {
    cloud: ["4e243", 19, 15],
    width: 152,
    height: 118,
    badge: { top: 62, size: 48, glyph: "017fd" },
    layers: [
      ["1450f", 24, 0],
      ["a49d5", 16, 14],
      ["df969", 11, 102],
      ["df969", 138, 28],
      ["a6dee", 130, 4],
    ],
  },
  md: {
    cloud: ["91572", 15, 15],
    width: 172,
    height: 128,
    badge: { top: 60, size: 56, glyph: "84108" },
    layers: [
      ["3ad76", 22, 0],
      ["cfe34", 14, 14],
      ["55a82", 9, 104],
      ["55a82", 152, 28],
      ["a49d5", 144, 4],
    ],
  },
  lg: {
    cloud: ["a0730", 23, 15],
    width: 220,
    height: 160,
    badge: { top: 84, size: 56, glyph: "84108" },
    layers: [
      ["7aeaf", 30, 0],
      ["55a82", 18, 12],
      ["cfe34", 192, 120],
      ["bba40", 15, 128],
      ["bba40", 200, 36],
      ["df969", 184, 4],
    ],
  },
};

function svg(id: string) {
  return applicationAsset(`empty-states-${id}.svg`);
}

export function EmptyStateIcon({ variant, size }: { variant: EmptyIconVariant; size: EmptySize }) {
  switch (variant) {
    case "folder":
      return (
        <span className={styles.folder}>
          <img className={styles.back} src={applicationAsset("empty-states-imgBackMain.svg")} alt="" />
          <span className={styles.paper} />
          <span className={styles.blur} />
          <img className={styles.front} src={applicationAsset("empty-states-imgFrontMain.svg")} alt="" />
          <img className={styles.noise} src={applicationAsset("empty-states-folder-front-noise.svg")} alt="" />
        </span>
      );
    case "file":
      return (
        <span className={styles.fileHalo}>
          <span className={styles.fileIcon}>
            <img src={svg("05102")} alt="" />
            <img className={styles.fileMark} src={svg("20d29")} alt="" />
          </span>
        </span>
      );
    case "featured":
      return (
        <span className={styles.featured}>
          <ThemeGlyph className={styles.featuredGlyph} src={svg(size === "lg" ? "e8e88" : "59897")} />
        </span>
      );
    case "illustration": {
      const { width, height, badge, layers, cloud } = illustrations[size];
      const [cloudAsset, cloudLeft, cloudTop] = cloud;
      return (
        <span className={styles.illustration} style={{ width, height }}>
          {layers.map(([asset, x, y]) => (
            <img
              className={styles.illustrationCircle}
              key={`${asset}-${x}`}
              src={svg(asset)}
              alt=""
              style={{ left: x, top: y }}
            />
          ))}
          <img
            className={styles.illustrationCloud}
            src={svg(cloudAsset)}
            alt=""
            style={
              {
                left: cloudLeft,
                top: cloudTop,
                "--empty-light-cloud": `url("${svg(`${cloudAsset}-light`)}")`,
              } as CSSProperties
            }
          />
          <span className={styles.illustrationBadge} style={{ top: badge.top, width: badge.size, height: badge.size }}>
            <img src={svg(badge.glyph)} alt="" />
          </span>
        </span>
      );
    }
  }
}
