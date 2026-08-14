export type ApiError = {
  status: number;
  message: string;
  code?: string;
};

export type ApiRequestOptions = Omit<RequestInit, "body"> & {
  body?: BodyInit | null;
  json?: unknown;
};

const absoluteURLPattern: RegExp = /^[a-zA-Z][a-zA-Z0-9+.-]*:/;

export async function request<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  const { json, ...requestOptions } = options;
  const headers = new Headers(requestOptions.headers);
  let body = requestOptions.body;

  if (json !== undefined) {
    body = JSON.stringify(json);
  }

  if (body && !(body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (!headers.has("Accept")) {
    headers.set("Accept", "application/json");
  }

  const response = await fetch(resolveRequestPath(path), {
    ...requestOptions,
    body,
    headers,
  });

  if (!response.ok) {
    throw await apiErrorFromResponse(response);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  if (!text.trim()) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

export async function requestText(path: string, options: ApiRequestOptions = {}): Promise<string> {
  const { json, ...requestOptions } = options;
  const headers = new Headers(requestOptions.headers);
  let body = requestOptions.body;

  if (json !== undefined) {
    body = JSON.stringify(json);
  }

  if (body && !(body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (!headers.has("Accept")) {
    headers.set("Accept", "text/plain");
  }

  const response = await fetch(resolveRequestPath(path), {
    ...requestOptions,
    body,
    headers,
  });

  if (!response.ok) {
    throw await apiErrorFromResponse(response);
  }

  if (response.status === 204) {
    return "";
  }

  return response.text();
}

export function resolveRequestPath(path: string): string {
  const value = String(path || "").trim();
  if (!value || value.startsWith("#") || value.startsWith("//") || absoluteURLPattern.test(value)) {
    return value;
  }
  return value.replace(/^\/+/, "");
}

export function get<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  return request<T>(path, options);
}

export function post<T>(path: string, json?: unknown, options: ApiRequestOptions = {}): Promise<T> {
  return request<T>(path, { ...options, method: "POST", json });
}

export function put<T>(path: string, json?: unknown, options: ApiRequestOptions = {}): Promise<T> {
  return request<T>(path, { ...options, method: "PUT", json });
}

export function patch<T>(path: string, json?: unknown, options: ApiRequestOptions = {}): Promise<T> {
  return request<T>(path, { ...options, method: "PATCH", json });
}

export function del<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  return request<T>(path, { ...options, method: "DELETE" });
}

export function errorMessage(error: unknown, fallback = ""): string {
  if (error && typeof error === "object" && "message" in error) {
    const value = (error as { message?: unknown }).message;
    if (typeof value === "string" && value.trim()) {
      return value;
    }
  }
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  return fallback;
}

export async function readResponseText(response: Response | null | undefined): Promise<string> {
  if (!response) {
    return "";
  }
  try {
    return (await response.text()).trim();
  } catch (_) {
    return "";
  }
}

async function apiErrorFromResponse(response: Response): Promise<ApiError> {
  const raw = (await readResponseText(response)) || response.statusText;
  try {
    const payload = JSON.parse(raw) as { error?: { code?: unknown; message?: unknown } };
    const code = typeof payload?.error?.code === "string" ? payload.error.code.trim() : "";
    const message = typeof payload?.error?.message === "string" ? payload.error.message.trim() : "";
    if (code || message) {
      return { status: response.status, code: code || undefined, message: message || code };
    }
  } catch {
    // Preserve legacy text error responses.
  }
  return { status: response.status, message: raw };
}
