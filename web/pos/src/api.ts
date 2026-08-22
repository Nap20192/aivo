import type { HandoffPreview, Me, NewLine, PosApi, PosState, PostedShift } from "./types.ts";
import { mockApi } from "./mock.ts";

export class ApiError extends Error {
  code: string;
  status: number;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch("/api/v1" + path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw await toApiError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

async function toApiError(res: Response): Promise<ApiError> {
  let code = "error";
  let message = res.statusText;
  try {
    const e = (await res.json()) as { error?: { code?: string; message?: string } };
    code = e.error?.code ?? code;
    message = e.error?.message ?? message;
  } catch {
    /* non-JSON error body */
  }
  return new ApiError(res.status, code, message);
}

// pos/state conditional fetch: If-None-Match with the last ETag, and raw-body
// dedupe when the server sends no ETag. null = unchanged, skip the re-render.
let stateEtag: string | null = null;
let stateBody = "";

export function invalidateStateCache(): void {
  stateEtag = null;
  stateBody = "";
}

async function fetchPosState(): Promise<PosState | null> {
  const res = await fetch("/api/v1/pos/state", {
    headers: stateEtag ? { "If-None-Match": stateEtag } : undefined,
  });
  if (res.status === 304) return null;
  if (!res.ok) throw await toApiError(res);
  stateEtag = res.headers.get("ETag");
  const text = await res.text();
  if (text === stateBody) return null;
  stateBody = text;
  return JSON.parse(text) as PosState;
}

const realApi: PosApi = {
  login: (email, password) => req<Me>("POST", "/auth/login", { email, password }),
  me: () => req<Me>("GET", "/auth/me"),
  state: fetchPosState,
  openShift: (opening_float_cents) => req("POST", "/pos/shifts", { opening_float_cents }),
  addLines: (tableId, lines: NewLine[]) => req("POST", `/pos/tables/${tableId}/lines`, { lines }),
  fire: (ticketId) => req("POST", `/pos/tickets/${ticketId}/fire`),
  handoff: (code) => req<HandoffPreview>("GET", `/pos/handoff/${encodeURIComponent(code)}`),
  acceptHandoff: (code, table_id) => req("POST", `/pos/handoff/${encodeURIComponent(code)}/accept`, { table_id }),
  ack: (requestId) => req("POST", `/pos/requests/${requestId}/ack`),
  dismiss: (requestId) => req("POST", `/pos/requests/${requestId}/dismiss`),
  closeShift: (shiftId, declared_cents) => req<PostedShift>("POST", `/pos/shifts/${shiftId}/close`, { declared_cents }),
};

let mockActive = import.meta.env.VITE_MOCK === "1";

export function isMock(): boolean {
  return mockActive;
}

// Real API by contract; falls over to mock fixtures when the API is unreachable
// (network-level failure, not HTTP errors).
export const api: PosApi = new Proxy(realApi, {
  get(target, prop: keyof PosApi) {
    return async (...args: never[]) => {
      if (mockActive) return (mockApi[prop] as (...a: never[]) => unknown)(...args);
      try {
        return await (target[prop] as (...a: never[]) => Promise<unknown>)(...args);
      } catch (e) {
        if (e instanceof TypeError) {
          mockActive = true;
          return (mockApi[prop] as (...a: never[]) => unknown)(...args);
        }
        throw e;
      }
    };
  },
}) as PosApi;
