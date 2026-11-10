import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { GlobalScrollbarController } from "@/bootstrap/GlobalScrollbarController";
import { GlobalNoticeProvider, TooltipProvider } from "@/components/ui";
import { queryClient } from "@/bootstrap/queryClient";
import { AppThemeSynchronizer } from "./AppThemeSynchronizer";
import { TemplateInstructionsDraftProvider } from "@/components/business/HubDetailPane/TemplateInstructionsDraftProvider";

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <AppThemeSynchronizer />
      <GlobalNoticeProvider>
        <TooltipProvider>
          <GlobalScrollbarController />
          <TemplateInstructionsDraftProvider>{children}</TemplateInstructionsDraftProvider>
        </TooltipProvider>
      </GlobalNoticeProvider>
    </QueryClientProvider>
  );
}
