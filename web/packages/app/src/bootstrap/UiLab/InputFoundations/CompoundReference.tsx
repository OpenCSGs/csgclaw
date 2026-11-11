import { useLabTranslation } from "../LabLocale";
import snapshot from "../figmaInputs.json";
import help from "./assets/e9c84.svg";
import mastercard from "./assets/6cede.svg";
import chevron from "./assets/3b8bf.svg";
import copy from "./assets/84b42.svg";
import styles from "./InputFoundations.module.css";

type Entry = (typeof snapshot.entries)[number];
const labels: Record<string, [string, string]> = {
  "Payment input": ["Card number", "Card number"],
  Tags: ["Users", "Add users"],
  "Leading dropdown": ["Phone number", "+1 (555) 000-0000"],
  "Leading text": ["Website", "www.untitledui.com"],
  "Trailing button": ["Website", "www.untitledui.com"],
  "Trailing dropdown": ["Sale amount", "1,000.00"],
};

// Frozen, inert source references. Interactive behavior is demonstrated by InputSample.
export function CompoundReference({ entry }: { entry: Entry }) {
  const { text: labText } = useLabTranslation();

  const verification = entry.group === "Verification code input field";
  const type = entry.Type ?? "";
  const [label, placeholder] = verification ? ["Secure code", "0"] : labels[type];
  return (
    <div className={styles.compound} data-type={type} style={{ width: entry.width }}>
      <div className={styles.referenceLabel}>
        {label}
        {!verification && <span>*</span>}
      </div>
      {verification ? (
        <div className={styles.codeRow}>
          {Array.from({ length: 4 }, (_, i) => (
            <div key={i} className={styles.codeDigit}>
              0
            </div>
          ))}
        </div>
      ) : (
        <div className={styles.compoundBox}>
          {type === "Payment input" && (
            <span className={styles.cardIcon}>
              <img src={mastercard} alt="" />
            </span>
          )}
          {type === "Leading dropdown" && (
            <span className={styles.addon}>
              {labText("US ")}
              <img src={chevron} alt="" />
            </span>
          )}
          {type === "Leading text" && <span className={styles.prefix}>{labText("http://")}</span>}
          <div className={styles.compoundContent}>
            {type === "Trailing dropdown" && <span className={styles.currency}>$</span>}
            <span className={styles.placeholder}>{placeholder}</span>
            <img src={help} alt="" />
          </div>
          {type === "Trailing dropdown" && (
            <span className={styles.addon}>
              {labText("USD ")}
              <img src={chevron} alt="" />
            </span>
          )}
          {type === "Trailing button" && (
            <span className={styles.copy}>
              <img src={copy} alt="" />
              <span>{labText("Copy")}</span>
            </span>
          )}
        </div>
      )}
      <div className={styles.referenceHint}>{labText("This is a hint text to help user.")}</div>
    </div>
  );
}
