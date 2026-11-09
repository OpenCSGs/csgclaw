import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import type { CSSProperties } from "react";
import { Field, TextInput } from "@/components/ui";
import palette from "../figmaColors.json";
import utility from "../figmaUtilityColors.json";
import tokenSource from "@/shared/styles/tokens.css?raw";
import styles from "./ColorFoundations.module.css";

const familyNames: Record<string, string> = {
  基础: "Base",
  "灰色（亮）": "Gray light",
  "灰色（暗）": "Gray dark",
  品牌: "Brand",
  错误: "Error",
  警告: "Warning",
  成功: "Success",
  灰蓝色的: "Gray blue",
  绿色: "Green",
  蓝色: "Blue",
  黄色: "Yellow",
};
const tokenFamilies: Record<string, string> = {
  基础: "",
  "灰色（亮）": "gray",
  "灰色（暗）": "gray-dark",
  品牌: "brand",
  错误: "error",
  警告: "warning",
  成功: "success",
  灰蓝色的: "gray-blue",
  绿色: "green",
  蓝色: "blue",
  黄色: "yellow",
};
const rootTokens = new Map(
  [...tokenSource.split("}")[0].matchAll(/(--[\w-]+):\s*(#[\da-f]{6,8});/gi)].map((m) => [m[1], m[2].toLowerCase()]),
);
const primitiveEntries = Object.entries(palette.palettes).filter(([name]) => {
  const parts = name.split("/");
  return parts[1] === "基础" || /^\d+$/.test(parts[2]);
});
const semanticEntries = Object.entries(palette.semantic).filter(([name]) => name.split("/")[1] !== "基础");
const sections = [
  { title: "基础色板 · Primitives", entries: primitiveEntries },
  { title: "语义颜色 · Semantic references", entries: semanticEntries },
  { title: "组件工具色 · Utility references", entries: Object.entries(utility) },
];

function mapping(name: string, value: string, text: (source: string) => string) {
  const [, family, shade] = name.split("/");
  if (!primitiveEntries.some(([key]) => key === name)) return text("语义映射待确认");
  const prefix = tokenFamilies[family] ?? family.toLowerCase().replaceAll(" ", "-").replace("rosé", "rose");
  const token = family === "基础" ? `--${shade}` : `--${prefix}-${shade}`;
  const current = rootTokens.get(token);
  return !current
    ? `${text("待映射：")}${token}`
    : current === value.toLowerCase()
      ? `${text("一致：")}${token}`
      : `${text("差异：")}${token} = ${current}`;
}

export function ColorFoundations() {
  const { text: labText } = useLabTranslation();

  const [query, setQuery] = useState("");
  const search = query.trim().toLowerCase();
  return (
    <section id="design-foundations" className={styles.foundation} aria-labelledby="color-title">
      <header>
        <p className={styles.eyebrow}>{labText("FOUNDATIONS / 设计基础")}</p>
        <h2 id="color-title">{labText("Colors · 颜色")}</h2>
        <p>
          {labText("Figma 变量快照 · ")}
          {palette.captured} · {primitiveEntries.length}
          {labText(" 个基础色 / ")}
          {semanticEntries.length}
          {labText(" 个语义引用 / ")}
          {Object.keys(utility).length}
          {labText(" 个工具色")}
        </p>
        <a href={palette.source} target="_blank" rel="noreferrer">
          {labText("查看 Figma Colors 设计来源 ↗")}
        </a>
      </header>
      <div className={styles.notice}>
        <strong>{labText("变量值优先，差异保留待确认")}</strong>
        <p>
          {labText(
            "设计稿部分色块的 HEX 文案和实际绑定变量不一致（如 Brand 25 文案为 #FCFAFF，变量为 #F0F3FF）。这里展示工具读取的绑定变量值，不沿用静态文案或对比度标注。",
          )}
        </p>
        <p>
          {labText(
            "语义与工具色为节点解析快照，可能混合不同模式，尚未形成完整亮暗主题映射。主题切换只改变展示容器，不改变这些参考值。Gradients 及说明文档尚待单独整理；未修改产品全局 token。",
          )}
        </p>
      </div>
      <Field label={labText("搜索颜色 / Search colors")} hint={labText("支持 Figma 名称、色阶、HEX 与代码 token。")}>
        <TextInput
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder={labText("例如 Brand、品牌、600、#3250bd")}
          type="search"
        />
      </Field>
      {sections.map(({ title, entries }) => {
        const groups = new Map<string, [string, string][]>();
        for (const [name, value] of entries) {
          const family = name.startsWith(labText("组件颜色/基础/")) ? name.split("/")[2] : name.split("/")[1];
          const label = `${family} ${familyNames[family] ?? ""}`;
          if (search && !`${name} ${label} ${value} ${mapping(name, value, labText)}`.toLowerCase().includes(search))
            continue;
          groups.set(label, [...(groups.get(label) ?? []), [name, value]]);
        }
        return (
          <section key={title} className={styles.section}>
            <h3>{labText(title)}</h3>
            {groups.size === 0 && <p role="status">{labText("没有匹配的颜色 / No matching colors")}</p>}
            {[...groups].map(([family, colors]) => (
              <details key={family} open={search ? true : undefined} className={styles.group}>
                <summary>
                  {family} <span>· {colors.length}</span>
                </summary>
                <div className={styles.swatches}>
                  {colors
                    .sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true }))
                    .map(([name, value]) => (
                      <article key={name} className={styles.swatch}>
                        <div className={styles.checker} aria-hidden="true">
                          <div className={styles.fill} style={{ "--swatch": value } as CSSProperties} />
                        </div>
                        <div className={styles.caption}>
                          <strong>{name.split("/").at(-1)}</strong>
                          <code>{value.toUpperCase()}</code>
                          <small>{name}</small>
                          <small>{mapping(name, value, labText)}</small>
                        </div>
                      </article>
                    ))}
                </div>
              </details>
            ))}
          </section>
        );
      })}
    </section>
  );
}
