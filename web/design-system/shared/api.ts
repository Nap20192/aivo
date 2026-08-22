// One error shape and one fetch wrapper for the {error:{code,message}}
// envelope (docs/PLATFORM.md). Apps build their endpoint clients on top.

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public retryAfterSeconds?: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * fetch + envelope parsing. Throws ApiError: status 0 code "network" when the
 * request never completed; otherwise the server's code/message with
 * retry_after_seconds (body) or Retry-After (header) on 429.
 */
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      headers: { "content-type": "application/json" },
      ...init,
    });
  } catch {
    throw new ApiError(0, "network", "No connection");
  }
  if (res.ok) return res.status === 204 ? (undefined as T) : res.json();
  let code = "http_" + res.status;
  let message = res.statusText;
  let retry: number | undefined;
  try {
    const body = await res.json();
    code = body.error?.code ?? code;
    message = body.error?.message ?? message;
    retry = body.error?.retry_after_seconds;
  } catch {
    // non-JSON error body
  }
  if (res.status === 429 && retry === undefined) {
    const h = Number(res.headers.get("retry-after"));
    if (h > 0) retry = h;
  }
  throw new ApiError(res.status, code, message, retry);
}
