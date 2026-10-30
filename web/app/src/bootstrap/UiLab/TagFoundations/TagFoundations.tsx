import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { Button, Field, TextInput, SegmentedControl } from "@/components/ui";
import snapshot from "../figmaTags.json";
import { TagSample } from "./TagSample";
import styles from "./TagFoundations.module.css";

const icons = [
  { value: "False", label: "无图标" },
  { value: "Dot", label: "状态点" },
  { value: "Country", label: "国旗" },
  { value: "Avatar", label: "头像" },
];
const actions = [
  { value: "Text only", label: "纯文字" },
  { value: "X close", label: "可移除" },
  { value: "Count", label: "计数" },
];
const readCount = snapshot.entries.filter((entry) => entry.reference).length;
export function TagFoundations() {
  const { text: labText, locale } = useLabTranslation();

  const [size, setSize] = useState("md");
  const [icon, setIcon] = useState("False");
  const [action, setAction] = useState("X close");
  const [checkbox, setCheckbox] = useState("yes");
  const [customLabel, setLabel] = useState<string | null>(null);
  const label = customLabel ?? labText("Label");
  const [removed, setRemoved] = useState(false);
  const entry = snapshot.entries.find(
    (item) =>
      item.size === size && item.icon === icon && item.action === action && item.checkbox === (checkbox === "yes"),
  )!;
  return (
    <section id="tag-foundations" className={styles.foundation} aria-labelledby="tag-title">
      <p className={styles.eyebrow}>{labText("COMPONENTS / 设计组件")}</p>
      <h2 id="tag-title">{labText("Tags · 标签")}</h2>
      <p lang={locale === "zh" ? "zh-CN" : "en"}>
        {labText(
          "Tags (also known as “chips”) are compact elements that allow users to enter information, make selections, filter content, or trigger actions. They’re similar to badges, but are more commonly used in inputs to allow users to select multiple items at once.",
        )}
      </p>
      <p>
        {labText("Figma 参数快照 · ")}
        {snapshot.captured}
        {labText(" · 72 个组合 / ")}
        {readCount}
        {labText(" 个已读取样式")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Tags 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("标签参考与交互试用")}</strong>
        <p>
          {labText(
            "sm / md 高度 24px，lg 高度 28px，圆角 6px；文字为 PingFang SC Regular 400。原始颜色和图片保持设计导出值，主题切换只改变展示容器。PingFang SC 未内置时使用系统字体回退。",
          )}
        </p>
        <p>
          {labText("勾选与移除复用现有 Checkbox、Button；勾选、焦点和悬浮效果尚未逐项对齐 Figma 子组件。剩余 ")}
          {72 - readCount}{" "}
          {labText("个组合因调用额度限制仅保留节点与尺寸，不推测样式。当前仅用于 UI Lab，尚未替换产品中的业务标签。")}
        </p>
      </div>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("Tag size")}
          value={size}
          onChange={setSize}
          options={["sm", "md", "lg"].map((value) => ({ value, label: value }))}
        />
      </div>
      <section className={styles.demo} aria-label={labText("标签交互试用")}>
        <h3>{labText("交互试用")}</h3>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Tag icon")}
            value={icon}
            onChange={setIcon}
            options={icons.map((item) => ({ ...item, label: labText(item.label) }))}
          />
        </div>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Tag action")}
            value={action}
            onChange={setAction}
            options={actions.map((item) => ({ ...item, label: labText(item.label) }))}
          />
        </div>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Tag checkbox")}
            value={checkbox}
            onChange={setCheckbox}
            options={[
              { value: "yes", label: labText("带复选框") },
              { value: "no", label: labText("不带复选框") },
            ]}
          />
        </div>
        <Field label={labText("标签文案")} hint={labText("最多 120 字；长文案省略显示，悬浮可查看完整内容。")}>
          <TextInput value={label} maxLength={120} onChange={(event) => setLabel(event.target.value)} />
        </Field>
        <div className={styles.stage}>
          {removed ? (
            <p role="status">{labText("标签已移除")}</p>
          ) : (
            <TagSample
              key={entry.id}
              entry={entry}
              label={label || labText("Label")}
              interactive
              onRemove={() => setRemoved(true)}
            />
          )}
        </div>
        <Button size="sm" onClick={() => setRemoved(false)}>
          {labText("恢复标签")}
        </Button>
      </section>
      <h3>{labText("当前尺寸的设计参考")}</h3>
      <p>{labText("参考保留节点外框宽高和默认未勾选状态。试用区按文案自适应宽度。")}</p>
      <div className={styles.grid}>
        {snapshot.entries
          .filter((item) => item.size === size)
          .map((item) => (
            <article
              key={item.id}
              className={styles.item}
              aria-label={`${item.icon} ${item.action} ${item.checkbox ? labText("带复选框") : labText("不带复选框")}`}
            >
              <h4>
                {labText(icons.find((i) => i.value === item.icon)?.label ?? item.icon)} ·{" "}
                {labText(actions.find((a) => a.value === item.action)?.label ?? item.action)}
              </h4>
              <p>
                {item.checkbox ? labText("带复选框") : labText("不带复选框")} · {item.width} × {item.height}
                {labText("px")}
              </p>
              <div className={styles.stage} inert>
                <TagSample entry={item} />
              </div>
              <a
                href={`${snapshot.source.split("?")[0]}?node-id=${item.id.replace(":", "-")}`}
                target="_blank"
                rel="noreferrer"
              >
                {labText("查看设计节点 ↗")}
              </a>
            </article>
          ))}
      </div>
    </section>
  );
}
