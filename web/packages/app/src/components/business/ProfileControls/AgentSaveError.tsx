import type { ReactNode } from "react";
import { InlineError, LinkButton } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";

export function AgentSaveError({
  children,
  billingURL,
  t,
}: {
  children: ReactNode;
  billingURL?: string;
  t: TranslateFn;
}) {
  return (
    <InlineError className="flex flex-wrap items-center gap-3" role="alert">
      <span className="min-w-0 grow basis-64">{children}</span>
      {billingURL ? (
        <LinkButton size="sm" variant="primary" href={billingURL} target="_blank" rel="noopener noreferrer">
          {t("rechargeAccount")}
        </LinkButton>
      ) : null}
    </InlineError>
  );
}
