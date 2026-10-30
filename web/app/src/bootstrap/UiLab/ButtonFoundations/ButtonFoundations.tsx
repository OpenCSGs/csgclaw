import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import type { CSSProperties } from "react";
import { Button, SegmentedControl } from "@/components/ui";
import type { ButtonSize, ButtonVariant } from "@/components/ui/Button";
import snapshot from "../figmaButtons.json";
import styles from "./ButtonFoundations.module.css";

type Entry = (typeof snapshot.entries)[number];
const assets = import.meta.glob<string>("./assets/*.svg", { eager: true, query: "?url", import: "default" });
const families = [
  { value: "standard", label: "普通按钮" },
  { value: "destructive", label: "危险操作" },
  { value: "utility", label: "工具按钮" },
  { value: "close", label: "关闭按钮" },
];
const stateLabels: Record<string, string> = {
  Default: "默认",
  Hover: "悬浮",
  Focused: "焦点",
  Disabled: "禁用",
  Loading: "加载",
};
const states = ["Default", "Hover", "Focused", "Disabled", "Loading"];
const verifiedCount = snapshot.entries.filter((item) => item.verified).length;

function codeVariant(item: Entry): ButtonVariant {
  if (item.family === "destructive") {
    return ({ Primary: "danger", Secondary: "outlineDanger", Tertiary: "tertiaryDanger", Link: "linkDanger" } as const)[
      item.hierarchy as "Primary" | "Secondary" | "Tertiary" | "Link"
    ];
  }
  return (
    {
      Primary: "primary",
      Secondary: "secondaryGray",
      Tertiary: "tertiaryGray",
      "Link color": "linkColor",
      "Link gray": "linkGray",
      Close: "tertiaryGray",
    } as const
  )[item.hierarchy as "Primary" | "Secondary" | "Tertiary" | "Link color" | "Link gray" | "Close"];
}

function ReferenceButton({ item }: { item: Entry }) {
  const { text: labText } = useLabTranslation();

  const ref = item.reference;
  if (!ref) return <p className={styles.pending}>{labText("样式待读取 · 仅已确认节点与尺寸")}</p>;
  const missing = ref.images.some((name) => !assets[`./assets/${name}`]);
  if (missing) return <p className={styles.pending}>{labText("设计图标下载待完成，暂不展示不完整样本")}</p>;
  const style: CSSProperties = {
    width: item.width,
    minWidth: item.width,
    height: item.height,
    minHeight: item.height,
    padding: 0,
    gap: ref.gap,
    border: `${ref.borderWidth}px solid ${ref.borderColor}`,
    borderRadius: ref.radius,
    background: ref.background,
    color: ref.color,
    boxShadow: ref.shadow,
    fontFamily: '"PingFang SC", var(--font-family-text)',
    fontSize: ref.fontSize,
    lineHeight: ref.lineHeight,
    fontWeight: 500,
    transition: "none",
    cursor: "default",
  };
  return (
    <div inert className={styles.reference} data-button-reference={item.id} data-dark-background={item.darkBackground}>
      <Button style={style} variant="tertiaryGray" tabIndex={-1} aria-label={labText("Figma 静态参考")}>
        {ref.images[0] && <img src={assets[`./assets/${ref.images[0]}`]} alt="" />}
        {ref.label && <span style={{ paddingInline: ref.textPadding }}>{ref.label}</span>}
        {ref.images[1] && <img src={assets[`./assets/${ref.images[1]}`]} alt="" />}
      </Button>
    </div>
  );
}

function CodeButton({ item, onClick }: { item: Entry; onClick: () => void }) {
  const { text: labText } = useLabTranslation();

  if (item.size === "xs") return <p>{labText("现有 Button 无 xs 尺寸，待映射")}</p>;
  if (!item.reference) return <p>{labText("待取得参考内容后对照")}</p>;
  const imageName = item.reference?.images[0];
  const image = imageName && assets[`./assets/${imageName}`];
  return (
    <Button
      variant={codeVariant(item)}
      size={item.size as ButtonSize}
      iconOnly={item.iconOnly}
      disabled={item.state === "Disabled"}
      loading={item.state === "Loading"}
      loadingLabel={labText("Submitting...")}
      aria-label={`${labText("代码样本")} ${item.family} ${item.hierarchy} ${item.state} ${item.darkBackground ? labText("深色") : labText("普通")}`}
      onClick={onClick}
    >
      {image && item.state !== "Loading" && <img src={image} alt="" />}
      {!item.iconOnly && labText("Button CTA")}
      {!item.iconOnly && item.state !== "Loading" && item.reference?.images[1] && (
        <img src={assets[`./assets/${item.reference.images[1]}`]} alt="" />
      )}
    </Button>
  );
}

export function ButtonFoundations() {
  const { text: labText, locale } = useLabTranslation();

  const [family, setFamily] = useState("standard");
  const [size, setSize] = useState("md");
  const [shape, setShape] = useState("text");
  const [clicks, setClicks] = useState(0);
  const available = snapshot.entries.filter((item) => item.family === family);
  const sizes = [...new Set(available.map((item) => item.size))].sort(
    (a, b) => ["xs", "sm", "md", "lg", "xl"].indexOf(a) - ["xs", "sm", "md", "lg", "xl"].indexOf(b),
  );
  const items = available.filter(
    (item) =>
      item.size === size && (family === "close" || family === "utility" || item.iconOnly === (shape === "icon")),
  );
  const hierarchies = [...new Set(items.map((item) => item.hierarchy))];
  return (
    <section id="button-foundations" className={styles.foundation} aria-labelledby="button-foundations-title">
      <p className={styles.eyebrow}>{labText("COMPONENTS / 设计组件")}</p>
      <h2 id="button-foundations-title">{labText("Buttons · 按钮")}</h2>
      <p lang={locale === "zh" ? "zh-CN" : "en"}>{labText("Buttons communicate actions that users can take.")}</p>
      <p>
        {labText("Figma 快照 · ")}
        {snapshot.captured} · {snapshot.entries.length}
        {labText(" 个变体索引 / ")}
        {verifiedCount}
        {labText(" 个已读取样式")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Buttons 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("参考矩阵与现有组件对照")}</strong>
        <p>
          {labText(
            "上方为固定状态参考，下方复用生产 Button，可点击、悬浮并用 Tab 检查焦点。代码样本的 Hover / Focused 需真实交互触发，不强制模拟状态。",
          )}
        </p>
        <p>
          {labText(
            "参考使用工具导出的颜色默认值；部分 Figma 截图处于不同主题，尚不能据此确认完整亮暗规范。PingFang SC 未内置，字体回退时字形与字重不代表还原完成。参考保留原始图标及尺寸。",
          )}
        </p>
        <p>
          {labText(
            "已同步：sm/md/lg/xl 高度 36/40/44/48px，普通按钮圆角 8px，加载态显示转圈与文案。Secondary 映射到 secondaryGray。设计参考已更新为 PingFang SC Medium 500；现有代码样本继续使用生产 Button 字体 token，便于对照。阴影及完整主题映射仍需验收。",
          )}
        </p>
        <p>
          {labText("Figma 调用额度已达上限，其余 ")}
          {snapshot.entries.length - verifiedCount}
          {labText(
            " 项仅记录节点和尺寸，待读取样式。共享 Button 已同步确认的通用尺寸、圆角和加载行为；社交登录、商店徽章不在本次范围。",
          )}
        </p>
      </div>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("Button family")}
          value={family}
          options={families.map((item) => ({ ...item, label: labText(item.label) }))}
          onChange={(value) => {
            setFamily(value);
            setSize(value === "utility" ? "sm" : "md");
          }}
        />
      </div>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("Button size")}
          value={size}
          onChange={setSize}
          options={sizes.map((value) => ({ value, label: value }))}
        />
      </div>
      {(family === "standard" || family === "destructive") && (
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Button content")}
            value={shape}
            onChange={setShape}
            options={[
              { value: "text", label: labText("文字与双图标") },
              { value: "icon", label: labText("仅图标") },
            ]}
          />
        </div>
      )}
      <p role="status">
        {labText("当前 ")}
        {items.length}
        {labText(" 个变体，已读取 ")}
        {items.filter((item) => item.verified).length}
        {labText(" 个 · 代码样本已点击 ")}
        {clicks} {labText("次")}
      </p>
      {hierarchies.map((hierarchy) => (
        <section className={styles.section} key={hierarchy} aria-label={hierarchy}>
          <h3>
            {hierarchy} → <code>{codeVariant(items.find((item) => item.hierarchy === hierarchy)!)}</code>
          </h3>
          <div className={styles.grid}>
            {items
              .filter((item) => item.hierarchy === hierarchy)
              .sort((a, b) => states.indexOf(a.state) - states.indexOf(b.state))
              .map((item) => (
                <article
                  key={item.id}
                  className={styles.item}
                  aria-label={`${hierarchy} ${item.state}${item.darkBackground ? labText(" 深色背景") : ""}`}
                >
                  <h4>
                    {labText(stateLabels[item.state])} · {item.state}
                    {item.darkBackground ? labText(" · 深色背景") : ""}
                  </h4>
                  <p>
                    {labText("Figma · ")}
                    {item.width} × {item.height}
                    {labText("px")}
                  </p>
                  <ReferenceButton item={item} />
                  <p>
                    {labText("现有代码 · ")}
                    {codeVariant(item)}
                  </p>
                  <div className={styles.codeStage} data-dark-background={item.darkBackground}>
                    <CodeButton item={item} onClick={() => setClicks((n) => n + 1)} />
                  </div>
                  <details>
                    <summary>{labText("参数与来源")}</summary>
                    <p>
                      {labText("节点 ")}
                      {item.id}
                    </p>
                    {item.reference && (
                      <p>
                        {labText("圆角 ")}
                        {item.reference.radius}
                        {labText(" · 间距 ")}
                        {item.reference.gap}
                        {labText(" · 字号 / 行高 ")}
                        {item.reference.fontSize} / {item.reference.lineHeight}
                        {labText(" · 原始内边距 X / Y ")}
                        {item.reference.paddingX} / {item.reference.paddingY}
                      </p>
                    )}
                    <a
                      href={`${snapshot.source.split("?")[0]}?node-id=${item.id.replace(":", "-")}`}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {labText("查看此变体 ↗")}
                    </a>
                  </details>
                </article>
              ))}
          </div>
        </section>
      ))}
    </section>
  );
}
