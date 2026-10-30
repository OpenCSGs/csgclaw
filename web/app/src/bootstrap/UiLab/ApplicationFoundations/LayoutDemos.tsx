import { useState } from "react";
import { Breadcrumb, Button, Field, PanelHeader, TextInput } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import styles from "./ApplicationFoundations.module.css";
import type { ApplicationKind } from "./registry";

export function HeaderDemo({ kind }: { kind: ApplicationKind }) {
  const { text: t } = useLabTranslation();
  const [query, setQuery] = useState("");
  const [members, setMembers] = useState(["Olivia Rhye", "Phoenix Baker"]);
  const [notice, setNotice] = useState("");
  return (
    <>
      <PanelHeader
        actions={
          <Button
            variant="primary"
            onClick={() => {
              setMembers([...members, `Demo ${members.length + 1}`]);
              setNotice("已添加示例成员。");
            }}
          >
            {t("添加成员")}
          </Button>
        }
      >
        <div>
          <h3>{t("团队成员")}</h3>
          <p>{t("管理团队成员与账号权限。")}</p>
        </div>
      </PanelHeader>
      {kind === "page-headers" && (
        <Field label={t("搜索成员")}>
          <TextInput type="search" value={query} onChange={(e) => setQuery(e.target.value)} />
        </Field>
      )}
      <ul>
        {members
          .filter((name) => name.toLowerCase().includes(query.toLowerCase()))
          .map((name) => (
            <li key={name}>{name}</li>
          ))}
      </ul>
      <p role="status">
        {t(notice)} {members.filter((name) => name.toLowerCase().includes(query.toLowerCase())).length} {t("位成员")}
      </p>
    </>
  );
}
export function NavigationDemo() {
  const { text: t } = useLabTranslation();
  const [view, setView] = useState("概览");
  const [open, setOpen] = useState(true);
  return (
    <div className={styles.navigationDemo}>
      <nav aria-label={t("示例应用导航")}>
        <Button aria-expanded={open} onClick={() => setOpen(!open)}>
          {t(open ? "收起导航" : "展开导航")}
        </Button>
        {open &&
          ["概览", "项目", "任务", "设置"].map((label) => (
            <Button
              key={label}
              variant="tertiaryGray"
              aria-current={view === label ? "page" : undefined}
              onClick={() => setView(label)}
            >
              {t(label)}
            </Button>
          ))}
      </nav>
      <section aria-live="polite">
        <h3>{t(view)}</h3>
        <p>{t("当前导航内容已切换。")}</p>
      </section>
    </div>
  );
}
export function BreadcrumbDemo() {
  const { text: t } = useLabTranslation();
  const [level, setLevel] = useState(3);
  const labels = ["设置", "工作区", "团队"];
  return (
    <>
      <Breadcrumb
        aria-label={t("示例面包屑")}
        home={{ label: t("首页"), onClick: () => setLevel(0) }}
        items={labels
          .slice(0, level)
          .map((label, i) => ({ label: t(label), current: i === level - 1, onClick: () => setLevel(i + 1) }))}
      />
      <p role="status">
        {t("当前位置")}：{t(level ? labels[level - 1] : "首页")}
      </p>
      <Button onClick={() => setLevel(3)}>{t("恢复完整路径")}</Button>
    </>
  );
}
export function InlineCtaDemo() {
  const { text: t } = useLabTranslation();
  const [visible, setVisible] = useState(true);
  const [details, setDetails] = useState(false);
  return visible ? (
    <div className={styles.cta}>
      <h3>{t("新版本已发布")}</h3>
      <p>{t("了解最新功能和界面改进。")}</p>
      <div className={styles.actions}>
        <Button onClick={() => setVisible(false)}>{t("暂时关闭")}</Button>
        <Button variant="primary" aria-expanded={details} onClick={() => setDetails(!details)}>
          {t("查看更新")}
        </Button>
      </div>
      {details && <p>{t("本次更新新增应用组件设计参考与交互示例。")}</p>}
    </div>
  ) : (
    <Button onClick={() => setVisible(true)}>{t("重新显示提示")}</Button>
  );
}
export function DividerDemo() {
  const { text: t } = useLabTranslation();
  const [label, setLabel] = useState("今天");
  return (
    <>
      <Field label={t("分割器标签")}>
        <TextInput value={label} onChange={(e) => setLabel(e.target.value)} />
      </Field>
      <div className={styles.divider}>
        <span />
        {label === "今天" ? t(label) : label}
        <span />
      </div>
    </>
  );
}
