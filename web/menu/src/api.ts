import { demoSession } from "./fixtures";
import type { BrowseSession, OpenRequest, OrderInput, TableSession } from "./types";

const COOLDOWN_MS = 90_000;

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status = 0,
    public retryAfterSeconds?: number,
  ) {
    super(message);
  }
}

export interface Client {
  getSession(token: string): Promise<TableSession>;
  getBrowse(restaurantSlug: string, menuSlug: string): Promise<BrowseSession>;
  submitOrder(token: string, order: OrderInput): Promise<void>;
  submitRequest(token: string, type: "waiter" | "bill"): Promise<OpenRequest>;
}

/** Tolerate a backend that still returns the pre-multi-menu flat `menu`. */
export function normalizeSession(raw: TableSession & { menu?: unknown }): TableSession {
  if (!raw.menus && Array.isArray(raw.menu)) {
    return {
      ...raw,
      menus: [{ id: "default", slug: "menu", name: "Menu", is_default: true, categories: raw.menu }],
    };
  }
  return raw;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      headers: { "content-type": "application/json" },
      ...init,
    });
  } catch {
    throw new ApiError("network", "No connection", 0);
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
  throw new ApiError(code, message, res.status, retry);
}

export const httpClient: Client = {
  getSession: async (token) =>
    normalizeSession(await request(`/api/v1/t/${encodeURIComponent(token)}`)),
  getBrowse: (restaurantSlug, menuSlug) =>
    request(`/api/v1/m/${encodeURIComponent(restaurantSlug)}/${encodeURIComponent(menuSlug)}`),
  submitOrder: (token, order) =>
    request(`/api/v1/t/${encodeURIComponent(token)}/orders`, {
      method: "POST",
      body: JSON.stringify(order),
    }),
  submitRequest: async (token, type) => {
    const r = await request<{ request: OpenRequest }>(
      `/api/v1/t/${encodeURIComponent(token)}/requests`,
      { method: "POST", body: JSON.stringify({ type }) },
    );
    return r.request;
  },
};

// Mock client: same contract against sessionStorage, so the full flow —
// including the 90s resend cooldown and one-open-request-per-table — is
// demoable without a backend.
const mockKey = (token: string, k: string) => `aivo:mock:${token}:${k}`;

export const mockClient: Client = {
  async getSession(token) {
    const open: OpenRequest[] = [];
    for (const type of ["waiter", "bill"] as const) {
      const at = sessionStorage.getItem(mockKey(token, "req-" + type));
      if (at) open.push({ type, created_at: at });
    }
    return { ...demoSession, open_requests: open };
  },
  async getBrowse(restaurantSlug, menuSlug) {
    const menu =
      restaurantSlug === demoSession.restaurant.slug
        ? demoSession.menus.find((m) => m.slug === menuSlug)
        : undefined;
    if (!menu) throw new ApiError("not_found", "Unknown menu", 404);
    return { restaurant: demoSession.restaurant, theme: demoSession.theme, menu };
  },
  async submitOrder(token) {
    const last = Number(sessionStorage.getItem(mockKey(token, "order-at")));
    const left = last > 0 ? COOLDOWN_MS - (Date.now() - last) : 0;
    if (left > 0) {
      throw new ApiError(
        "rate_limited",
        "An order from this table just went in.",
        429,
        Math.ceil(left / 1000),
      );
    }
    sessionStorage.setItem(mockKey(token, "order-at"), String(Date.now()));
  },
  async submitRequest(token, type) {
    const key = mockKey(token, "req-" + type);
    const existing = sessionStorage.getItem(key);
    if (existing) return { type, created_at: existing };
    const at = new Date().toISOString();
    sessionStorage.setItem(key, at);
    return { type, created_at: at };
  },
};

export const preferMock = import.meta.env.VITE_MOCK === "1";
