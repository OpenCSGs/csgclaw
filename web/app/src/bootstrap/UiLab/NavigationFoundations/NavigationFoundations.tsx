import { useId, useState, type ReactNode } from "react";
import { Tabs } from "radix-ui";
import { Button } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import styles from "./NavigationFoundations.module.css";

const file = "https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/Untitled-UI";
function Foundation({
  id,
  title,
  description,
  node,
  scope,
  children,
}: {
  id: string;
  title: string;
  description: string;
  node: string;
  scope: string;
  children: ReactNode;
}) {
  const { text: t } = useLabTranslation();
  return (
    <section id={id} className={styles.foundation} aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`}>{t(title)}</h2>
      <p>{t(description)}</p>
      <a href={`${file}?node-id=${node}`} target="_blank" rel="noreferrer">
        {t("查看 Figma 设计来源 ↗")}
      </a>
      <p>{t(scope)}</p>
      <div className={styles.stage}>{children}</div>
      <p>{t("当前为 UI Lab 基础样本；其他变体与完整像素验收待补充，尚未迁移生产页面。")}</p>
    </section>
  );
}
const tabNames = ["个人信息", "个人资料", "密码", "团队", "套餐", "账单", "邮件", "通知", "集成", "API"];
const tabDescriptions = [
  "管理姓名与邮箱。",
  "编辑公开资料。",
  "管理登录密码。",
  "查看团队成员。",
  "查看当前套餐。",
  "管理付款信息。",
  "管理邮件偏好。",
  "管理通知偏好。",
  "配置外部集成。",
  "管理 API 访问。",
];
export function TabsFoundations() {
  const { text: t } = useLabTranslation();
  return (
    <Foundation
      id="tabs-foundations"
      title="Tabs · 选项卡"
      description="选项卡将相关内容组织到不同面板中，每次显示一个面板。"
      node="43-0"
      scope="已对齐 sm 下划线选项卡：32px 高、14/20px Medium、12px 间距、2px 品牌色指示线。其他类型与尺寸待补充。"
    >
      <Tabs.Root defaultValue="0">
        <Tabs.List className={styles.tabs} aria-label={t("设置选项卡")}>
          {tabNames.map((name, i) => (
            <Tabs.Trigger className={styles.tab} key={name} value={String(i)}>
              {t(name)}
              {i === 7 && <span className={styles.badge}>2</span>}
            </Tabs.Trigger>
          ))}
        </Tabs.List>
        {tabNames.map((name, i) => (
          <Tabs.Content className={styles.tabPanel} key={name} value={String(i)}>
            <h3>{t(name)}</h3>
            <p>{t(tabDescriptions[i])}</p>
          </Tabs.Content>
        ))}
      </Tabs.Root>
    </Foundation>
  );
}
export function FooterFoundations() {
  const { text: t } = useLabTranslation();
  const id = useId();
  const [value, setValue] = useState("");
  const [saved, setSaved] = useState("");
  const [help, setHelp] = useState(false);
  const [notice, setNotice] = useState("");
  return (
    <Foundation
      id="footer-foundations"
      title="Section footers · 区块页脚"
      description="区块页脚集中放置当前表单或内容区域的辅助操作与主要操作。"
      node="3275-371793"
      scope="已对齐 Section / 无按钮组 / Desktop：顶部分隔线、20px 间距、40px 操作按钮。窄屏自动换行；Card 与按钮组变体待补充。"
    >
      <label className={styles.field} htmlFor={id}>
        {t("草稿名称")}
        <input id={id} value={value} onChange={(e) => setValue(e.target.value)} />
      </label>
      <footer className={styles.footer}>
        <Button className={styles.action} variant="linkGray" onClick={() => setHelp(!help)} aria-expanded={help}>
          {t("了解更多")}
        </Button>
        <div className={styles.actions}>
          <Button
            className={styles.action}
            variant="tertiaryGray"
            onClick={() => {
              setValue(saved);
              setNotice("已恢复已保存内容。");
            }}
          >
            {t("重置")}
          </Button>
          <Button
            className={styles.action}
            onClick={() => {
              setValue(saved);
              setNotice("已取消本次修改。");
            }}
          >
            {t("取消")}
          </Button>
          <Button
            className={styles.action}
            variant="primary"
            onClick={() => {
              setSaved(value);
              setNotice("已保存本次草稿。");
            }}
          >
            {t("保存")}
          </Button>
        </div>
      </footer>
      {help && <p>{t("此处仅演示本地草稿，不提交服务器。")}</p>}
      <p role="status">{t(notice)}</p>
    </Foundation>
  );
}
export function PaginationFoundations() {
  const { text: t } = useLabTranslation();
  const [page, setPage] = useState(1);
  const [circle, setCircle] = useState(false);
  return (
    <Foundation
      id="pagination-foundations"
      title="Pagination · 分页"
      description="分页将较长的数据集合分为多个页面，并帮助用户定位当前位置。"
      node="225-7288"
      scope="已对齐 40px 页码、8px 方形圆角与圆形页码、14/20px Regular、2px 页码间距。移动端采用换行演示；其他分页布局与圆点变体待补充。"
    >
      <label className={styles.choice}>
        <input type="checkbox" checked={circle} onChange={(e) => setCircle(e.target.checked)} />
        {t("圆形页码")}
      </label>
      <ul className={styles.records}>
        {Array.from({ length: 3 }, (_, i) => (
          <li key={i}>
            {t("示例记录")} {(page - 1) * 3 + i + 1}
          </li>
        ))}
      </ul>
      <nav className={styles.pagination} aria-label={t("示例分页")}>
        <Button className={styles.action} variant="linkGray" disabled={page === 1} onClick={() => setPage(page - 1)}>
          <img src="ui-lab/figma/navigation/arrow-left.svg" alt="" width="20" height="20" />
          {t("上一页")}
        </Button>
        <div className={styles.numbers}>
          {Array.from({ length: 5 }, (_, i) => (
            <button
              key={i}
              type="button"
              className={styles.number}
              data-circle={circle}
              aria-label={`${t("页码")} ${i + 1}`}
              aria-current={page === i + 1 ? "page" : undefined}
              onClick={() => setPage(i + 1)}
            >
              {i + 1}
            </button>
          ))}
        </div>
        <Button className={styles.action} variant="linkGray" disabled={page === 5} onClick={() => setPage(page + 1)}>
          {t("下一页")}
          <img src="ui-lab/figma/navigation/arrow-right.svg" alt="" width="20" height="20" />
        </Button>
      </nav>
      <p role="status">
        {t("当前页")} {page} / 5
      </p>
    </Foundation>
  );
}
const stepTitles = ["个人信息", "团队", "确认"];
const stepDescriptions = ["管理姓名与邮箱。", "查看团队成员。", "检查并完成设置。"];
export function StepsFoundations() {
  const { text: t } = useLabTranslation();
  const [current, setCurrent] = useState(0);
  return (
    <Foundation
      id="steps-foundations"
      title="Progress steps · 步骤进度"
      description="步骤进度展示多步骤流程的位置，以及已完成和未完成的步骤。"
      node="53-1288"
      scope="已对齐 sm 文字线条步骤的未完成、当前和已完成状态：4px 顶部线条、12px 顶部间距、14/20px 文字。图标、数字与其他布局待补充。"
    >
      <ol className={styles.steps} aria-label={t("设置进度")}>
        {stepTitles.map((title, i) => (
          <li
            key={title}
            className={styles.step}
            data-state={i < current ? "complete" : i === current ? "current" : "incomplete"}
            aria-current={current === i ? "step" : undefined}
          >
            <strong>{t(title)}</strong>
            <span>{t(stepDescriptions[i])}</span>
            <small>{t(i < current ? "已完成" : i === current ? "进行中" : "未开始")}</small>
          </li>
        ))}
      </ol>
      <div className={styles.actions}>
        <Button disabled={current === 0} onClick={() => setCurrent(current - 1)}>
          {t("上一步")}
        </Button>
        <Button variant="primary" disabled={current === 3} onClick={() => setCurrent(current + 1)}>
          {t(current === 2 ? "完成" : "下一步")}
        </Button>
        <Button variant="tertiaryGray" onClick={() => setCurrent(0)}>
          {t("重新开始")}
        </Button>
      </div>
      <p role="status">{t(current === 3 ? "所有步骤已完成。" : "按顺序完成设置步骤。")}</p>
    </Foundation>
  );
}
