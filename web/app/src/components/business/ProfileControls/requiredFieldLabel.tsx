import type { ReactNode } from "react";

export function requiredFieldLabel(label: ReactNode): ReactNode {
  return (
    <span className="field-label">
      {label}
      <span className="field-required-star" aria-hidden="true">
        *
      </span>
    </span>
  );
}
