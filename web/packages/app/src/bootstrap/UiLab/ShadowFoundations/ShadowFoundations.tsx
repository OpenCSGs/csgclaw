import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import type { CSSProperties } from "react";
import { SegmentedControl } from "@/components/ui";
import snapshot from "../figmaShadows.json";
import tokenSource from "@/shared/styles/tokens.css?raw";
import styles from "./ShadowFoundations.module.css";

const groups = [...new Set(snapshot.styles.map((item) => item.group))];
function codeShadow(name: string) {
  return tokenSource.match(new RegExp(`--${name}:\\s*([^;]+);`))?.[1];
}

export function ShadowFoundations() {
  const { text: labText } = useLabTranslation();

  const [mode, setMode] = useState("design");
  return (
    <section id="shadow-foundations" className={styles.foundation} aria-labelledby="shadow-title">
      <p className={styles.eyebrow}>{labText("FOUNDATIONS / 设计基础")}</p>
      <h2 id="shadow-title">{labText("Shadows · 阴影与焦点环")}</h2>
      <p>
        {labText("Figma 效果快照 · ")}
        {snapshot.captured}
        {labText(" · 7 级阴影 / 7 种焦点环组合")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Shadows 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("参考效果与现有 token 对照")}</strong>
        <p>
          {labText(
            "每层 CSS 依次为 X 偏移、Y 偏移、模糊半径、扩散半径、颜色；inset 表示内阴影。八位 HEX 的最后两位为透明度。",
          )}
        </p>
        <p>
          {labText(
            "Figma 参考保留设计中的白色表面、边框和已解析颜色，主题切换仅影响展示容器。现有代码模式使用同名 token，缺失项明确标记；不使用相近值代替。",
          )}
        </p>
        <p>
          {labText(
            "焦点环为常显效果样本，不是可点击控件。本次不修改全局 shadow 或 focus token；Blurs 与长篇说明不在本次阴影展示范围。",
          )}
        </p>
      </div>
      <SegmentedControl
        value={mode}
        onChange={setMode}
        label={labText("Shadow preview mode")}
        options={[
          { value: "design", label: labText("Figma 参考") },
          { value: "code", label: labText("现有代码") },
        ]}
      />
      {groups.map((group) => (
        <section key={group} className={styles.section} aria-label={group}>
          <h3>{group}</h3>
          <div className={styles.grid}>
            {snapshot.styles
              .filter((item) => item.group === group)
              .map((item) => {
                const current = codeShadow(item.name);
                const missing = mode === "code" && !current;
                const effect = mode === "design" ? item.css : current ? `var(--${item.name})` : "none";
                return (
                  <article key={item.name} className={styles.item} aria-label={item.name}>
                    <h4>{item.name}</h4>
                    <div className={styles.stage}>
                      <div
                        className={styles.sample}
                        style={
                          {
                            "--sample-shadow": effect,
                            "--sample-border": item.group === "Shadows" ? "#00000014" : "#d5d7da",
                          } as CSSProperties
                        }
                      >
                        {missing ? labText("未定义同名 token") : item.name}
                      </div>
                    </div>
                    <p>{current ? labText("存在同名 token，参数需对照") : labText("待映射：代码尚无同名 token")}</p>
                    <details>
                      <summary>{labText("Figma CSS 参数")}</summary>
                      <code>{item.css}</code>
                    </details>
                    <details>
                      <summary>
                        {labText("代码 --")}
                        {item.name}
                      </summary>
                      <code>{current ?? labText("未定义")}</code>
                    </details>
                  </article>
                );
              })}
          </div>
        </section>
      ))}
    </section>
  );
}
