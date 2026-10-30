import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { Button, Field, SegmentedControl, TextInput } from "@/components/ui";
import snapshot from "../figmaCheckboxes.json";
import { CheckboxSample } from "./CheckboxSample";
import styles from "./CheckboxFoundations.module.css";

const choices = [
  { value: "email", label: "邮件通知" },
  { value: "desktop", label: "桌面通知" },
  { value: "sound", label: "声音提醒" },
];
const stateLabels: Record<string, string> = { Default: "默认", Hover: "悬浮", Focused: "聚焦", Disabled: "禁用" };
export function CheckboxFoundations() {
  const { text: labText, locale } = useLabTranslation();

  const [size, setSize] = useState<"sm" | "md">("md");
  const [text, setText] = useState("yes");
  const [supporting, setSupporting] = useState("yes");
  const [mode, setMode] = useState("enabled");
  const [selected, setSelected] = useState<string[]>(["email"]);
  const [customLabel, setLabel] = useState<string | null>(null);
  const label = customLabel ?? labText("全选通知");
  const [customDescription, setDescription] = useState<string | null>(null);
  const description = customDescription ?? labText("选择部分通知后，全选复选框显示半选状态。");
  const checked = selected.length === choices.length ? true : selected.length > 0 ? "indeterminate" : false;
  const entries = snapshot.entries.filter(
    (entry) => entry.Size === size && entry.Text === (text === "yes" ? "True" : "False"),
  );
  return (
    <section id="checkbox-foundations" className={styles.foundation} aria-labelledby="checkbox-title">
      <p className={styles.eyebrow}>{labText("COMPONENTS / 设计组件")}</p>
      <h2 id="checkbox-title">{labText("Checkboxes · 复选框")}</h2>
      <p lang={locale === "zh" ? "zh-CN" : "en"}>
        {labText(
          "Checkboxes allow users to select one or more items from a set, while radio buttons allow users to select just one option from a set. Both can also be used to turn an option on or off.",
        )}
      </p>
      <p>
        {labText("Figma 参数快照 · ")}
        {snapshot.captured}
        {labText(" · 40 个组合 / 40 个已读取样式")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Checkboxes 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("选中、未选中与半选")}</strong>
        <p>
          {labText(
            "sm / md：16 / 20px，圆角 4 / 6px；标签为 PingFang SC 400，说明为 Light 300。聚焦使用 2px 白色间隔与 2px 品牌色外环，禁用使用独立背景和图标。",
          )}
        </p>
        <details>
          <summary>{labText("参考范围与组件复用")}</summary>
          <p>
            {labText(
              "设计提供带文字的选中/未选中状态，以及无文字的选中/未选中/半选状态。半选带文字仅为组合交互示例，不计入原稿 40 个变体。图标使用原始 SVG，保留各尺寸与禁用态差异。",
            )}
          </p>
          <p>
            {labText(
              "交互复用共享 Checkbox 的 Radix 语义，Figma 样式仅用于 UI Lab。白色参考区固定设计导出颜色，外层支持主题切换；产品复选框样式尚未同步。缺少 PingFang SC 时使用系统字体回退。通知选项不保存或发送。",
            )}
          </p>
        </details>
      </div>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("Checkbox size")}
          value={size}
          onChange={(value) => setSize(value === "sm" ? "sm" : "md")}
          options={[
            { value: "sm", label: "sm · 16px" },
            { value: "md", label: "md · 20px" },
          ]}
        />
        <SegmentedControl
          label={labText("Checkbox text")}
          value={text}
          onChange={setText}
          options={[
            { value: "yes", label: labText("带文字") },
            { value: "no", label: labText("仅复选框") },
          ]}
        />
      </div>
      <section className={styles.demo} aria-label={labText("复选框交互试用")}>
        <h3>{labText("交互试用")}</h3>
        <p>{labText("选择部分通知查看半选；点击全选或按 Space 切换全部选项。切换尺寸、布局和禁用状态时保留选择。")}</p>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Checkbox disabled")}
            value={mode}
            onChange={setMode}
            options={[
              { value: "enabled", label: labText("可用") },
              { value: "disabled", label: labText("禁用") },
            ]}
          />
          {text === "yes" && (
            <SegmentedControl
              label={labText("Checkbox supporting text")}
              value={supporting}
              onChange={setSupporting}
              options={[
                { value: "yes", label: labText("显示说明") },
                { value: "no", label: labText("隐藏说明") },
              ]}
            />
          )}
        </div>
        {text === "yes" && (
          <div className={styles.fields}>
            <Field label={labText("复选框标题")}>
              <TextInput value={label} maxLength={120} onChange={(event) => setLabel(event.target.value)} />
            </Field>
            <Field label={labText("复选框说明")}>
              <TextInput value={description} maxLength={240} onChange={(event) => setDescription(event.target.value)} />
            </Field>
          </div>
        )}
        <div className={styles.liveStage}>
          <CheckboxSample
            size={size}
            checked={checked}
            text={text === "yes"}
            supporting={supporting === "yes"}
            label={label || labText("全选通知")}
            description={description}
            disabled={mode === "disabled"}
            onCheckedChange={(value) => setSelected(value === true ? choices.map((item) => item.value) : [])}
          />
          <fieldset className={styles.choices}>
            <legend>{labText("通知选项")}</legend>
            {choices.map((choice) => (
              <CheckboxSample
                key={choice.value}
                size={size}
                checked={selected.includes(choice.value)}
                label={labText(choice.label)}
                supporting={false}
                disabled={mode === "disabled"}
                onCheckedChange={(value) =>
                  setSelected((current) =>
                    value === true
                      ? [...current.filter((item) => item !== choice.value), choice.value]
                      : current.filter((item) => item !== choice.value),
                  )
                }
              />
            ))}
          </fieldset>
        </div>
        <div className={styles.actions}>
          <span role="status">
            {labText("已选择 ")}
            {selected.length}
            {labText(" / 3 项 · ")}
            {checked === "indeterminate" ? labText("半选") : checked ? labText("全选") : labText("未选")}
            {mode === "disabled" ? labText("（已禁用）") : ""}
          </span>
          <Button
            size="sm"
            onClick={() => {
              setSelected(["email"]);
              setMode("enabled");
              setLabel(null);
              setDescription(null);
            }}
          >
            {labText("重置示例")}
          </Button>
        </div>
      </section>
      <h3>{labText("设计状态参考")}</h3>
      <p>
        {labText("当前 ")}
        {entries.length}
        {labText(" 个状态，保留设计节点外框；静态参考不可操作，请在上方试用真实交互。")}
      </p>
      <div className={styles.grid}>
        {entries
          .slice()
          .sort(
            (a, b) =>
              ["Default", "Hover", "Focused", "Disabled"].indexOf(a.State) -
                ["Default", "Hover", "Focused", "Disabled"].indexOf(b.State) ||
              `${a.Checked}${a.Indeterminate}`.localeCompare(`${b.Checked}${b.Indeterminate}`),
          )
          .map((entry) => (
            <article className={styles.item} key={entry.id}>
              <h4>
                {labText(stateLabels[entry.State])} ·{" "}
                {entry.Indeterminate === "True"
                  ? labText("半选")
                  : entry.Checked === "True"
                    ? labText("选中")
                    : labText("未选")}
              </h4>
              <p>
                {entry.width} × {entry.height}
                {labText("px · ")}
                {entry.id}
              </p>
              <div className={styles.referenceStage} inert>
                <div style={{ width: entry.width, height: entry.height }}>
                  <CheckboxSample
                    size={size}
                    text={text === "yes"}
                    checked={entry.Indeterminate === "True" ? "indeterminate" : entry.Checked === "True"}
                    referenceState={entry.State}
                  />
                </div>
              </div>
              <a
                href={`${snapshot.source.split("?")[0]}?node-id=${entry.id.replace(":", "-")}`}
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
