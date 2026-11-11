import { useState } from "react";
import { Button, SegmentedControl, Slider } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import snapshot from "../figmaProgress.json";
import { ProgressBar } from "./ProgressBar";
import { ProgressCircleReference } from "./ProgressCircleReference";
import { labelOptions } from "./progressOptions";
import styles from "./ProgressFoundations.module.css";

const sourceFor = (id: string) => `${snapshot.source.split("?")[0]}?node-id=${id.replace(":", "-")}`;

export function ProgressFoundations() {
  const { text: labText } = useLabTranslation();
  const [value, setValue] = useState(40);
  const [label, setLabel] = useState("Right");
  const [family, setFamily] = useState("bars");
  const [shape, setShape] = useState("Circle");
  const barEntries = snapshot.bars.filter((entry) => entry.label === label);
  const circleEntries = snapshot.circles.filter((entry) => entry.shape === shape);
  return (
    <section id="progress-foundations" className={styles.foundation} aria-labelledby="progress-title">
      <p className={styles.eyebrow}>{labText("COMPONENTS / 设计组件")}</p>
      <h2 id="progress-title">{labText("Progress indicators · 进度指示器")}</h2>
      <p>
        {labText(
          "Progress bars indicate the percentage completed of a task. They can be useful to prompt the user to complete an action or process.",
        )}
      </p>
      <p>{labText("已读取 55 个进度条变体和 10 个圆环／半圆变体，覆盖 0%–100% 与 5 种标签布局。")}</p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {labText("查看 Figma 进度指示器来源 ↗")}
      </a>
      <div className={styles.notice}>
        <strong>{labText("进度、标签与参考范围")}</strong>
        <p>
          {labText(
            "进度条高 8px，轨道 #e9eaeb，填充 #3250bd；常规标签为 PingFang SC 14/20px、400，浮动标签为 12/18px、500。圆环包含 xxs / xs / sm / md / lg，保留原始 SVG 与设计尺寸。",
          )}
        </p>
        <p>
          {labText(
            "部分原稿进度条的标称百分比与填充长度不一致，静态参考保留原值；交互演示按实际百分比显示，0% 为空、100% 为满。圆环参考固定显示原稿的 40%，暂不提供动态圆环。参考配色固定，主题仅影响外层容器；尚未迁移业务页面。",
          )}
        </p>
      </div>
      <section className={styles.demo} aria-label={labText("进度交互试用")}>
        <h3>{labText("交互试用")}</h3>
        <p>{labText("拖动滑块或使用方向键、Home、End 调整进度；也可使用按钮检查开始和完成状态。")}</p>
        <div className={styles.controls}>
          <SegmentedControl
            label={labText("进度标签位置")}
            value={label}
            onChange={setLabel}
            options={labelOptions.map((item) => ({ ...item, label: labText(item.label) }))}
          />
        </div>
        <Slider
          value={value}
          min={0}
          max={100}
          step={1}
          label={labText("任务进度")}
          valueText={`${value}%`}
          onValueChange={setValue}
        />
        <div className={styles.liveStage}>
          <ProgressBar value={value} label={label} name={labText("任务完成进度")} />
        </div>
        <div className={styles.actions}>
          <Button size="sm" onClick={() => setValue(0)}>
            {labText("从零开始")}
          </Button>
          <Button size="sm" onClick={() => setValue((current) => Math.min(100, current + 10))} disabled={value === 100}>
            {labText("增加 10%")}
          </Button>
          <Button size="sm" onClick={() => setValue(100)}>
            {labText("完成任务")}
          </Button>
          <Button size="sm" onClick={() => setValue(40)}>
            {labText("重置示例")}
          </Button>
          <span role="status">
            {labText(value === 100 ? "演示已完成" : value === 0 ? "演示尚未开始" : "演示进行中")} · {value}%
          </span>
        </div>
      </section>
      <h3>{labText("设计静态参考")}</h3>
      <div className={styles.controls}>
        <SegmentedControl
          label={labText("进度参考类型")}
          value={family}
          onChange={setFamily}
          options={[
            { value: "bars", label: labText("进度条") },
            { value: "circles", label: labText("圆环与半圆") },
          ]}
        />
        {family === "circles" && (
          <SegmentedControl
            label={labText("圆环形状")}
            value={shape}
            onChange={setShape}
            options={[
              { value: "Circle", label: labText("圆环") },
              { value: "Half circle", label: labText("半圆") },
            ]}
          />
        )}
      </div>
      <div className={styles.grid}>
        {family === "bars"
          ? barEntries.map((entry) => (
              <article key={entry.id} className={styles.item}>
                <h4>
                  {labText(labelOptions.find((item) => item.value === entry.label)!.label)} · {entry.value}%
                </h4>
                <p>
                  {entry.width} × {entry.height}px · {entry.id}
                </p>
                <div className={styles.referenceStage} inert>
                  <div className={styles.barFrame} style={{ width: entry.width, height: entry.height }}>
                    <ProgressBar value={entry.value} label={entry.label} reference={entry} />
                  </div>
                </div>
                <a href={sourceFor(entry.id)} target="_blank" rel="noreferrer">
                  {labText("查看设计节点 ↗")}
                </a>
              </article>
            ))
          : circleEntries.map((entry) => (
              <article key={entry.id} className={styles.item}>
                <h4>
                  {entry.size} · {labText(entry.shape === "Circle" ? "圆环" : "半圆")}
                </h4>
                <p>
                  {entry.width} × {entry.height}px · {entry.id}
                </p>
                <div className={styles.circleStage} inert>
                  <ProgressCircleReference entry={entry} />
                </div>
                <a href={sourceFor(entry.id)} target="_blank" rel="noreferrer">
                  {labText("查看设计节点 ↗")}
                </a>
              </article>
            ))}
      </div>
    </section>
  );
}
