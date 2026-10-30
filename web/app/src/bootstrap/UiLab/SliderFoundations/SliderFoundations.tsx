import { useState } from "react";
import { Slider as RadixSlider } from "radix-ui";
import { Button, SegmentedControl } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import snapshot from "../figmaSliders.json";
import styles from "./SliderFoundations.module.css";

function RangeSample({
  values,
  label,
  onChange,
  disabled = false,
  name,
}: {
  values: number[];
  label: string;
  onChange?: (values: number[]) => void;
  disabled?: boolean;
  name: string;
}) {
  return (
    <RadixSlider.Root
      className={styles.slider}
      value={values}
      min={0}
      max={100}
      step={1}
      minStepsBetweenThumbs={1}
      disabled={disabled}
      onValueChange={onChange}
      data-label={label}
    >
      <RadixSlider.Track className={styles.track}>
        <RadixSlider.Range className={styles.range} />
      </RadixSlider.Track>
      {values.map((value, index) => (
        <RadixSlider.Thumb
          key={index}
          className={styles.thumb}
          aria-label={`${name} ${index + 1}`}
          aria-valuetext={`${value}%`}
        >
          {label !== "False" && <span className={styles.value}>{value}%</span>}
        </RadixSlider.Thumb>
      ))}
    </RadixSlider.Root>
  );
}

export function SliderFoundations() {
  const { text: t } = useLabTranslation();
  const [values, setValues] = useState([25, 75]);
  const [label, setLabel] = useState("Bottom");
  const [disabled, setDisabled] = useState("no");
  const options = [
    { value: "False", label: t("无标签") },
    { value: "Bottom", label: t("底部标签") },
    { value: "Top floating", label: t("顶部浮动标签") },
  ];
  return (
    <section id="slider-foundations" className={styles.foundation} aria-labelledby="slider-title">
      <h2 id="slider-title">{t("Sliders · 滑块")}</h2>
      <p>{t("滑块允许用户在连续范围内选择数值或区间。")}</p>
      <a href={snapshot.source} target="_blank" rel="noreferrer">
        {t("查看 Figma Sliders 来源 ↗")}
      </a>
      <p>
        {t(
          "已读取 30 个滑块组合：无标签、底部标签及顶部浮动标签。轨道 8px，手柄 24px，品牌色 #3250bd；文字使用 PingFang SC。控制柄的独立状态矩阵尚未收录。",
        )}
      </p>
      <section className={styles.demo} aria-label={t("滑块交互试用")}>
        <h3>{t("交互试用")}</h3>
        <p>
          {t(
            "拖动两个手柄选择区间，或使用 Tab、方向键、Home、End 操作；两个手柄保持至少 1% 间隔。切换语言和样式保留数值。",
          )}
        </p>
        <div className={styles.controls}>
          <SegmentedControl label={t("滑块标签")} value={label} onChange={setLabel} options={options} />
          <SegmentedControl
            label={t("滑块禁用状态")}
            value={disabled}
            onChange={setDisabled}
            options={[
              { value: "no", label: t("可用") },
              { value: "yes", label: t("禁用") },
            ]}
          />
        </div>
        <div className={styles.stage}>
          <RangeSample
            values={values}
            label={label}
            onChange={setValues}
            disabled={disabled === "yes"}
            name={t("区间手柄")}
          />
        </div>
        <p role="status">
          {values[0]}% – {values[1]}%
        </p>
        <Button
          onClick={() => {
            setValues([25, 75]);
            setDisabled("no");
          }}
        >
          {t("重置示例")}
        </Button>
      </section>
      <h3>{t("设计静态参考")}</h3>
      <p>
        {t(
          "静态参考保留 320px 原尺寸，窄屏可在卡片内横向滚动。原稿个别填充和手柄位置存在偏移；此处按标称区间展示，完整像素验收尚待完成。仅用于 UI Lab，尚未同步生产 Slider。",
        )}
      </p>
      <div className={styles.grid}>
        {snapshot.entries
          .filter((e) => e.label === label)
          .map((e) => (
            <article key={e.id} className={styles.demo}>
              <h4>
                {e.left}% – {e.right}%
              </h4>
              <p>
                320 × {e.height}px · {e.id}
              </p>
              <div className={styles.reference} inert>
                <div style={{ width: 320 }}>
                  <RangeSample values={[e.left, e.right]} label={label} name="Figma" />
                </div>
              </div>
              <a
                href={`${snapshot.source.split("?")[0]}?node-id=${e.id.replace(":", "-")}`}
                target="_blank"
                rel="noreferrer"
              >
                {t("查看设计节点 ↗")}
              </a>
            </article>
          ))}
      </div>
    </section>
  );
}
