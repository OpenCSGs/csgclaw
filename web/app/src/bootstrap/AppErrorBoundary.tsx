import React from "react";
import { Button } from "@/components/ui";
import { createTranslator, detectInitialLocale } from "@/shared/i18n";

type AppErrorBoundaryProps = {
  children: React.ReactNode;
};

type AppErrorBoundaryState = {
  error: Error | null;
};

export class AppErrorBoundary extends React.Component<AppErrorBoundaryProps, AppErrorBoundaryState> {
  constructor(props: AppErrorBoundaryProps) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error: Error): AppErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error) {
    console.error(error);
  }

  render() {
    if (this.state.error) {
      const t = createTranslator(detectInitialLocale());
      return (
        <div className="empty-state app-error-state">
          <strong>{t("appErrorBoundaryTitle")}</strong>
          <span>{t("appErrorBoundaryDescription")}</span>
          <Button className="secondary-button" onClick={() => window.location.reload()}>
            {t("appErrorBoundaryReload")}
          </Button>
        </div>
      );
    }
    return this.props.children;
  }
}
