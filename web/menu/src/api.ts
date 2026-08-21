import { demoCustomer, demoOrderHistory, demoSession } from "./fixtures";
import type {
  BrowseSession,
  Customer,
  CustomerMe,
  Handoff,
  OpenRequest,
  OrderInput,
  TableSession,
} from "./types";

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
  /** null when no customer session. */
  me(): Promise<CustomerMe | null>;
  login(email: string, password: string): Promise<Customer>;
  register(email: string, password: string, name: string): Promise<Customer>;
  logout(): Promise<void>;
  submitHandoff(token: string, order: OrderInput): Promise<Handoff>;
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
  me: async () => {
    try {
      return await request<CustomerMe>("/api/v1/customer/me");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return null;
      throw e;
    }
  },
  login: async (email, password) => {
    const r = await request<{ customer: Customer }>("/api/v1/customer/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    return r.customer;
  },
  register: async (email, password, name) => {
    const r = await request<{ customer: Customer }>("/api/v1/customer/register", {
      method: "POST",
      body: JSON.stringify({ email, password, name }),
    });
    return r.customer;
  },
  logout: () => request("/api/v1/customer/logout", { method: "POST" }),
  submitHandoff: (token, order) =>
    request(`/api/v1/t/${encodeURIComponent(token)}/handoff`, {
      method: "POST",
      body: JSON.stringify(order),
    }),
};

/** Pickup code alphabet — A-Z2-9 minus lookalikes 0/O/1/I. */
export const HANDOFF_CHARSET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";

export function genHandoffCode(): string {
  let code = "";
  for (let i = 0; i < 6; i++) {
    code += HANDOFF_CHARSET[Math.floor(Math.random() * HANDOFF_CHARSET.length)];
  }
  return code;
}

/**
 * Deterministic QR-looking SVG data URI for mock mode (NOT scannable — the
 * real backend serves a PNG at qr_url).
 */
export function pseudoQrDataUri(code: string): string {
  const n = 21;
  let seed = 0;
  for (const c of code) seed = (seed * 31 + c.charCodeAt(0)) >>> 0;
  const rnd = () => {
    seed = (seed * 1103515245 + 12345) >>> 0;
    return seed / 4294967296;
  };
  const finder = (x: number, y: number) =>
    `<rect x="${x}" y="${y}" width="7" height="7" fill="#12100f"/><rect x="${x + 1}" y="${y + 1}" width="5" height="5" fill="#fff"/><rect x="${x + 2}" y="${y + 2}" width="3" height="3" fill="#12100f"/>`;
  let cells = "";
  for (let y = 0; y < n; y++) {
    for (let x = 0; x < n; x++) {
      const inFinder =
        (x < 8 && y < 8) || (x >= n - 8 && y < 8) || (x < 8 && y >= n - 8);
      if (!inFinder && rnd() < 0.45) cells += `<rect x="${x}" y="${y}" width="1" height="1" fill="#12100f"/>`;
    }
  }
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-1 -1 ${n + 2} ${n + 2}"><rect x="-1" y="-1" width="${n + 2}" height="${n + 2}" fill="#fff"/>${cells}${finder(0, 0)}${finder(n - 7, 0)}${finder(0, n - 7)}</svg>`;
  return "data:image/svg+xml," + encodeURIComponent(svg);
}

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
  async me() {
    const raw = sessionStorage.getItem(MOCK_CUSTOMER_KEY);
    if (!raw) return null;
    return { customer: JSON.parse(raw), orders: demoOrderHistory };
  },
  async login(email, password) {
    if (email.toLowerCase() === demoCustomer.email && password === "embertest1") {
      sessionStorage.setItem(MOCK_CUSTOMER_KEY, JSON.stringify(demoCustomer));
      return demoCustomer;
    }
    throw new ApiError("invalid_credentials", "Wrong email or password.", 401);
  },
  async register(email, password, name) {
    if (!email.includes("@") || password.length < 8 || !name.trim()) {
      throw new ApiError("invalid", "Email, a name, and a password of 8+ characters.", 422);
    }
    const customer: Customer = { id: "cust-" + Date.now(), email: email.toLowerCase(), name: name.trim() };
    sessionStorage.setItem(MOCK_CUSTOMER_KEY, JSON.stringify(customer));
    return customer;
  },
  async logout() {
    sessionStorage.removeItem(MOCK_CUSTOMER_KEY);
  },
  async submitHandoff(token) {
    // A new handoff replaces the previous active one from the same table.
    const handoff: Handoff = {
      code: genHandoffCode(),
      qr_url: "",
      expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
    };
    handoff.qr_url = pseudoQrDataUri(handoff.code);
    sessionStorage.setItem(mockKey(token, "handoff"), JSON.stringify(handoff));
    return handoff;
  },
};

const MOCK_CUSTOMER_KEY = "aivo:mock:customer";

export const preferMock = import.meta.env.VITE_MOCK === "1";
