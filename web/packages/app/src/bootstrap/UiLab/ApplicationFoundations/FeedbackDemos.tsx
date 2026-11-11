import { applicationAsset } from "./assets";
import { useState } from "react";
import { Button, EmptyState, Field, TextInput } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import styles from "./ApplicationFoundations.module.css";
export function LoadingDemo() {
  const { text: t } = useLabTranslation();
  const [loading, setLoading] = useState(true);
  return (
    <>
      <div className={styles.loading} role="status" aria-live="polite" aria-busy={loading}>
        {loading ? (
          <>
            <span className={styles.spinner}>
              <img src={applicationAsset("loading-imgLoadingCircle.svg")} alt="" width="48" height="48" />
            </span>
            <p>{t("正在加载…")}</p>
          </>
        ) : (
          <p>{t("内容已加载。")}</p>
        )}
      </div>
      <Button onClick={() => setLoading(!loading)}>{t(loading ? "模拟完成" : "重新加载")}</Button>
    </>
  );
}
export function EmptyDemo() {
  const { text: t } = useLabTranslation();
  const [projects, setProjects] = useState(["Design system"]);
  const [query, setQuery] = useState("Landing page");
  const filtered = projects.filter((name) => name.toLowerCase().includes(query.toLowerCase()));
  return (
    <>
      <Field label={t("搜索项目")}>
        <TextInput value={query} onChange={(event) => setQuery(event.target.value)} />
      </Field>
      {filtered.length ? (
        <ul>
          {filtered.map((name) => (
            <li key={name}>{name}</li>
          ))}
        </ul>
      ) : (
        <EmptyState
          className={styles.emptyDemo}
          title={t("未找到项目")}
          description={t("请调整搜索条件，或创建一个新项目。")}
          actions={
            <>
              <Button onClick={() => setQuery("")}>{t("清空搜索")}</Button>
              <Button
                variant="primary"
                onClick={() => {
                  setProjects([...projects, `Project ${projects.length + 1}`]);
                  setQuery("");
                }}
              >
                {t("新建项目")}
              </Button>
            </>
          }
        />
      )}
      <p role="status">
        {t("项目总数")}：{projects.length}
      </p>
    </>
  );
}
