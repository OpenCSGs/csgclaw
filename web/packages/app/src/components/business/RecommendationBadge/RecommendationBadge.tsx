import { Star } from "lucide-react";
import { StatusBadge } from "@/components/ui";
import styles from "./RecommendationBadge.module.css";

export function RecommendationBadge({ label }: { label: string }) {
  return (
    <StatusBadge kind="tag" className={styles.badge}>
      <Star size={11} aria-hidden="true" />
      {label}
    </StatusBadge>
  );
}
