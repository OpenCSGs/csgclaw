import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { Button, Field, SegmentedControl, TextInput } from "@/components/ui";
import snapshot from "../figmaToggles.json";
import { ToggleSample } from "./ToggleSample";
import styles from "./ToggleFoundations.module.css";

const stateLabels: Record<string, string> = { Default: "默认", Hover: "悬浮", Focus: "聚焦", Disabled: "禁用" };
export function ToggleFoundations() {
  const { text: labText, locale } = useLabTranslation();

  const [size, setSize] = useState<"sm" | "md">("md");
  const [type, setType] = useState<"Default" | "Slim">("Default");
  const [text, setText] = useState("yes");
  const [supporting, setSupporting] = useState("yes");
  const [mode, setMode] = useState("enabled");
  const [checked, setChecked] = useState(false);
  const [customLabel, setLabel] = useState<string | null>(null);
  const label = customLabel ?? labText("Remember me");
  const [customDescription, setDescription] = useState<string | null>(null);
  const description = customDescription ?? labText("Save my login details for next time.");
  const entries = snapshot.entries.filter(
    (entry) => entry.Type === type && entry.Size === size && entry.Text === (text === "yes" ? "True" : "False"),
  );
  return (
    <section id="toggle-foundations" className={styles.foundation} aria-labelledby="toggle-title">
      <p className={styles.eyebrow}>{labText("COMPONENTS / 设计组件")}</p>
      <h2 id="toggle-title">{labText("Toggles · 开关")}</h2>
      <p lang={locale === "zh" ? "zh-CN" : "en"}>
        {labText(
          "Toggles (also known as “switches”) is a UI control that has two mutually-exclusive states, such as ON and OFF. The design and functionality of this component is based on a physical switch that allows users to turn things ON or OFF. Toggles are a great visual cue for settings, especially on mobile, because they take less screen estate (in comparison with two radio buttons).",
        )}
      </p>
      <p>
        {labText("Figma 参数快照 · ")}
        {snapshot.captured}
        {labText(" · 64 个组合 / 64 个已读取样式")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Toggles 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("Default 与 Slim")}</strong>
        <p>
          {labText(
            "Default sm / md：36 × 20 / 44 × 24px；Slim sm / md：32 × 16 / 40 × 20px。开启为品牌色，禁用使用独立灰色；聚焦环为 2px 白色间隔与 2px 品牌色外环。",
          )}
        </p>
        <details>
          <summary>{labText("字体差异与使用范围")}</summary>
          <p>
            {labText(
              "标签统一使用 PingFang SC Regular 400，说明文字使用 Light 300。已依据更新后的共享文字样式同步字体和字重；缺少本地 PingFang SC 时使用系统字体回退。",
            )}
          </p>
          <p>
            {labText(
              "试用复用共享 Switch 的 Radix 交互，Figma 样式限定在 UI Lab。白色参考区保持设计导出颜色，主题切换只影响外层容器；尚未同步产品 Switch 或建立完整暗色映射。此示例不保存登录信息。",
            )}
          </p>
        </details>
      </div>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("Toggle type")}
          value={type}
          onChange={(value) => setType(value === "Slim" ? "Slim" : "Default")}
          options={[
            { value: "Default", label: "Default" },
            { value: "Slim", label: "Slim" },
          ]}
        />
        <SegmentedControl
          label={labText("Toggle size")}
          value={size}
          onChange={(value) => setSize(value === "sm" ? "sm" : "md")}
          options={[
            { value: "sm", label: "sm" },
            { value: "md", label: "md" },
          ]}
        />
        <SegmentedControl
          label={labText("Toggle text")}
          value={text}
          onChange={setText}
          options={[
            { value: "yes", label: labText("带文字") },
            { value: "no", label: labText("仅开关") },
          ]}
        />
      </div>
      <section className={styles.demo} aria-label={labText("开关交互试用")}>
        <h3>{labText("交互试用")}</h3>
        <p>{labText("点击标签或开关切换，也可 Tab 聚焦后按 Space。切换外观和尺寸时保留当前开关值。")}</p>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Toggle disabled")}
            value={mode}
            onChange={setMode}
            options={[
              { value: "enabled", label: labText("可用") },
              { value: "disabled", label: labText("禁用") },
            ]}
          />
          {text === "yes" && (
            <SegmentedControl
              label={labText("Toggle supporting text")}
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
            <Field label={labText("开关标题")}>
              <TextInput value={label} maxLength={120} onChange={(event) => setLabel(event.target.value)} />
            </Field>
            <Field label={labText("开关说明")}>
              <TextInput value={description} maxLength={240} onChange={(event) => setDescription(event.target.value)} />
            </Field>
          </div>
        )}
        <div className={styles.liveStage}>
          <ToggleSample
            size={size}
            type={type}
            text={text === "yes"}
            supporting={supporting === "yes"}
            checked={checked}
            disabled={mode === "disabled"}
            label={label}
            description={description}
            onCheckedChange={setChecked}
          />
        </div>
        <div className={styles.actions}>
          <span role="status">
            {labText("当前：")}
            {checked ? labText("开启") : labText("关闭")}
            {mode === "disabled" ? labText("（已禁用）") : ""}
          </span>
          <Button
            size="sm"
            onClick={() => {
              setChecked(false);
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
      <p>{labText("当前筛选包含 8 个状态，保留设计节点外框。参考为静态快照，真实交互请使用上方试用区。")}</p>
      <div className={styles.grid}>
        {entries
          .slice()
          .sort(
            (a, b) =>
              ["Default", "Hover", "Focus", "Disabled"].indexOf(a.State) -
                ["Default", "Hover", "Focus", "Disabled"].indexOf(b.State) || a.Pressed.localeCompare(b.Pressed),
          )
          .map((entry) => (
            <article className={styles.item} key={entry.id}>
              <h4>
                {labText(stateLabels[entry.State])} · {entry.Pressed === "True" ? labText("开启") : labText("关闭")}
              </h4>
              <p>
                {entry.width} × {entry.height}
                {labText("px · ")}
                {entry.id}
              </p>
              <div className={styles.referenceStage} inert>
                <div style={{ width: entry.width, height: entry.height }}>
                  <ToggleSample
                    type={type}
                    size={size}
                    text={text === "yes"}
                    checked={entry.Pressed === "True"}
                    referenceState={entry.State}
                  />
                </div>
              </div>
              {entry.Text === "True" && (
                <p>
                  {entry.labelFamily} {entry.labelWeight}
                  {labText(" · 说明 PingFang SC 300")}
                </p>
              )}
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
