import { ApiError, request } from "../../design-system/shared/api";
import type { CashOperation, ClosedTicket, HandoffPreview, Me, NewLine, PosApi, PosState, ShiftClose, Tender, ZReport } from "./types.ts";
import { mockApi } from "./mock.ts";

export { ApiError };

const req = <T>(method: string, path: string, body?: unknown): Promise<T> =>
  request<T>("/api/v1" + path, { method, body: body === undefined ? undefined : JSON.stringify(body) });

// pos/state conditional fetch: If-None-Match with the last ETag, and raw-body
// dedupe when the server sends no ETag. null = unchanged, skip the re-render.
let stateEtag: string | null = null;
let stateBody = "";

export function invalidateStateCache(): void {
  stateEtag = null;
  stateBody = "";
}

async function fetchPosState(): Promise<PosState | null> {
  let res: Response;
  try {
    res = await fetch("/api/v1/pos/state", {
      headers: stateEtag ? { "If-None-Match": stateEtag } : undefined,
    });
  } catch {
    throw new ApiError(0, "network", "No connection");
  }
  if (res.status === 304) return null;
  if (!res.ok) {
    let code = "http_" + res.status;
    let message = res.statusText;
    try {
      const body = await res.json();
      code = body.error?.code ?? code;
      message = body.error?.message ?? message;
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, code, message);
  }
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
  closeTicket: (ticketId, tenders: Tender[]) => req<ClosedTicket>("POST", `/pos/tickets/${ticketId}/close`, { payments: tenders }),
  cashOperation: (shiftId, kind, amount_cents, reason) =>
    req<CashOperation>("POST", `/pos/shifts/${shiftId}/cash-operations`, { kind, amount_cents, reason }),
  closeShift: (shiftId, declared_cents) => req<ShiftClose>("POST", `/pos/shifts/${shiftId}/close`, { declared_cents }),
  zReport: (shiftId) => req<ZReport>("GET", `/pos/shifts/${shiftId}/z-report`),
};

let mockActive = import.meta.env.VITE_MOCK === "1";

export function isMock(): boolean {
  return mockActive;
}

// Real API by contract; falls over to mock fixtures when the API is unreachable
// (network-level failure — ApiError status 0 — not HTTP errors).
export const api: PosApi = new Proxy(realApi, {
  get(target, prop: keyof PosApi) {
    return async (...args: never[]) => {
      if (mockActive) return (mockApi[prop] as (...a: never[]) => unknown)(...args);
      try {
        return await (target[prop] as (...a: never[]) => Promise<unknown>)(...args);
      } catch (e) {
        if (e instanceof ApiError && e.status === 0) {
          mockActive = true;
          return (mockApi[prop] as (...a: never[]) => unknown)(...args);
        }
        throw e;
      }
    };
  },
}) as PosApi;
