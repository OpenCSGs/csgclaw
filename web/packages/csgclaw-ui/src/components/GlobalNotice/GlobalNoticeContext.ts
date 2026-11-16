import type { ReactNode } from "react";
import { createContext, useContext } from "react";

export type GlobalNoticeInput = {
  title?: string;
  message: string;
  closeLabel: string;
  icon?: ReactNode;
  dismissLabel?: string;
  action?: { label: string; onSelect: () => void };
  tone?: "info" | "warning";
};

export type GlobalNoticeContextValue = {
  showNotice: (notice: GlobalNoticeInput) => void;
};

const noopContext: GlobalNoticeContextValue = { showNotice: () => undefined };

export const GlobalNoticeContext =
  createContext<GlobalNoticeContextValue>(noopContext);

export function useGlobalNotice(): GlobalNoticeContextValue {
  return useContext(GlobalNoticeContext);
}
