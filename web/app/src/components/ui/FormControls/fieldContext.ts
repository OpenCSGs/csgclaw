import { createContext, useContext } from "react";

export type FieldContextValue = {
  controlId: string;
  labelId?: string;
  descriptionIds: string;
  invalid: boolean;
  required: boolean;
};

export const FieldContext = createContext<FieldContextValue | null>(null);

export function useFieldControlProps<
  T extends {
    id?: string;
    "aria-labelledby"?: string;
    "aria-label"?: string;
    "aria-required"?: boolean | "true" | "false";
    "aria-describedby"?: string;
    "aria-invalid"?: boolean | "true" | "false" | "grammar" | "spelling";
  },
>(props: T) {
  const field = useContext(FieldContext);
  return {
    ...props,
    id: props.id ?? field?.controlId,
    "aria-labelledby": props["aria-labelledby"] ?? (props["aria-label"] ? undefined : field?.labelId),
    "aria-describedby": [field?.descriptionIds, props["aria-describedby"]].filter(Boolean).join(" ") || undefined,
    "aria-invalid": props["aria-invalid"] ?? (field?.invalid || undefined),
    "aria-required": props["aria-required"] ?? (field?.required || undefined),
  };
}
