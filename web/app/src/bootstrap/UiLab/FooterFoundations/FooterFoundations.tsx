import { useState } from "react";
import { Checkbox, SegmentedControl } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import { SectionFooter, type FooterLayout, type FooterBreakpoint } from "./SectionFooter";
import styles from "./FooterFoundations.module.css";
const variants = [
  { id: "3275:372565", layout: "Section", group: true, breakpoint: "Desktop", height: 61 },
  { id: "3275:373080", layout: "Section", group: false, breakpoint: "Desktop", height: 61 },
  { id: "3275:372639", layout: "Section", group: true, breakpoint: "Mobile", height: 57 },
  { id: "3275:373087", layout: "Section", group: false, breakpoint: "Mobile", height: 57 },
  { id: "3275:372530", layout: "Card", group: true, breakpoint: "Desktop", height: 73 },
  { id: "3275:373066", layout: "Card", group: false, breakpoint: "Desktop", height: 73 },
  { id: "3275:372632", layout: "Card", group: true, breakpoint: "Mobile", height: 65 },
  { id: "3275:373073", layout: "Card", group: false, breakpoint: "Mobile", height: 65 },
] as const;
const file = "https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/";
export function FooterFoundations() {
  const { text: t } = useLabTranslation();
  const [layout, setLayout] = useState<FooterLayout>("Section");
  const [breakpoint, setBreakpoint] = useState<FooterBreakpoint>("Desktop");
  const [group, setGroup] = useState(true);
  const [divider, setDivider] = useState(true);
  const [secondary, setSecondary] = useState(true);
  const [actions, setActions] = useState(true);
  const [action, setAction] = useState("");
  return (
    <section id="footer-foundations" className={styles.foundation} aria-labelledby="footer-foundations-title">
      <h2 id="footer-foundations-title">{t("Section footers · 区块页脚")}</h2>
      <p>{t("区块页脚集中放置当前表单或内容区域的辅助操作与主要操作。")}</p>
      <a href={`${file}?node-id=3275-371793`} target="_blank" rel="noreferrer">
        {t("查看 Figma 设计来源 ↗")}
      </a>
      <p>
        {t(
          "已实现全部 8 个页脚变体：Section / Card、按钮组开关、Desktop / Mobile。文字使用 PingFang SC 500，14/20px；按钮高 40px，圆角 8px。",
        )}
      </p>
      <section className={styles.playground} aria-label={t("区块页脚交互试用")}>
        <h3>{t("交互试用")}</h3>
        <div className={styles.controls}>
          <SegmentedControl
            label={t("页脚类型")}
            value={layout}
            onChange={setLayout}
            options={[
              { value: "Section", label: t("Section") },
              { value: "Card", label: t("Card") },
            ]}
          />
          <SegmentedControl
            label={t("设计断点")}
            value={breakpoint}
            onChange={setBreakpoint}
            options={[
              { value: "Desktop", label: t("Desktop") },
              { value: "Mobile", label: t("Mobile") },
            ]}
          />
        </div>
        <div className={styles.controls}>
          {[
            { label: "按钮组", checked: group, set: setGroup },
            { label: "分隔线", checked: divider, set: setDivider },
            { label: "了解更多", checked: secondary, set: setSecondary },
            { label: "操作按钮", checked: actions, set: setActions },
          ].map((option) => (
            <label className={styles.option} key={option.label}>
              <Checkbox
                checked={option.checked}
                onCheckedChange={(value) => option.set(value === true)}
                aria-label={t(option.label)}
              />
              {t(option.label)}
            </label>
          ))}
        </div>
        <div className={styles.adaptive}>
          <SectionFooter
            layout={layout}
            breakpoint={breakpoint}
            buttonGroup={group}
            divider={divider}
            secondaryButton={secondary}
            actions={actions}
            onAction={setAction}
          />
        </div>
        <p role="status">
          {action ? `${t("已触发")}：${t(action)}` : t("可切换时间范围并点击操作，反馈显示在页脚外部。")}
        </p>
      </section>
      <h3>{t("完整设计变体")}</h3>
      <p>{t("原尺寸样本保留 1216px / 688px 宽度，窄屏在样本容器内滚动；交互试用区会在更窄空间自动换行。")}</p>
      {variants.map((variant) => (
        <article key={variant.id} className={styles.sample}>
          <header>
            <h4>
              {t(variant.layout)} · {t(variant.breakpoint)} · {t(variant.group ? "带按钮组" : "无按钮组")}
            </h4>
            <a href={`${file}?node-id=${variant.id.replace(":", "-")}`} target="_blank" rel="noreferrer">
              {variant.id} ↗
            </a>
            <small>
              {variant.breakpoint === "Desktop" ? 1216 : 688} × {variant.height}px
            </small>
          </header>
          <div className={styles.scroll} role="region" tabIndex={0} aria-label={`${t("原尺寸页脚")} ${variant.id}`}>
            <div className={styles.canvas} data-width={variant.breakpoint} data-node-id={variant.id}>
              <SectionFooter
                layout={variant.layout}
                breakpoint={variant.breakpoint}
                buttonGroup={variant.group}
                onAction={setAction}
              />
            </div>
          </div>
        </article>
      ))}
    </section>
  );
}
