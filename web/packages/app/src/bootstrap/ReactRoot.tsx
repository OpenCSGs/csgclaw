import { App } from "@/bootstrap/App";
import { AppErrorBoundary } from "@/bootstrap/AppErrorBoundary";

export function ReactRoot() {
  return (
    <AppErrorBoundary>
      <App />
    </AppErrorBoundary>
  );
}
