import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { Button, Field, SegmentedControl, TextInput } from "@/components/ui";
import snapshot from "../figmaTooltips.json";
import { TooltipBubble } from "./TooltipBubble";
import { tooltipPositions, defaultTooltipTitle, defaultTooltipDescription } from "./tooltipData";
import { TooltipPreview } from "./TooltipPreview";
import styles from "./TooltipFoundations.module.css";

export function TooltipFoundations() {
  const { text: labText, locale } = useLabTranslation();

  const [supporting, setSupporting] = useState("no");
  const [arrow, setArrow] = useState("Bottom center");
  const [title, setTitle] = useState(defaultTooltipTitle);
  const [description, setDescription] = useState(defaultTooltipDescription);
  const entries = snapshot.entries.filter(
    (entry) => entry["Supporting text"] === (supporting === "yes" ? "True" : "False"),
  );
  const previewProps = { arrow, supporting: supporting === "yes", title, description };
  return (
    <section id="tooltip-foundations" className={styles.foundation} aria-labelledby="tooltip-title">
      <p className={styles.eyebrow}>{labText("COMPONENTS / 设计组件")}</p>
      <h2 id="tooltip-title">{labText("Tooltips · 气泡提示")}</h2>
      <p lang={locale === "zh" ? "zh-CN" : "en"}>
        {labText(
          "Tooltips are used to describe or identify an element. In most scenarios, tooltips help the user understand the meaning or function of an icon, or display the alt text of an image.",
        )}
      </p>
      <p>
        {labText("Figma 参数快照 · ")}
        {snapshot.captured}
        {labText(" · 14 个气泡变体 / 14 个已读取样式")}
      </p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma Tooltips 设计来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("方向、内容与阴影")}</strong>
        <p>
          {labText(
            "圆角 8px，文字 PingFang SC 12/18px；标题 Medium 500，说明 Regular 400。纯文字内边距为 8 × 12px，带说明为 12px，内容间距 2px；箭头占位 6px，使用原始 SVG 和三层阴影。",
          )}
        </p>
        <details>
          <summary>{labText("参考范围与交互约定")}</summary>
          <p>
            {labText(
              "箭头名称指箭头所在边，例如“下居中”表示气泡位于触发点上方。静态参考保留设计尺寸；真实提示会随文案、触发点和可用空间调整位置，避让翻转后箭头跟随实际方向。",
            )}
          </p>
          <p>
            {labText(
              "复用现有 Tooltip 的延迟、Portal、焦点和 Escape 行为。帮助图标读取了开/关两个状态，采用真实系统指针，不绘制设计稿的可选光标示意。气泡固定设计导出配色，主题仅影响外层；产品 Tooltip 样式尚未同步，字体缺失时回退。",
            )}
          </p>
        </details>
      </div>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("Tooltip content")}
          value={supporting}
          onChange={setSupporting}
          options={[
            { value: "no", label: labText("纯文字") },
            { value: "yes", label: labText("带说明") },
          ]}
        />
      </div>
      <section className={styles.demo} aria-label={labText("气泡提示交互试用")}>
        <h3>{labText("交互试用")}</h3>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("Tooltip arrow")}
            value={arrow}
            onChange={setArrow}
            options={tooltipPositions.map((item) => ({ value: item.value, label: labText(item.label) }))}
          />
        </div>
        <div className={styles.fields}>
          <Field label={labText("提示标题")}>
            <TextInput value={title} maxLength={120} onChange={(event) => setTitle(event.target.value)} />
          </Field>
          {supporting === "yes" && (
            <Field label={labText("提示说明")}>
              <TextInput value={description} maxLength={480} onChange={(event) => setDescription(event.target.value)} />
            </Field>
          )}
        </div>
        <p>
          {labText(
            "悬浮或 Tab 聚焦任一触发点查看提示；Escape 关闭。两侧帮助图标分别请求向左/向右显示，用于检查视口边缘的自动避让。",
          )}
        </p>
        <div className={styles.previewStage}>
          <TooltipPreview {...previewProps} edge="left" />
          <TooltipPreview {...previewProps} />
          <TooltipPreview {...previewProps} edge="right" />
        </div>
        <Button
          size="sm"
          onClick={() => {
            setTitle(defaultTooltipTitle);
            setDescription(defaultTooltipDescription);
          }}
        >
          {labText("重置文案")}
        </Button>
      </section>
      <h3>{labText("设计静态参考")}</h3>
      <p>{labText("当前 7 种箭头位置。参考为静态样式，不会弹出；长气泡可在卡片内横向查看。")}</p>
      <div className={styles.grid}>
        {entries.map((entry) => (
          <article className={styles.item} key={entry.id}>
            <h4>
              {labText(tooltipPositions.find((item) => item.value === entry.Arrow)?.label ?? entry.Arrow)} ·{" "}
              {entry.Arrow}
            </h4>
            <p>
              {entry.width} × {entry.height}
              {labText("px · ")}
              {entry.id}
            </p>
            <div className={styles.referenceStage} inert>
              <TooltipBubble
                arrow={entry.Arrow}
                supporting={supporting === "yes"}
                width={entry.width}
                height={entry.height}
              />
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
