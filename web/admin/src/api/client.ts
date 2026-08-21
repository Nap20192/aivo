// Typed client for the /api/v1 contract (docs/PLATFORM.md).
// Mock mode: VITE_MOCK=1, or automatic fallback when the API is unreachable.
import { ApiError } from "./error";
import { mockApi } from "./mock";
import type {
  AssistantApplyResult,
  AssistantMessage,
  Category,
  Me,
  Menu,
  MenuItem,
  Plan,
  Restaurant,
  Role,
  StaffMember,
  Subscription,
  Table,
  Theme,
} from "./types";

const BASE = "/api/v1";

let mocked = import.meta.env.VITE_MOCK === "1";

export function isMocked(): boolean {
  return mocked;
}

const mockListeners = new Set<() => void>();
export function onMockChange(fn: () => void): () => void {
  mockListeners.add(fn);
  return () => mockListeners.delete(fn);
}

function enableMock() {
  if (!mocked) {
    mocked = true;
    mockListeners.forEach((fn) => fn());
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(BASE + path, {
      method,
      credentials: "same-origin",
      headers: body instanceof FormData ? undefined : body !== undefined ? { "Content-Type": "application/json" } : undefined,
      body: body instanceof FormData ? body : body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    // API unreachable: switch this session to mock mode.
    enableMock();
    throw new ApiError("network", "API unreachable — switched to demo mode.", 0);
  }
  if (!res.ok) {
    let code = "error";
    let message = res.statusText || "Request failed.";
    try {
      const data = await res.json();
      if (data?.error) {
        code = data.error.code ?? code;
        message = data.error.message ?? message;
      }
    } catch {
      // non-JSON error body
    }
    throw new ApiError(code, message, res.status);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

// Wrap a real call so that a network failure retries once against the mock.
async function withFallback<T>(real: () => Promise<T>, mock: () => Promise<T>): Promise<T> {
  if (mocked) return mock();
  try {
    return await real();
  } catch (e) {
    if (e instanceof ApiError && e.status === 0) return mock();
    throw e;
  }
}

export const api = {
  register(input: {
    org_name: string;
    restaurant_name: string;
    email: string;
    password: string;
  }): Promise<Me> {
    return withFallback(
      () => request("POST", "/auth/register", input),
      () => mockApi.register(input),
    );
  },

  login(input: { email: string; password: string }): Promise<Me> {
    return withFallback(
      () => request("POST", "/auth/login", input),
      () => mockApi.login(input),
    );
  },

  logout(): Promise<void> {
    return withFallback(
      () => request("POST", "/auth/logout"),
      () => mockApi.logout(),
    );
  },

  me(): Promise<Me> {
    return withFallback(
      () => request("GET", "/auth/me"),
      () => mockApi.me(),
    );
  },

  getRestaurant(id: string): Promise<Restaurant> {
    return withFallback(
      () => request("GET", `/restaurants/${id}`),
      () => mockApi.getRestaurant(id),
    );
  },

  patchRestaurant(id: string, patch: Partial<Restaurant>): Promise<Restaurant> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}`, patch),
      () => mockApi.patchRestaurant(id, patch),
    );
  },

  getTheme(id: string): Promise<Theme> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/theme`),
      () => mockApi.getTheme(id),
    );
  },

  putTheme(id: string, theme: Theme): Promise<Theme> {
    return withFallback(
      () => request("PUT", `/restaurants/${id}/theme`, theme),
      () => mockApi.putTheme(id, theme),
    );
  },

  generateTheme(id: string): Promise<{ proposal: Theme; based_on: string }> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/theme/generate`),
      () => mockApi.generateTheme(id),
    );
  },

  listMenus(id: string): Promise<Menu[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/menus`),
      () => mockApi.listMenus(id),
    );
  },

  createMenu(id: string, input: { name: string; slug: string }): Promise<Menu> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/menus`, input),
      () => mockApi.createMenu(id, input),
    );
  },

  updateMenu(id: string, menuId: string, patch: Partial<Menu>): Promise<Menu> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/menus/${menuId}`, patch),
      () => mockApi.updateMenu(id, menuId, patch),
    );
  },

  deleteMenu(id: string, menuId: string, force = false): Promise<void> {
    return withFallback(
      () =>
        request(
          "DELETE",
          `/restaurants/${id}/menus/${menuId}${force ? "?force=1" : ""}`,
        ),
      () => mockApi.deleteMenu(id, menuId, force),
    );
  },

  listCategories(id: string, menuId?: string): Promise<Category[]> {
    return withFallback(
      () =>
        request(
          "GET",
          `/restaurants/${id}/categories${menuId ? `?menu_id=${menuId}` : ""}`,
        ),
      () => mockApi.listCategories(id, menuId),
    );
  },

  createCategory(
    id: string,
    input: { name: string; menu_id: string },
  ): Promise<Category> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/categories`, input),
      () => mockApi.createCategory(id, input),
    );
  },

  updateCategory(
    id: string,
    catId: string,
    patch: Partial<Category>,
  ): Promise<Category> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/categories/${catId}`, patch),
      () => mockApi.updateCategory(id, catId, patch),
    );
  },

  deleteCategory(id: string, catId: string): Promise<void> {
    return withFallback(
      () => request("DELETE", `/restaurants/${id}/categories/${catId}`),
      () => mockApi.deleteCategory(id, catId),
    );
  },

  listItems(id: string): Promise<MenuItem[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/items`),
      () => mockApi.listItems(id),
    );
  },

  createItem(id: string, input: Omit<MenuItem, "id">): Promise<MenuItem> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/items`, input),
      () => mockApi.createItem(id, input),
    );
  },

  updateItem(
    id: string,
    itemId: string,
    patch: Partial<MenuItem>,
  ): Promise<MenuItem> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/items/${itemId}`, patch),
      () => mockApi.updateItem(id, itemId, patch),
    );
  },

  deleteItem(id: string, itemId: string): Promise<void> {
    return withFallback(
      () => request("DELETE", `/restaurants/${id}/items/${itemId}`),
      () => mockApi.deleteItem(id, itemId),
    );
  },

  uploadImage(id: string, file: File): Promise<{ url: string }> {
    const form = new FormData();
    form.append("image", file);
    return withFallback(
      () => request("POST", `/restaurants/${id}/images`, form),
      () => mockApi.uploadImage(id, file),
    );
  },

  listTables(id: string): Promise<Table[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/tables`),
      () => mockApi.listTables(id),
    );
  },

  createTable(id: string, input: { label: string }): Promise<Table> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/tables`, input),
      () => mockApi.createTable(id, input),
    );
  },

  regenerateTableToken(id: string, tableId: string): Promise<Table> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/tables/${tableId}/regenerate`),
      () => mockApi.regenerateTableToken(id, tableId),
    );
  },

  qrUrl(id: string, tableId: string): string {
    return `${BASE}/restaurants/${id}/tables/${tableId}/qr`;
  },

  listStaff(id: string): Promise<StaffMember[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/staff`),
      () => mockApi.listStaff(id),
    );
  },

  inviteStaff(
    id: string,
    input: { email: string; role: Role },
  ): Promise<StaffMember> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/staff`, input),
      () => mockApi.inviteStaff(id, input),
    );
  },

  listAssistantMessages(id: string): Promise<AssistantMessage[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/assistant/messages?limit=50`),
      () => mockApi.listAssistantMessages(id),
    );
  },

  sendAssistantMessage(
    id: string,
    text: string,
    files: File[],
  ): Promise<AssistantMessage> {
    const form = new FormData();
    form.append("text", text);
    for (const f of files) form.append("files", f);
    return withFallback(
      () => request("POST", `/restaurants/${id}/assistant/messages`, form),
      () => mockApi.sendAssistantMessage(id, text, files),
    );
  },

  applyAssistantActions(
    id: string,
    msgId: string,
    indexes?: number[],
  ): Promise<{ results: AssistantApplyResult[] }> {
    return withFallback(
      () =>
        request(
          "POST",
          `/restaurants/${id}/assistant/messages/${msgId}/apply`,
          indexes ? { action_indexes: indexes } : {},
        ),
      () => mockApi.applyAssistantActions(id, msgId, indexes),
    );
  },

  discardAssistantActions(id: string, msgId: string): Promise<void> {
    return withFallback(
      () =>
        request(
          "POST",
          `/restaurants/${id}/assistant/messages/${msgId}/discard`,
        ),
      () => mockApi.discardAssistantActions(id, msgId),
    );
  },

  getSubscription(): Promise<Subscription> {
    return withFallback(
      () => request("GET", "/org/subscription"),
      () => mockApi.getSubscription(),
    );
  },

  setSubscription(plan: Plan): Promise<Subscription> {
    return withFallback(
      () => request("POST", "/org/subscription", { plan }),
      () => mockApi.setSubscription(plan),
    );
  },
};
