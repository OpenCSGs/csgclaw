import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { GlobalScrollbarController } from "@/bootstrap/GlobalScrollbarController";
import { TooltipProvider } from "@/components/ui";
import { queryClient } from "@/bootstrap/queryClient";

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <GlobalScrollbarController />
        {children}
      </TooltipProvider>
    </QueryClientProvider>
  );
}
