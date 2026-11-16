import { applicationAsset } from "./assets";
import { useState } from "react";
import { Button } from "@/components/ui";
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
