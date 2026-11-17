import { Fragment, useState } from "react";
import { Button } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import styles from "./CodeDemo.module.css";

type Highlight = "comment" | "keyword" | "identifier";
type CodePart = string | readonly [Highlight, string];

// Keep the original Figma text and its explicit color ranges together.
const lines: readonly (readonly CodePart[])[] = [
  [["comment", "// Imports"]],
  [
    ["keyword", "import"],
    " ",
    ["identifier", "mongoose"],
    ", { ",
    ["identifier", "Schema"],
    " } ",
    ["keyword", "from"],
    " 'untitled'",
  ],
  [],
  [["comment", "// Collection name"]],
  [["keyword", "export const"], " ", ["identifier", "collection"], " = 'Design'"],
  [],
  [["comment", "// Schema"]],
  [["keyword", "const"], " ", ["identifier", "schema"], " = ", ["keyword", "new"], " Schema({"],
  ["  ", ["identifier", "name"], ": {"],
  ["    ", ["identifier", "type"], ": String,"],
  ["    ", ["identifier", "required"], ": true"],
  ["  },"],
  ["  ", ["identifier", "description"], ": {"],
  ["    ", ["identifier", "type"], ": String"],
  ["  }"],
  ["}, {", ["identifier", "timestamps"], ": true})"],
  [],
  [["comment", "// Model"]],
  [
    ["keyword", "export default"],
    " untitled.model(",
    ["identifier", "collection"],
    ", ",
    ["identifier", "schema"],
    ", ",
    ["identifier", "collection"],
    ")",
  ],
  [],
  [],
];

const example = lines
  .map((line) => line.map((part) => (typeof part === "string" ? part : part[1])).join(""))
  .join("\n");

export function CodeDemo() {
  const { text: t } = useLabTranslation();
  const [notice, setNotice] = useState("");
  return (
    <div className={styles.demo}>
      <div className={styles.snippet}>
        <div className={styles.gutter} aria-hidden="true">
          {lines.map((_, index) => (
            <div key={index}>{index + 1}</div>
          ))}
        </div>
        <pre className={styles.code} tabIndex={0} aria-label={t("代码片段")}>
          <code>
            {lines.map((line, index) => (
              <Fragment key={index}>
                {line.map((part, partIndex) =>
                  typeof part === "string" ? (
                    part
                  ) : (
                    <span className={styles[part[0]]} key={partIndex}>
                      {part[1]}
                    </span>
                  ),
                )}
                {index < lines.length - 1 && "\n"}
              </Fragment>
            ))}
          </code>
        </pre>
      </div>
      <div className={styles.actions}>
        <Button
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(example);
              setNotice("代码已复制。");
            } catch {
              setNotice("复制失败，请选择代码后手动复制。");
            }
          }}
        >
          {t("复制代码")}
        </Button>
        <span role="status">{t(notice)}</span>
      </div>
    </div>
  );
}
