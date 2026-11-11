import { useState } from "react";
import {
  Button,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import { ThemeGlyph } from "../ThemeGlyph";
import chevron from "./assets/chevron.svg";
import profile from "./assets/profile.svg";
import settings from "./assets/settings.svg";
import signOut from "./assets/sign-out.svg";
import styles from "./DropdownPreview.module.css";

export function DropdownPreview() {
  const { text: t } = useLabTranslation();
  const [selected, setSelected] = useState("");
  return (
    <div className={styles.preview}>
      <a
        href="https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/?node-id=3281-383089"
        target="_blank"
        rel="noreferrer"
      >
        {t("查看下拉菜单设计稿")}
      </a>
      <div className={styles.row}>
        <DropdownMenuRoot>
          <DropdownMenuTrigger asChild>
            <Button className={styles.trigger}>
              {t("账户菜单")}
              <ThemeGlyph src={chevron} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className={styles.menu}>
            <DropdownMenuItem className={styles.item} onSelect={() => setSelected("查看个人资料")}>
              <ThemeGlyph src={profile} />
              {t("查看个人资料")}
            </DropdownMenuItem>
            <DropdownMenuItem className={styles.item} onSelect={() => setSelected("账户设置")}>
              <ThemeGlyph src={settings} />
              {t("账户设置")}
            </DropdownMenuItem>
            <DropdownMenuSeparator className={styles.separator} />
            <DropdownMenuItem className={styles.item} disabled>
              <ThemeGlyph src={signOut} />
              {t("退出登录")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenuRoot>
        <Button size="sm" onClick={() => setSelected("")}>
          {t("重置示例")}
        </Button>
        <span role="status">{selected && t(selected)}</span>
      </div>
    </div>
  );
}
