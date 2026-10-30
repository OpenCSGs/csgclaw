import { applicationAsset, resolveAssetStyle } from "./assets";
import { createElement, useState, type CSSProperties, type ReactNode } from "react";
import references from "./figmaReferences.json";
import inventory from "./figmaInventory.json";
import referenceStyles from "./FigmaReferences.module.css";
import styles from "./ApplicationFoundations.module.css";
import { useLabTranslation } from "../LabLocale";
import type { ApplicationKind } from "./registry";

type DesignNode =
  | string
  | { tag: string; css: string; attrs: Record<string, string>; style?: CSSProperties; children: DesignNode[] };
function renderNode(node: DesignNode, key: number): ReactNode {
  if (typeof node === "string") return node;
  return createElement(
    node.tag,
    {
      ...node.attrs,
      ...(node.attrs.src ? { src: applicationAsset(node.attrs.src) } : {}),
      key,
      className: referenceStyles[node.css],
      style: node.style
        ? Object.fromEntries(
            Object.entries(node.style).map(([name, value]) => [
              name,
              typeof value === "string" ? resolveAssetStyle(value) : value,
            ]),
          )
        : undefined,
    },
    ...(node.children.length ? node.children.map(renderNode) : []),
  );
}
export function FigmaReference({ kind }: { kind: ApplicationKind }) {
  const { text: t } = useLabTranslation();
  const [open, setOpen] = useState(false);
  const [indexOpen, setIndexOpen] = useState(false);
  const [query, setQuery] = useState("");
  const reference = references[kind] as unknown as { nodeId: string; tree: DesignNode };
  const source = inventory.find((item) => item.key === kind)!;
  const variants = source.variants.filter((v) => `${v.name} ${v.id}`.toLowerCase().includes(query.toLowerCase()));
  return (
    <>
      <details onToggle={(event) => setOpen(event.currentTarget.open)}>
        <summary>{t("Figma 原尺寸参考")}</summary>
        <p>{t("以下为设计节点的静态结构参考，保留原始文案与尺寸；可在容器内滚动。交互操作请使用上方试用区。")}</p>
        <p>
          {reference.nodeId} ·{" "}
          <a
            href={`https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/?node-id=${reference.nodeId.replace(":", "-")}`}
            target="_blank"
            rel="noreferrer"
          >
            {t("查看设计节点 ↗")}
          </a>
        </p>
        {open && (
          <div className={styles.referenceScroll} tabIndex={0} role="region" aria-label={t("Figma 原尺寸参考")}>
            <div className={`${styles.reference} ${referenceStyles.defaults}`} inert lang="en">
              {renderNode(reference.tree, 0)}
            </div>
          </div>
        )}
      </details>
      <details onToggle={(event) => setIndexOpen(event.currentTarget.open)}>
        <summary>
          {t("设计节点索引")} · {source.variants.length}
        </summary>
        <p>{t("索引包含组合变体及内部基础节点；收录索引不代表该变体已实现。")}</p>
        <label className={styles.field}>
          {t("筛选设计节点")}
          <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} />
        </label>
        {indexOpen && (
          <div className={styles.indexScroll}>
            <ul>
              {variants.map((v) => (
                <li key={v.id}>
                  <a
                    href={`https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/?node-id=${v.id.replace(":", "-")}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {v.name}
                  </a>
                  <small>
                    {v.width} × {v.height}px · {v.id}
                  </small>
                </li>
              ))}
            </ul>
            {!variants.length && <p role="status">{t("未找到匹配节点。")}</p>}
          </div>
        )}
      </details>
    </>
  );
}
