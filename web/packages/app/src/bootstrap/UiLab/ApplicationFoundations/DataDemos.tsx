import { applicationAsset } from "./assets";
import { useState } from "react";
import { Button, Checkbox, Field, SegmentedControl, TextInput } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import styles from "./ApplicationFoundations.module.css";
const people = [
  { name: "Olivia Rhye", role: "设计师" },
  { name: "Phoenix Baker", role: "产品经理" },
  { name: "Lana Steiner", role: "工程师" },
];
export function TableDemo() {
  const { text: t } = useLabTranslation();
  const [query, setQuery] = useState("");
  const [asc, setAsc] = useState(true);
  const [selected, setSelected] = useState<string[]>([]);
  const rows = people
    .filter((p) => `${p.name} ${t(p.role)}`.toLowerCase().includes(query.toLowerCase()))
    .sort((a, b) => (asc ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name)));
  return (
    <>
      <Field label={t("筛选成员")}>
        <TextInput type="search" value={query} onChange={(e) => setQuery(e.target.value)} />
      </Field>
      <div className={styles.tableScroll}>
        <table className={styles.table}>
          <caption>{t("团队成员")}</caption>
          <thead>
            <tr>
              <th>{t("选择")}</th>
              <th aria-sort={asc ? "ascending" : "descending"}>
                <Button variant="linkGray" onClick={() => setAsc(!asc)}>
                  {t("姓名")} {asc ? "↑" : "↓"}
                </Button>
              </th>
              <th>{t("角色")}</th>
              <th>{t("状态")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((person) => (
              <tr key={person.name}>
                <td>
                  <Checkbox
                    aria-label={`${t("选择")} ${person.name}`}
                    checked={selected.includes(person.name)}
                    onCheckedChange={(checked) =>
                      setSelected(
                        checked ? [...selected, person.name] : selected.filter((name) => name !== person.name),
                      )
                    }
                  />
                </td>
                <td>{person.name}</td>
                <td>{t(person.role)}</td>
                <td>{t("活跃")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {!rows.length && <p>{t("没有匹配的成员。")}</p>}
      <p role="status">
        {t("已选择")} {selected.length} / {people.length}
      </p>
      <Button disabled={!selected.length} onClick={() => setSelected([])}>
        {t("清除选择")}
      </Button>
    </>
  );
}
export function MetricDemo() {
  const { text: t, locale } = useLabTranslation();
  const [period, setPeriod] = useState("day");
  const value = period === "day" ? 2000 : 14000;
  return (
    <>
      <SegmentedControl
        label={t("统计周期")}
        value={period}
        onChange={setPeriod}
        options={[
          { value: "day", label: t("过去 24 小时") },
          { value: "week", label: t("过去 7 天") },
        ]}
      />
      <div className={styles.metric}>
        <p>{t("浏览量")}</p>
        <strong>{new Intl.NumberFormat(locale === "zh" ? "zh-CN" : "en-US").format(value)}</strong>
        <span>{t("较上一周期增长")} 100%</span>
      </div>
    </>
  );
}
export function ChartDemo() {
  const { text: t } = useLabTranslation();
  const [period, setPeriod] = useState("week");
  const values = period === "week" ? [45, 80, 55, 90, 65, 75, 60] : [70, 50, 95, 60, 85, 40, 75];
  return (
    <>
      <SegmentedControl
        label={t("图表周期")}
        value={period}
        onChange={setPeriod}
        options={[
          { value: "week", label: t("本周") },
          { value: "previous", label: t("上周") },
        ]}
      />
      <figure className={styles.chart}>
        <figcaption>{t("示例访问量")}</figcaption>
        <div className={styles.bars}>
          {values.map((value, i) => (
            <div key={i}>
              <span style={{ height: `${value}%` }} />
              <small>{i + 1}</small>
            </div>
          ))}
        </div>
      </figure>
      <details>
        <summary>{t("查看数据表")}</summary>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>{t("日期序号")}</th>
              <th>{t("访问量")}</th>
            </tr>
          </thead>
          <tbody>
            {values.map((value, i) => (
              <tr key={i}>
                <td>{i + 1}</td>
                <td>{value}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </>
  );
}
export function ActivityDemo() {
  const { text: t } = useLabTranslation();
  const [unread, setUnread] = useState(true);
  return (
    <>
      <Button disabled={!unread} onClick={() => setUnread(false)}>
        {t("全部标为已读")}
      </Button>
      <ol className={styles.feed}>
        {["创建了项目", "上传了文件", "完成了设计评审"].map((action, i) => (
          <li key={action}>
            <img src={applicationAsset(`activity-feeds-imgAvatar${i ? i : ""}.png`)} alt="" width="32" height="32" />
            <div>
              <strong>{people[i].name}</strong>
              <p>{t(action)}</p>
              <small>
                {i + 1} {t("分钟前")}
              </small>
            </div>
            <span>{t(unread ? "未读" : "已读")}</span>
          </li>
        ))}
      </ol>
      <p role="status">{t(unread ? "有未读活动。" : "所有活动已读。")}</p>
    </>
  );
}
