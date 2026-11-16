import { useContext, type ReactNode } from "react";
import { LabView } from "./labViewContext";

export function DesignOnly({ children }: { children: ReactNode }) {
  return useContext(LabView) === "preview" ? null : children;
}

export function PreviewOnly({ children }: { children: ReactNode }) {
  return useContext(LabView) === "design" ? null : children;
}
