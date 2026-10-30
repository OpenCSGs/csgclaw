import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { Button, SegmentedControl } from "@/components/ui";
import snapshot from "../figmaInputs.json";
import { InputSample } from "./InputSample";
import { CompoundReference } from "./CompoundReference";
import styles from "./InputFoundations.module.css";

const types = [
  { value: "input", label: "单行输入" },
  { value: "textarea", label: "多行文本" },
  { value: "compound", label: "组合输入" },
  { value: "verification", label: "验证码" },
];
const stateLabels: Record<string, string> = {
  Placeholder: "占位",
  Filled: "已填写",
  Default: "已填写",
  Focused: "聚焦",
  Disabled: "禁用",
};
const sourceFor = (id: string) => `${snapshot.source.split("?")[0]}?node-id=${id.replace(":", "-")}`;
const verified = snapshot.entries.filter((entry) => entry.verified);

export function InputFoundations() {
  const { text: labText, locale } = useLabTranslation();

  const [size, setSize] = useState<"sm" | "md">("md");
  const [family, setFamily] = useState("input");
  const [mode, setMode] = useState("normal");
  const [email, setEmail] = useState("");
  const [description, setDescription] = useState("");
  const [validated, setValidated] = useState(false);
  const [status, setStatus] = useState("");
  const emailError =
    mode === "error" || (validated && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))
      ? "请输入有效邮箱，例如 name@example.com。"
      : undefined;
  const descriptionError = mode === "error" || (validated && !description.trim()) ? "请输入描述。" : undefined;
  const entries = verified.filter((entry) =>
    family === "input"
      ? entry.group === "Input field" && entry.Type === "Default" && entry.Size === size
      : family === "textarea"
        ? entry.group === "Textarea input field"
        : family === "verification"
          ? entry.group === "Verification code input field"
          : entry.group === "Input field" && entry.Type !== "Default",
  );
  return (
    <section id="input-foundations" className={styles.foundation} aria-labelledby="input-title">
      <p className={styles.eyebrow}>{labText("COMPONENTS / 设计组件")}</p>
      <h2 id="input-title">{labText("Input fields · 输入框")}</h2>
      <p lang={locale === "zh" ? "zh-CN" : "en"}>
        {labText(
          "Input fields allow users to enter text into a UI. They typically appear in forms and dialogs. Input fields on mobiles should be at least 16px text size to avoid auto zoom on mobile browsers.",
        )}
      </p>
      <p>
        {labText("Figma 参数快照 · ")}
        {snapshot.captured} · {snapshot.entries.length}
        {labText(" 个节点 / ")}
        {verified.length}
        {labText(" 个已读取样式")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Inputs 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("尺寸、字体与状态基线")}</strong>
        <p>
          {labText(
            "单行 sm / md：40 / 44px；圆角 8px；多行文本区域 128px。正文 16/24px、标签 14/20px，PingFang SC Regular 400；提示为 Light 300。聚焦描边 2px，普通描边 1px。",
          )}
        </p>
        <p>
          {labText(
            "本次完整读取普通单行 14 个和普通多行 7 个状态，另收录 6 种组合输入的 md 占位态及 1 个验证码参考。其余",
          )}{" "}
          {snapshot.entries.length - verified.length}{" "}
          {labText("个节点仅建立索引，样式待读取。组合输入与验证码为静态参考，后续补充交互与状态。")}
        </p>
        <p>
          {labText(
            "参考区保持设计导出颜色和原始图标，外层容器跟随主题。PingFang SC 未内置，缺少本地字体时会回退。试用复用现有 Field、TextInput、TextArea，样式限定在 UI Lab；产品输入框尚未同步。",
          )}
        </p>
      </div>
      <section className={styles.demo} aria-label={labText("输入框交互试用")}>
        <h3>{labText("交互试用")}</h3>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Input size")}
            value={size}
            onChange={(value) => setSize(value === "sm" ? "sm" : "md")}
            options={[
              { value: "sm", label: "sm · 40px" },
              { value: "md", label: "md · 44px" },
            ]}
          />
          <SegmentedControl
            label={labText("Input state")}
            value={mode}
            onChange={setMode}
            options={[
              { value: "normal", label: labText("正常") },
              { value: "error", label: labText("错误") },
              { value: "disabled", label: labText("禁用") },
            ]}
          />
        </div>
        <p>{labText("点击或 Tab 聚焦，查看真实焦点；校验只在本地运行。输入内容在切换尺寸和状态时保留。")}</p>
        <div className={styles.liveGrid}>
          <div className={styles.stage}>
            <InputSample
              size={size}
              state={mode === "disabled" ? "Disabled" : "Default"}
              error={emailError ? labText(emailError) : undefined}
              value={email}
              onChange={(event) => {
                setEmail(event.target.value);
                setStatus("");
              }}
            />
          </div>
          <div className={styles.stage}>
            <InputSample
              multiline
              error={descriptionError ? labText(descriptionError) : undefined}
              state={mode === "disabled" ? "Disabled" : "Default"}
              value={description}
              onChange={(event) => {
                setDescription(event.target.value);
                setStatus("");
              }}
            />
          </div>
        </div>
        <div className={styles.actions}>
          <Button
            size="sm"
            disabled={mode === "disabled"}
            onClick={() => {
              setValidated(true);
              setStatus(
                mode === "error" || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) || !description.trim()
                  ? "请修正字段错误，输入内容已保留。"
                  : "本地校验通过，未提交或保存数据。",
              );
            }}
          >
            {labText("校验示例")}
          </Button>
          <Button
            size="sm"
            variant="secondaryGray"
            onClick={() => {
              setEmail("");
              setDescription("");
              setValidated(false);
              setMode("normal");
              setStatus("示例已重置。");
            }}
          >
            {labText("重置示例")}
          </Button>
          <span role="status">{labText(status)}</span>
        </div>
      </section>
      <h3>{labText("设计状态参考")}</h3>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("Input reference type")}
          value={family}
          onChange={setFamily}
          options={types.map((item) => ({ ...item, label: labText(item.label) }))}
        />
      </div>
      <p>{labText("静态参考保留节点外框；窄屏可在卡片内横向查看。聚焦样式为固定快照，请在上方试用真实键盘操作。")}</p>
      <div className={styles.grid}>
        {entries.map((entry) => {
          const multiline = entry.group === "Textarea input field";
          const ordinary = entry.Type === "Default";
          const destructive = entry.Destructive === "True";
          const state = entry.State ?? "Placeholder";
          const value =
            state === "Placeholder" || (multiline && state === "Disabled")
              ? ""
              : multiline
                ? "A little about the company and the team that you’ll be working with."
                : "olivia@untitledui.com";
          return (
            <article className={styles.item} key={entry.id}>
              <h4>
                {ordinary
                  ? `${destructive ? labText("错误") : labText("正常")} · ${labText(stateLabels[state])}`
                  : (entry.Type ?? labText("验证码 · md / 4 位"))}
              </h4>
              <p>
                {entry.width} × {entry.height}
                {labText("px · ")}
                {entry.id}
              </p>
              <div className={styles.referenceStage} inert>
                {ordinary ? (
                  <div style={{ width: entry.width }}>
                    <InputSample
                      reference
                      size={entry.Size === "sm" ? "sm" : "md"}
                      multiline={multiline}
                      state={state}
                      error={destructive ? "This is an error message." : undefined}
                      value={value}
                    />
                  </div>
                ) : (
                  <CompoundReference entry={entry} />
                )}
              </div>
              <a href={sourceFor(entry.id)} target="_blank" rel="noreferrer">
                {labText("查看设计节点 ↗")}
              </a>
            </article>
          );
        })}
      </div>
      <details className={styles.index}>
        <summary>
          {labText("全部节点索引与待补项（")}
          {snapshot.entries.length}）
        </summary>
        <p>{labText("待读取项只记录 Figma 名称、尺寸和链接，不作为已实现样式。")}</p>
        <ul>
          {snapshot.entries.map((entry) => (
            <li key={entry.id}>
              <a href={sourceFor(entry.id)} target="_blank" rel="noreferrer">
                {entry.group} / {entry.Type ?? ""} / {entry.Size ?? ""} / {entry.Digits ? `${entry.Digits} 位` : ""} /{" "}
                {entry.Destructive === "True" ? labText("错误 / ") : ""}
                {entry.State ?? ""}
              </a>{" "}
              · {entry.width} × {entry.height}
              {labText("px · ")}
              {entry.verified ? labText("已读取") : labText("样式待读取")}
            </li>
          ))}
        </ul>
      </details>
    </section>
  );
}
