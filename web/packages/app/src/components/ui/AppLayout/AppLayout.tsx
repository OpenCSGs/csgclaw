import type { HTMLAttributes, PropsWithChildren, ReactNode } from "react";

export type AppLayoutProps = PropsWithChildren<{
  loadingFallback?: ReactNode;
  ready: boolean;
}>;

export function AppLayout({ children, loadingFallback = null, ready }: AppLayoutProps) {
  if (!ready) {
    return <>{loadingFallback}</>;
  }

  return <>{children}</>;
}

export type AppLayoutLoadingProps = HTMLAttributes<HTMLDivElement>;

export function AppLayoutLoading({ children, className = "empty-state", ...props }: AppLayoutLoadingProps) {
  return (
    <div className={className} {...props}>
      {children}
    </div>
  );
}

export type AppLayoutShellProps = HTMLAttributes<HTMLDivElement>;

export function AppLayoutShell({ children, className, ...props }: AppLayoutShellProps) {
  return (
    <div className={className} {...props}>
      {children}
    </div>
  );
}

export function AppLayoutSidebar({ children }: PropsWithChildren) {
  return <>{children}</>;
}

export function AppLayoutMain({ children }: PropsWithChildren) {
  return <>{children}</>;
}

export function AppLayoutOverlays({ children }: PropsWithChildren) {
  return <>{children}</>;
}
