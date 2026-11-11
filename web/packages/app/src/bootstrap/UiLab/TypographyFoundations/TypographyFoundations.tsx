import { useLabTranslation } from "../LabLocale";
import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { Field, TextInput, SegmentedControl } from "@/components/ui";
import reference from "../figmaTypography.json";
import tokenSource from "@/shared/styles/tokens.css?raw";
import styles from "./TypographyFoundations.module.css";

const scaleTokens: Record<string, string> = {
  "display-2xl": "text-7xl",
  "display-xl": "text-6xl",
  "display-lg": "text-5xl",
  "display-md": "text-4xl",
  "display-sm": "text-3xl",
  "display-xs": "text-2xl",
};
const weightTokens: Record<string, string> = {
  Light: "light",
  Regular: "normal",
  Medium: "medium",
  Semibold: "semibold",
};
const scales = [...new Set(reference.styles.map((item) => item.scale))].sort(
  (a, b) =>
    reference.styles.find((item) => item.scale === b)!.size - reference.styles.find((item) => item.scale === a)!.size,
);
const sampleDefault = "设计让协作更清晰 · The quick brown fox 0123456789";
function literal(token: string) {
  return tokenSource.match(new RegExp(`${token}:\\s*([^;]+);`))?.[1] ?? "未定义";
}

export function TypographyFoundations() {
  const { text: labText } = useLabTranslation();

  const [sample, setSample] = useState(sampleDefault);
  const [mode, setMode] = useState("design");
  const [fontStatus, setFontStatus] = useState("正在检查本地 PingFang SC 字体…");
  useEffect(() => {
    let cancelled = false;
    if (typeof FontFace === "undefined") {
      setFontStatus("未找到本地 PingFang SC，当前使用系统字体回退，字形与字重不代表设计还原。");
      return;
    }
    const font = new FontFace("UiLabPingFangProbe", 'local("PingFang SC"), local("PingFangSC-Regular")', {
      weight: "400",
    });
    font
      .load()
      .then(() => {
        if (cancelled) return;
        setFontStatus("本地 PingFang SC 已加载；各字重仍需设计验收。");
      })
      .catch(() => {
        if (!cancelled) setFontStatus("未找到本地 PingFang SC，当前使用系统字体回退，字形与字重不代表设计还原。");
      });
    return () => {
      cancelled = true;
    };
  }, []);
  return (
    <section id="typography-foundations" className={styles.foundation} aria-labelledby="typography-title">
      <p className={styles.eyebrow}>{labText("FOUNDATIONS / 设计基础")}</p>
      <h2 id="typography-title">{labText("Typography · 排版")}</h2>
      <p>
        {labText("Figma 样式快照 · ")}
        {reference.captured}
        {labText(" · 11 个字号层级 / 44 个文字样式")}
      </p>
      <a href={reference.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Typography 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("实际样式与页面说明存在差异")}</strong>
        <p>
          {labText(
            "实际文字样式为 PingFang SC，Light 300 / Regular 400 / Medium 500 / Semibold 600；页面静态说明写着 Inter 与 Regular 400 / Medium 500 / Semibold 600 / Bold 700。大标题静态说明的 -2% 字距与导出样式的 0 也不一致。预览使用导出的文字样式，不将静态标签视为确认后的规范。",
          )}
        </p>
        <p role="status">{labText(fontStatus)}</p>
        <p>
          {labText(
            "项目未内置 PingFang SC 字体文件。本页仅尝试加载本机字体，不下载字体或修改全局文字 token。字号与行高一致不代表字体、字重及字距已经对齐。",
          )}
        </p>
      </div>
      <div className={styles.controls}>
        <Field label={labText("预览文案 / Preview text")} hint={labText("清空后恢复默认中英文及数字示例。")}>
          <TextInput value={sample} onChange={(event) => setSample(event.target.value)} />
        </Field>
        <SegmentedControl
          value={mode}
          onChange={setMode}
          label={labText("Typography preview mode")}
          options={[
            { value: "design", label: labText("Figma 参考") },
            { value: "code", label: labText("现有代码") },
          ]}
        />
      </div>
      {scales.map((scale) => {
        const variants = reference.styles.filter((item) => item.scale === scale).sort((a, b) => a.weight - b.weight);
        const first = variants[0];
        const prefix = scaleTokens[scale] ?? scale;
        return (
          <section key={scale} className={styles.scale} aria-label={scale}>
            <h3>{scale}</h3>
            <p>
              {labText("Figma ")}
              {first.size}
              {labText("px / ")}
              {first.lineHeight}
              {labText("px · 字距 ")}
              {first.letterSpacing}
              {labText("px")}
            </p>
            <p className={styles.mapping}>
              {labText("候选映射：--")}
              {prefix}
              {labText("-size = ")}
              {labText(literal(`--${prefix}-size`))} / --{prefix}
              {labText("-line-height =")} {labText(literal(`--${prefix}-line-height`))} / --{prefix}
              {labText("-tracking = ")}
              {labText(literal(`--${prefix}-tracking`))}
            </p>
            <div className={styles.variants}>
              {variants.map((item) => {
                const weightName = item.name.split("/")[1];
                const weight = weightTokens[weightName];
                const token = `--${prefix}-${weight}`;
                const style: CSSProperties =
                  mode === "design"
                    ? {
                        fontFamily: '"PingFang SC", var(--font-family-text)',
                        fontSize: item.size,
                        lineHeight: `${item.lineHeight}px`,
                        fontWeight: item.weight,
                        letterSpacing: item.letterSpacing,
                      }
                    : { font: `var(${token})`, letterSpacing: `var(--${prefix}-tracking)` };
                return (
                  <article key={item.name} className={styles.variant}>
                    <h4>
                      {weightName} · {item.weight}
                    </h4>
                    <p className={styles.sample} style={style}>
                      {sample || sampleDefault}
                    </p>
                    <code>{token}</code>
                    <small>
                      {labText("代码字重：")}
                      {literal(`--font-weight-${weight}`)}
                      {labText(" · Figma：")}
                      {item.weight}
                    </small>
                    <small>
                      {labText("代码 font：")}
                      {literal(token)}
                    </small>
                  </article>
                );
              })}
            </div>
          </section>
        );
      })}
    </section>
  );
}
