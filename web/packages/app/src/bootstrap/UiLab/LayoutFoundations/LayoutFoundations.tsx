import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { SegmentedControl } from "@/components/ui";
import tokenSource from "@/shared/styles/tokens.css?raw";
import snapshot from "../figmaLayout.json";
import styles from "./LayoutFoundations.module.css";

const codeTokens = [...tokenSource.matchAll(/--((?:space|radius)-[\w-]+):\s*([\d.]+)px;/g)].map(([, name, value]) => ({
  name,
  value: Number(value),
}));
const groups = [
  { value: "spacing", label: "语义间距" },
  { value: "primitives", label: "间距标尺" },
  { value: "radius", label: "圆角" },
  { value: "width", label: "宽度与容器" },
  { value: "grids", label: "网格" },
] as const;
type Group = (typeof groups)[number]["value"];

export function LayoutFoundations() {
  const { text: labText } = useLabTranslation();

  const [group, setGroup] = useState<Group>("spacing");
  const [gridName, setGridName] = useState("Desktop");
  const grid = snapshot.grids.find((item) => item.name === gridName) ?? snapshot.grids[0];
  const items = group === "grids" ? [] : snapshot[group];
  return (
    <section id="layout-foundations" className={styles.foundation} aria-labelledby="layout-title">
      <p className={styles.eyebrow}>{labText("FOUNDATIONS / 设计基础")}</p>
      <h2 id="layout-title">{labText("Spacing, radius & grids · 间距、圆角与网格")}</h2>
      <p>
        {labText("Figma 参数快照 · ")}
        {snapshot.captured}
        {labText(" · 32 级标尺 / 17 级语义间距 / 11 级圆角 / 8 种网格")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma 布局设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("先建立映射，再迁移组件")}</strong>
        <p>
          {labText(
            "同名 token 与等值候选分别列出；等值仅供映射参考，不代表用途一致。本次只扩充设计基础，不修改产品全局 token。",
          )}
        </p>
        <p>
          {labText("命名冲突：Figma radius-xs = 4px、radius-sm = 6px；代码同名值为 2px、4px。6px 间距尚无 space-1-5。")}
        </p>
      </div>
      <div className={styles.controls}>
        <SegmentedControl
          value={group}
          onChange={(value) => setGroup(value as Group)}
          label={labText("Layout foundation category")}
          options={groups.map((item) => ({ ...item, label: labText(item.label) }))}
        />
      </div>
      {group === "grids" ? (
        <div className={styles.section}>
          <h3>{labText("Grid layouts · 网格预览")}</h3>
          <div className={styles.controls}>
            <SegmentedControl
              value={gridName}
              onChange={setGridName}
              label={labText("Grid preset")}
              options={snapshot.grids.map((item) => ({ value: item.name, label: item.name }))}
            />
          </div>
          <p role="status">
            {grid.width}
            {labText("px 画布 · ")}
            {grid.columns}
            {labText(" 列 · 列间距 ")}
            {grid.gap}
            {labText("px · 单侧边距 ")}
            {grid.margin}
            {labText("px · 列宽")}{" "}
            {Number(((grid.width - grid.margin * 2 - grid.gap * (grid.columns - 1)) / grid.columns).toFixed(2))}
            {labText("px")}
          </p>
          <p>
            {labText(
              "按 1 CSS px 展示，窄窗口可在预览内横向滚动。网格叠层为示意配色，参数来自 Figma layoutGrids；预设宽度不是产品响应式断点。",
            )}
          </p>
          <div className={styles.scroll} role="region" aria-label={labText("网格原尺寸预览")} tabIndex={0}>
            <div className={styles.canvas} style={{ width: grid.width, paddingInline: grid.margin }}>
              <div
                className={styles.columns}
                style={{ gridTemplateColumns: `repeat(${grid.columns}, minmax(0, 1fr))`, gap: grid.gap }}
              >
                {Array.from({ length: grid.columns }, (_, i) => (
                  <div key={i} className={styles.column}>
                    {i + 1}
                  </div>
                ))}
              </div>
            </div>
          </div>
          <p>
            {labText(
              "Desktop：1440px 画布中居中的 1280px 容器，加上两侧各 32px 内边距，列起点距离画布边缘 112px。Tablet 为 6 列，Mobile 为 4 列。",
            )}
          </p>
          <p>{labText("本预览展示列网格；设计稿额外的行网格和边距辅助层未叠加。")}</p>
          <a
            href={`${snapshot.source.split("?")[0]}?node-id=${grid.node.replace(":", "-")}`}
            target="_blank"
            rel="noreferrer"
          >
            {labText("查看当前网格节点 ↗")}
          </a>
        </div>
      ) : (
        <div className={styles.section}>
          <h3>{labText(groups.find((item) => item.value === group)?.label ?? group)}</h3>
          <p>
            {group === "radius"
              ? labText("圆角以原始半径显示，full 在矩形上呈胶囊形。")
              : labText("色条按 1 CSS px 显示，超出卡片时局部滚动；0px 仅显示起点边界。")}
          </p>
          <div className={styles.grid}>
            {items.map((item) => {
              const name = group === "primitives" ? `space-${item.name.replace(".", "-")}` : item.name;
              const current = codeTokens.find((token) => token.name === name);
              const candidates = codeTokens.filter(
                (token) =>
                  token.value === item.value && token.name.startsWith(group === "radius" ? "radius-" : "space-"),
              );
              return (
                <article className={styles.item} key={item.name} aria-label={item.name}>
                  <h4>{group === "primitives" ? `间距 / ${item.name}` : item.name}</h4>
                  <strong>
                    {item.value}
                    {labText("px")}
                  </strong>
                  {group === "radius" ? (
                    <div className={styles.radius} style={{ borderRadius: item.value }} aria-hidden="true" />
                  ) : (
                    <div className={styles.ruler} role="region" aria-label={`${item.name} 原尺寸标尺`} tabIndex={0}>
                      <div className={styles.bar} style={{ width: item.value }} />
                    </div>
                  )}
                  <p>
                    {labText("同名代码 --")}
                    {name}：
                    {current
                      ? `${current.value}px · ${current.value === item.value ? labText("一致") : labText("数值冲突")}`
                      : labText("未定义")}
                  </p>
                  <p>
                    {labText("等值候选：")}
                    {candidates.length ? candidates.map((token) => `--${token.name}`).join(" / ") : labText("暂无")}
                  </p>
                </article>
              );
            })}
          </div>
        </div>
      )}
    </section>
  );
}
