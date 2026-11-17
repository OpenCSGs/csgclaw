import { useRef, useState } from "react";
import { Button, EmptyState, Field, TextInput } from "@/components/ui";
import { LabOptions, LabOptionSelect } from "../LabOptions";
import { useLabTranslation } from "../LabLocale";
import { applicationAsset } from "./assets";
import { EmptyStateIcon } from "./EmptyStateIcon";
import styles from "./EmptyDemo.module.css";

const highlightedTiles = [
  17, 26, 34, 38, 57, 60, 65, 78, 84, 87, 96, 109, 115, 122, 133, 141, 154, 161, 173, 178, 184, 196, 205, 217, 224,
];

const iconOptions = {
  folder: ["文件夹图标", "Folder icon"],
  file: ["文件类型图标", "File type icon"],
  illustration: ["插画", "Illustration"],
  featured: ["特色图标", "Featured icon"],
} as const;

export type EmptyIconVariant = keyof typeof iconOptions;
export type EmptySize = "sm" | "md" | "lg";

export function EmptyDemo() {
  const { locale, text: t } = useLabTranslation();
  const zh = locale === "zh";
  const search = useRef<HTMLInputElement>(null);
  const [size, setSize] = useState<EmptySize>("sm");
  const [variant, setVariant] = useState<EmptyIconVariant>("folder");
  const [projects, setProjects] = useState(["Design system"]);
  const [query, setQuery] = useState("Landing page design");
  const searchTerm = query.trim();
  const filtered = projects.filter((name) => name.toLowerCase().includes(searchTerm.toLowerCase()));

  function clearSearch() {
    setQuery("");
    search.current?.focus();
  }

  return (
    <>
      <LabOptions>
        <LabOptionSelect
          label={zh ? "图标样式" : "Icon style"}
          value={variant}
          onChange={(value) => setVariant(value as EmptyIconVariant)}
          options={Object.entries(iconOptions).map(([value, labels]) => ({ value, label: labels[zh ? 0 : 1] }))}
        />
        <LabOptionSelect
          label={zh ? "空状态尺寸" : "Empty state size"}
          value={size}
          onChange={(value) => setSize(value as EmptySize)}
          options={["sm", "md", "lg"].map((value) => ({ value, label: value }))}
        />
      </LabOptions>
      <div className={styles.variant} aria-live="polite">
        {zh ? "当前变体" : "Current variant"} · {iconOptions[variant][zh ? 0 : 1]} · {size}
      </div>
      <Field label={t("搜索项目")}>
        <TextInput ref={search} value={query} onChange={(event) => setQuery(event.target.value)} />
      </Field>
      {filtered.length ? (
        <ul>
          {filtered.map((name) => (
            <li key={name}>{name}</li>
          ))}
        </ul>
      ) : (
        <div className={styles.preview} data-size={size} data-icon={variant}>
          {variant === "folder" && (
            <div className={styles.pattern} aria-hidden="true">
              {highlightedTiles.map((tile) => (
                <span key={tile} style={{ gridColumn: (tile % 15) + 1, gridRow: Math.floor(tile / 15) + 1 }} />
              ))}
            </div>
          )}
          <EmptyState
            className={styles.empty}
            title={t("未找到项目")}
            description={
              zh
                ? `搜索“${searchTerm}”未匹配到任何项目，请重试。`
                : `Your search “${searchTerm}” did not match any projects. Please try again.`
            }
            icon={<EmptyStateIcon variant={variant} size={size} />}
            actions={
              <div className={styles.actions}>
                <Button size={size === "lg" ? "lg" : "md"} variant="secondaryGray" onClick={clearSearch}>
                  {t("清空搜索")}
                </Button>
                <Button
                  size={size === "lg" ? "lg" : "md"}
                  variant="primary"
                  onClick={() => {
                    setProjects((current) => [...current, `Project ${current.length + 1}`]);
                    clearSearch();
                  }}
                >
                  <img src={applicationAsset("empty-states-imgPlus.svg")} alt="" />
                  {t("新建项目")}
                </Button>
              </div>
            }
          />
        </div>
      )}
      <p role="status">
        {t("项目总数")}：{projects.length}
      </p>
    </>
  );
}
