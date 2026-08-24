// Typed client for the /api/v1 contract (docs/PLATFORM.md).
// Mock mode: VITE_MOCK=1, or automatic fallback when the API is unreachable.
import {
  ApiError,
  request as sharedRequest,
} from "../../../design-system/shared/api";
import { mockApi } from "./mock";
import { ledgerMock } from "./ledgerMock";
import { inventoryMock } from "./inventoryMock";
import type {
  AcceptanceOverride,
  Account,
  AccountMapEntry,
  AssistantApplyResult,
  AssistantMessage,
  Category,
  CostCenter,
  DocLineInput,
  FoodCostReport,
  GoodsReceipt,
  GoodsReceiptInput,
  GuestDetail,
  GuestSummary,
  JournalDocument,
  JournalSummary,
  ManualJournalInput,
  Me,
  Menu,
  MenuItem,
  OnHand,
  Plan,
  Product,
  ProductInput,
  Restaurant,
  Role,
  ShiftAcceptance,
  ShiftRow,
  StaffMember,
  StockMove,
  Stocktake,
  StocktakePreview,
  Subscription,
  Supplier,
  Table,
  TechCard,
  TechCardInput,
  TechCardVersion,
  Theme,
  WriteOff,
  WriteOffInput,
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

// Shared fetch wrapper + the admin's mock-fallback trigger on network failure.
async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  try {
    return await sharedRequest<T>(BASE + path, {
      method,
      credentials: "same-origin",
      body:
        body instanceof FormData
          ? body
          : body !== undefined
            ? JSON.stringify(body)
            : undefined,
      // FormData must set its own multipart boundary.
      ...(body instanceof FormData ? { headers: undefined } : {}),
    });
  } catch (e) {
    if (e instanceof ApiError && e.status === 0) {
      // API unreachable: switch this session to mock mode.
      enableMock();
      throw new ApiError(0, "network", "API unreachable — switched to demo mode.");
    }
    throw e;
  }
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

  createMenu(id: string, input: { name: string; slug?: string }): Promise<Menu> {
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

  listGuests(id: string, query?: string): Promise<GuestSummary[]> {
    const qs = query ? `?query=${encodeURIComponent(query)}&limit=100` : "?limit=100";
    return withFallback(
      () => request("GET", `/restaurants/${id}/guests${qs}`),
      () => mockApi.listGuests(id, query),
    );
  },

  getGuest(id: string, customerId: string): Promise<GuestDetail> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/guests/${customerId}`),
      () => mockApi.getGuest(id, customerId),
    );
  },

  patchGuest(
    id: string,
    customerId: string,
    patch: { notes?: string; tags?: string[] },
  ): Promise<GuestDetail> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/guests/${customerId}`, patch),
      () => mockApi.patchGuest(id, customerId, patch),
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

  // ── Shift acceptance (manager+) ──
  listShifts(id: string, state: "closed" | "accepted"): Promise<ShiftRow[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/shifts?state=${state}`),
      () => ledgerMock.listShifts(state),
    );
  },

  getAcceptance(id: string, shiftId: string): Promise<ShiftAcceptance> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/shifts/${shiftId}/acceptance`),
      () => ledgerMock.getAcceptance(shiftId),
    );
  },

  patchAcceptance(id: string, shiftId: string, lines: AcceptanceOverride[]): Promise<ShiftAcceptance> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/shifts/${shiftId}/acceptance`, { lines }),
      () => ledgerMock.patchAcceptance(shiftId, lines),
    );
  },

  acceptShift(id: string, shiftId: string): Promise<{ shift: ShiftRow; document: JournalDocument }> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/shifts/${shiftId}/accept`),
      () => ledgerMock.acceptShift(shiftId),
    );
  },

  // ── Ledger back-office (manager+) ──
  listAccounts(id: string): Promise<Account[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/ledger/accounts`),
      () => ledgerMock.listAccounts(),
    );
  },

  // §4 has no cost-centers list endpoint; the override dropdown needs one. Flagged.
  listCostCenters(id: string): Promise<CostCenter[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/ledger/cost-centers`),
      () => ledgerMock.listCostCenters(),
    );
  },

  getAccountMap(id: string): Promise<AccountMapEntry[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/ledger/account-map`),
      () => ledgerMock.getAccountMap(),
    );
  },

  putAccountMap(id: string, map: { purpose: string; account_id: string }[]): Promise<AccountMapEntry[]> {
    return withFallback(
      () => request("PUT", `/restaurants/${id}/ledger/account-map`, { map }),
      () => ledgerMock.putAccountMap(map),
    );
  },

  listJournals(id: string, params: { from: string; to?: string; account?: string; source?: string }): Promise<JournalSummary[]> {
    const qs = new URLSearchParams({ from: params.from });
    if (params.to) qs.set("to", params.to);
    if (params.account) qs.set("account", params.account);
    if (params.source) qs.set("source", params.source);
    return withFallback(
      () => request("GET", `/restaurants/${id}/ledger/journals?${qs}`),
      () => ledgerMock.listJournals(),
    );
  },

  getJournal(id: string, docId: string): Promise<JournalDocument> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/ledger/journals/${docId}`),
      () => ledgerMock.getJournal(docId),
    );
  },

  postManualJournal(id: string, input: ManualJournalInput, post: boolean): Promise<{ document: JournalDocument }> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/ledger/journals${post ? "?post=1" : ""}`, input),
      () => ledgerMock.postManualJournal(input, post),
    );
  },

  cancelJournal(id: string, docId: string): Promise<{ reversal: JournalDocument; original: JournalDocument }> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/ledger/journals/${docId}/cancel`),
      () => ledgerMock.cancelJournal(docId),
    );
  },

  // ── Inventory: nomenclature (impl-contract-2 §10) ──
  listProducts(id: string): Promise<Product[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/products`),
      () => inventoryMock.listProducts(),
    );
  },
  getProduct(id: string, pid: string): Promise<Product> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/products/${pid}`),
      () => inventoryMock.getProduct(pid),
    );
  },
  createProduct(id: string, input: ProductInput): Promise<Product> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/products`, input),
      () => inventoryMock.createProduct(input),
    );
  },
  updateProduct(id: string, pid: string, patch: Partial<ProductInput> & { archived?: boolean }): Promise<Product> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/inventory/products/${pid}`, patch),
      () => inventoryMock.updateProduct(pid, patch),
    );
  },

  // ── Inventory: tech-cards ──
  listTechCards(id: string, pid: string): Promise<TechCardVersion[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/products/${pid}/tech-cards`),
      () => inventoryMock.listTechCards(pid),
    );
  },
  activeTechCard(id: string, pid: string, on: string): Promise<TechCard | null> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/products/${pid}/tech-cards/active?on=${on}`),
      () => inventoryMock.activeTechCard(pid, on),
    );
  },
  getTechCard(id: string, tcid: string): Promise<TechCard> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/tech-cards/${tcid}`),
      () => inventoryMock.getTechCard(tcid),
    );
  },
  createTechCard(id: string, pid: string, input: TechCardInput): Promise<TechCard> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/products/${pid}/tech-cards`, input),
      () => inventoryMock.createTechCard(pid, input),
    );
  },
  recostTechCard(id: string, tcid: string): Promise<TechCard> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/tech-cards/${tcid}/recost`),
      () => inventoryMock.recost(tcid),
    );
  },

  // ── Inventory: suppliers ──
  listSuppliers(id: string): Promise<Supplier[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/suppliers`),
      () => inventoryMock.listSuppliers(),
    );
  },
  createSupplier(id: string, input: { name: string; contacts?: Record<string, string>; note?: string }): Promise<Supplier> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/suppliers`, input),
      () => inventoryMock.createSupplier(input),
    );
  },
  updateSupplier(id: string, sid: string, patch: Partial<Pick<Supplier, "name" | "contacts" | "archived">>): Promise<Supplier> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/inventory/suppliers/${sid}`, patch),
      () => inventoryMock.updateSupplier(sid, patch),
    );
  },

  // ── Inventory: goods receipts ──
  listReceipts(id: string, status?: string): Promise<GoodsReceipt[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/receipts${status ? `?status=${status}` : ""}`),
      () => inventoryMock.listReceipts(status),
    );
  },
  getReceipt(id: string, rid: string): Promise<GoodsReceipt> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/receipts/${rid}`),
      () => inventoryMock.getReceipt(rid),
    );
  },
  createReceipt(id: string, input: GoodsReceiptInput): Promise<GoodsReceipt> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/receipts`, input),
      () => inventoryMock.createReceipt(input),
    );
  },
  postReceipt(id: string, rid: string): Promise<GoodsReceipt> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/receipts/${rid}/post`),
      () => inventoryMock.postReceipt(rid),
    );
  },
  cancelReceipt(id: string, rid: string): Promise<GoodsReceipt> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/receipts/${rid}/cancel`),
      () => inventoryMock.cancelReceipt(rid),
    );
  },

  // ── Inventory: write-offs ──
  listWriteOffs(id: string, status?: string): Promise<WriteOff[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/write-offs${status ? `?status=${status}` : ""}`),
      () => inventoryMock.listWriteOffs(status),
    );
  },
  getWriteOff(id: string, wid: string): Promise<WriteOff> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/write-offs/${wid}`),
      () => inventoryMock.getWriteOff(wid),
    );
  },
  createWriteOff(id: string, input: WriteOffInput): Promise<WriteOff> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/write-offs`, input),
      () => inventoryMock.createWriteOff(input),
    );
  },
  postWriteOff(id: string, wid: string): Promise<WriteOff> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/write-offs/${wid}/post`),
      () => inventoryMock.postWriteOff(wid),
    );
  },
  cancelWriteOff(id: string, wid: string): Promise<WriteOff> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/write-offs/${wid}/cancel`),
      () => inventoryMock.cancelWriteOff(wid),
    );
  },

  // ── Inventory: stocktakes ──
  listStocktakes(id: string, status?: string): Promise<Stocktake[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/stocktakes${status ? `?status=${status}` : ""}`),
      () => inventoryMock.listStocktakes(status),
    );
  },
  getStocktake(id: string, sid: string): Promise<Stocktake> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/stocktakes/${sid}`),
      () => inventoryMock.getStocktake(sid),
    );
  },
  createStocktake(id: string, input: { note?: string }): Promise<Stocktake> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/stocktakes`, input),
      () => inventoryMock.createStocktake(input),
    );
  },
  patchStocktake(id: string, sid: string, lines: DocLineInput[]): Promise<Stocktake> {
    return withFallback(
      () => request("PATCH", `/restaurants/${id}/inventory/stocktakes/${sid}`, { lines }),
      () => inventoryMock.patchStocktake(sid, lines),
    );
  },
  dryRunStocktake(id: string, sid: string): Promise<StocktakePreview> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/stocktakes/${sid}/dry-run`),
      () => inventoryMock.dryRunStocktake(sid),
    );
  },
  postStocktake(id: string, sid: string): Promise<Stocktake> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/stocktakes/${sid}/post`),
      () => inventoryMock.postStocktake(sid),
    );
  },
  cancelStocktake(id: string, sid: string): Promise<Stocktake> {
    return withFallback(
      () => request("POST", `/restaurants/${id}/inventory/stocktakes/${sid}/cancel`),
      () => inventoryMock.cancelStocktake(sid),
    );
  },

  // ── Inventory: on-hand, moves, food cost ──
  onHand(id: string, lowStock = false): Promise<OnHand[]> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/on-hand${lowStock ? "?low_stock=1" : ""}`),
      () => inventoryMock.onHandList(lowStock),
    );
  },
  stockMoves(id: string, params: { from?: string; product?: string }): Promise<StockMove[]> {
    const qs = new URLSearchParams();
    if (params.from) qs.set("from", params.from);
    if (params.product) qs.set("product", params.product);
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/stock-moves?${qs}`),
      () => inventoryMock.stockMoveList(params),
    );
  },
  foodCost(id: string, from: string, to: string): Promise<FoodCostReport> {
    return withFallback(
      () => request("GET", `/restaurants/${id}/inventory/reports/food-cost?from=${from}&to=${to}`),
      () => inventoryMock.foodCost(from, to),
    );
  },
};
