import type { ReactNode } from "react";

export function requiredFieldLabel(label: ReactNode, props: { id?: string } = {}): ReactNode {
  return (
    <span id={props.id} className="field-label">
      {label}
      <span className="field-required-star" aria-hidden="true">
        *
      </span>
    </span>
  );
}
