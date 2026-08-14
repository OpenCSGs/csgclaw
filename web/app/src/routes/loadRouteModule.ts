const ROUTE_CHUNK_RETRY_KEY = "csgclaw.route-chunk-retry";

export type RouteChunkRecoveryEnvironment = {
  clearRetryMarker: () => void;
  getRetryMarker: () => string | null;
  reload: () => void;
  setRetryMarker: (marker: string) => void;
};

export function isRouteChunkLoadError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : typeof error === "string" ? error : "";
  return /failed to fetch dynamically imported module|error loading dynamically imported module|importing a module script failed|failed to load module script|unable to preload css/i.test(
    message,
  );
}

export async function loadRouteModule<T>(
  importer: () => Promise<T>,
  environment: RouteChunkRecoveryEnvironment = browserRouteChunkRecoveryEnvironment(),
): Promise<T> {
  try {
    const loaded = await importer();
    try {
      environment.clearRetryMarker();
    } catch {
      // Storage can be unavailable in hardened browser contexts. A successful
      // import should still render normally.
    }
    return loaded;
  } catch (error) {
    if (!isRouteChunkLoadError(error)) {
      throw error;
    }

    const marker = error instanceof Error ? error.message : String(error);
    let previousMarker: string | null;
    try {
      previousMarker = environment.getRetryMarker();
      if (previousMarker !== marker) {
        environment.setRetryMarker(marker);
      }
    } catch {
      throw error;
    }
    if (previousMarker === marker) {
      throw error;
    }

    try {
      environment.reload();
    } catch {
      throw error;
    }
    return new Promise<T>(() => {});
  }
}

function browserRouteChunkRecoveryEnvironment(): RouteChunkRecoveryEnvironment {
  return {
    clearRetryMarker: () => window.sessionStorage.removeItem(ROUTE_CHUNK_RETRY_KEY),
    getRetryMarker: () => window.sessionStorage.getItem(ROUTE_CHUNK_RETRY_KEY),
    reload: () => window.location.reload(),
    setRetryMarker: (marker) => window.sessionStorage.setItem(ROUTE_CHUNK_RETRY_KEY, marker),
  };
}
