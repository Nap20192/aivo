// In-browser mock backend. Seeded with the Ember & Bone demo tenant,
// persisted to localStorage so demo edits survive reload.
import {
  demoCategories,
  demoGuests,
  demoItems,
  demoMenus,
  demoOrg,
  demoPassword,
  demoRestaurant,
  demoStaff,
  demoSubscription,
  demoTables,
  demoTheme,
  demoUser,
} from "./fixtures";
import type {
  AssistantAction,
  AssistantApplyResult,
  AssistantMessage,
  Category,
  GuestDetail,
  GuestSummary,
  Me,
  Menu,
  MenuItem,
  Org,
  Plan,
  Restaurant,
  Role,
  StaffMember,
  Subscription,
  Table,
  Theme,
  User,
} from "./types";
import { ApiError } from "./error";

interface Db {
  org: Org;
  user: User;
  password: string;
  loggedIn: boolean;
  restaurant: Restaurant;
  theme: Theme;
  menus: Menu[];
  categories: Category[];
  items: MenuItem[];
  tables: Table[];
  staff: StaffMember[];
  subscription: Subscription;
  assistant: AssistantMessage[];
  guests: GuestDetail[];
}

function seedGuests(): GuestDetail[] {
  // Deep copy with totals derived from order history.
  return demoGuests.map((g) => ({
    ...g,
    tags: [...g.tags],
    orders: g.orders.map((o) => ({ ...o, lines: o.lines.map((l) => ({ ...l })) })),
    visits: g.orders.length,
    total_spent_cents: g.orders.reduce((t, o) => t + o.total_cents, 0),
  }));
}

const KEY = "aivo-admin-mock";

function seed(): Db {
  return {
    org: demoOrg,
    user: demoUser,
    password: demoPassword,
    loggedIn: false,
    restaurant: demoRestaurant,
    theme: demoTheme,
    menus: demoMenus,
    categories: demoCategories,
    items: demoItems,
    tables: demoTables,
    staff: demoStaff,
    subscription: demoSubscription,
    assistant: [],
    guests: seedGuests(),
  };
}

function load(): Db {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) {
      const db = JSON.parse(raw) as Db;
      // Mirror of the backend migration: stored state from before multi-menu
      // gets a default menu and its categories moved into it.
      if (!db.menus) {
        db.menus = [
          { id: "menu-default", slug: "menu", name: "Menu", position: 0, is_default: true },
        ];
        db.categories.forEach((c) => (c.menu_id = c.menu_id ?? "menu-default"));
      }
      db.assistant = db.assistant ?? [];
      db.guests = db.guests ?? seedGuests();
      return db;
    }
  } catch {
    // corrupted storage: reseed
  }
  return seed();
}

let db = load();

function save() {
  localStorage.setItem(KEY, JSON.stringify(db));
}

function uid(prefix: string): string {
  return prefix + "-" + Math.random().toString(36).slice(2, 10);
}

function token(): string {
  return Math.random().toString(36).slice(2, 8);
}

async function delay<T>(v: T): Promise<T> {
  await new Promise((r) => setTimeout(r, 120));
  return v;
}

function requireAuth() {
  if (!db.loggedIn) throw new ApiError("unauthorized", "Not signed in.", 401);
}

function requireRestaurant(id: string): Restaurant {
  requireAuth();
  if (db.restaurant.id !== id)
    throw new ApiError("not_found", "Restaurant not found.", 404);
  return db.restaurant;
}

function me(): Me {
  return { user: db.user, org: db.org, restaurants: [db.restaurant] };
}

export const mockApi = {
  async register(input: {
    org_name: string;
    restaurant_name: string;
    email: string;
    password: string;
  }): Promise<Me> {
    const slug = input.restaurant_name
      .toLowerCase()
      .replace(/&/g, "and")
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "");
    db = seed();
    db.org = { id: uid("org"), name: input.org_name };
    db.user = { id: uid("user"), email: input.email, role: "owner" };
    db.password = input.password;
    db.restaurant = {
      ...db.restaurant,
      id: uid("rest"),
      org_id: db.org.id,
      name: input.restaurant_name,
      slug,
    };
    db.theme = { ...db.theme, brand_name: input.restaurant_name, design_md: "" };
    // Provisioning auto-creates the default menu.
    db.menus = [
      { id: uid("menu"), slug: "menu", name: "Menu", position: 0, is_default: true },
    ];
    db.categories = [];
    db.items = [];
    db.tables = [];
    db.staff = [
      { id: uid("staff"), email: input.email, role: "owner", status: "active" },
    ];
    db.subscription = { plan: "free", status: "trialing", renews_at: "" };
    db.assistant = [];
    db.guests = [];
    db.loggedIn = true;
    save();
    return delay(me());
  },

  async login(input: { email: string; password: string }): Promise<Me> {
    if (input.email !== db.user.email || input.password !== db.password)
      throw new ApiError("invalid_credentials", "Wrong email or password.", 401);
    db.loggedIn = true;
    save();
    return delay(me());
  },

  async logout(): Promise<void> {
    db.loggedIn = false;
    save();
    return delay(undefined);
  },

  async me(): Promise<Me> {
    requireAuth();
    return delay(me());
  },

  async getRestaurant(id: string): Promise<Restaurant> {
    return delay({ ...requireRestaurant(id) });
  },

  async patchRestaurant(
    id: string,
    patch: Partial<Restaurant>,
  ): Promise<Restaurant> {
    requireRestaurant(id);
    db.restaurant = { ...db.restaurant, ...patch, id };
    save();
    return delay({ ...db.restaurant });
  },

  async getTheme(id: string): Promise<Theme> {
    requireRestaurant(id);
    return delay({ ...db.theme });
  },

  async putTheme(id: string, theme: Theme): Promise<Theme> {
    requireRestaurant(id);
    db.theme = { ...theme };
    save();
    return delay({ ...db.theme });
  },

  // Canned proposal so the AI flow is demoable without the backend.
  async generateTheme(
    id: string,
  ): Promise<{ proposal: Theme; based_on: string }> {
    requireRestaurant(id);
    if (!db.theme.design_md.trim())
      throw new ApiError(
        "empty_brief",
        "The design brief is empty — write or paste one first.",
        409,
      );
    await new Promise((r) => setTimeout(r, 1400));
    return {
      proposal: {
        ...db.theme,
        accent: "Wine",
        bold: true,
        css_vars: { ...db.theme.css_vars, "--radius-md": "6px" },
      },
      based_on: "design_md",
    };
  },

  async listMenus(id: string): Promise<Menu[]> {
    requireRestaurant(id);
    return delay(
      [...db.menus]
        .sort((a, b) => a.position - b.position)
        .map((m) => ({ ...m })),
    );
  },

  async createMenu(
    id: string,
    input: { name: string; slug: string },
  ): Promise<Menu> {
    requireRestaurant(id);
    if (db.menus.some((m) => m.slug === input.slug))
      throw new ApiError("conflict", "A menu with that slug already exists.", 422);
    const menu: Menu = {
      id: uid("menu"),
      slug: input.slug,
      name: input.name,
      position: db.menus.length,
      is_default: false,
    };
    db.menus.push(menu);
    save();
    return delay({ ...menu });
  },

  async updateMenu(
    id: string,
    menuId: string,
    patch: Partial<Menu>,
  ): Promise<Menu> {
    requireRestaurant(id);
    const menu = db.menus.find((m) => m.id === menuId);
    if (!menu) throw new ApiError("not_found", "Menu not found.", 404);
    if (
      patch.slug !== undefined &&
      db.menus.some((m) => m.id !== menuId && m.slug === patch.slug)
    )
      throw new ApiError("conflict", "A menu with that slug already exists.", 422);
    if (patch.is_default === false && menu.is_default)
      throw new ApiError(
        "invalid",
        "One menu must be the default — set another menu as default instead.",
        422,
      );
    if (patch.is_default === true)
      db.menus.forEach((m) => (m.is_default = m.id === menuId));
    Object.assign(menu, patch, { id: menuId });
    save();
    return delay({ ...menu });
  },

  async deleteMenu(id: string, menuId: string, force = false): Promise<void> {
    requireRestaurant(id);
    const menu = db.menus.find((m) => m.id === menuId);
    if (!menu) throw new ApiError("not_found", "Menu not found.", 404);
    if (menu.is_default)
      throw new ApiError("invalid", "The default menu can't be deleted.", 422);
    if (db.menus.length === 1)
      throw new ApiError("invalid", "The last menu can't be deleted.", 422);
    const cats = db.categories.filter((c) => c.menu_id === menuId);
    if (cats.length > 0 && !force)
      throw new ApiError(
        "menu_not_empty",
        "The menu still has categories — delete with its contents to proceed.",
        422,
      );
    const catIds = new Set(cats.map((c) => c.id));
    db.items = db.items.filter((i) => !catIds.has(i.category_id));
    db.categories = db.categories.filter((c) => c.menu_id !== menuId);
    db.menus = db.menus.filter((m) => m.id !== menuId);
    db.menus
      .sort((a, b) => a.position - b.position)
      .forEach((m, i) => (m.position = i));
    save();
    return delay(undefined);
  },

  async listCategories(id: string, menuId?: string): Promise<Category[]> {
    requireRestaurant(id);
    return delay(
      db.categories
        .filter((c) => !menuId || c.menu_id === menuId)
        .sort((a, b) => a.position - b.position)
        .map((c) => ({ ...c })),
    );
  },

  async createCategory(
    id: string,
    input: { name: string; menu_id: string },
  ): Promise<Category> {
    requireRestaurant(id);
    if (!db.menus.some((m) => m.id === input.menu_id))
      throw new ApiError("not_found", "Menu not found.", 404);
    const cat: Category = {
      id: uid("cat"),
      menu_id: input.menu_id,
      name: input.name,
      position: db.categories.filter((c) => c.menu_id === input.menu_id).length,
    };
    db.categories.push(cat);
    save();
    return delay({ ...cat });
  },

  async updateCategory(
    id: string,
    catId: string,
    patch: Partial<Category>,
  ): Promise<Category> {
    requireRestaurant(id);
    const cat = db.categories.find((c) => c.id === catId);
    if (!cat) throw new ApiError("not_found", "Category not found.", 404);
    Object.assign(cat, patch, { id: catId });
    save();
    return delay({ ...cat });
  },

  async deleteCategory(id: string, catId: string): Promise<void> {
    requireRestaurant(id);
    db.categories = db.categories.filter((c) => c.id !== catId);
    db.items = db.items.filter((i) => i.category_id !== catId);
    for (const m of db.menus)
      db.categories
        .filter((c) => c.menu_id === m.id)
        .sort((a, b) => a.position - b.position)
        .forEach((c, i) => (c.position = i));
    save();
    return delay(undefined);
  },

  async listItems(id: string): Promise<MenuItem[]> {
    requireRestaurant(id);
    return delay(db.items.map((i) => ({ ...i })));
  },

  async createItem(
    id: string,
    input: Omit<MenuItem, "id">,
  ): Promise<MenuItem> {
    requireRestaurant(id);
    const item: MenuItem = { ...input, id: uid("item") };
    db.items.push(item);
    save();
    return delay({ ...item });
  },

  async updateItem(
    id: string,
    itemId: string,
    patch: Partial<MenuItem>,
  ): Promise<MenuItem> {
    requireRestaurant(id);
    const item = db.items.find((i) => i.id === itemId);
    if (!item) throw new ApiError("not_found", "Item not found.", 404);
    Object.assign(item, patch, { id: itemId });
    save();
    return delay({ ...item });
  },

  async deleteItem(id: string, itemId: string): Promise<void> {
    requireRestaurant(id);
    db.items = db.items.filter((i) => i.id !== itemId);
    save();
    return delay(undefined);
  },

  async uploadImage(id: string, file: File): Promise<{ url: string }> {
    requireRestaurant(id);
    const url = await new Promise<string>((resolve, reject) => {
      const r = new FileReader();
      r.onload = () => resolve(r.result as string);
      r.onerror = () => reject(new ApiError("upload_failed", "Could not read file.", 422));
      r.readAsDataURL(file);
    });
    return delay({ url });
  },

  async listTables(id: string): Promise<Table[]> {
    requireRestaurant(id);
    return delay(db.tables.map((t) => ({ ...t })));
  },

  async createTable(id: string, input: { label: string }): Promise<Table> {
    requireRestaurant(id);
    const t: Table = { id: uid("table"), label: input.label, token: token() };
    db.tables.push(t);
    save();
    return delay({ ...t });
  },

  async regenerateTableToken(id: string, tableId: string): Promise<Table> {
    requireRestaurant(id);
    const t = db.tables.find((x) => x.id === tableId);
    if (!t) throw new ApiError("not_found", "Table not found.", 404);
    t.token = token();
    save();
    return delay({ ...t });
  },

  // Real endpoint returns an image; mock has none. Pages detect mock mode
  // and render a client-side placeholder instead.
  qrUrl(id: string, tableId: string): string {
    return `/api/v1/restaurants/${id}/tables/${tableId}/qr`;
  },

  async listStaff(id: string): Promise<StaffMember[]> {
    requireRestaurant(id);
    return delay(db.staff.map((s) => ({ ...s })));
  },

  async inviteStaff(
    id: string,
    input: { email: string; role: Role },
  ): Promise<StaffMember> {
    requireRestaurant(id);
    if (db.staff.some((s) => s.email === input.email))
      throw new ApiError("conflict", "That email is already on the team.", 422);
    const s: StaffMember = {
      id: uid("staff"),
      email: input.email,
      role: input.role,
      status: "invited",
    };
    db.staff.push(s);
    save();
    return delay({ ...s });
  },

  async listAssistantMessages(id: string): Promise<AssistantMessage[]> {
    requireRestaurant(id);
    return delay(db.assistant.map((m) => ({ ...m })));
  },

  async sendAssistantMessage(
    id: string,
    text: string,
    files: File[],
  ): Promise<AssistantMessage> {
    requireRestaurant(id);
    const attachments = await Promise.all(
      files.map(
        (f) =>
          new Promise<{ name: string; url: string; mime: string }>(
            (resolve, reject) => {
              const r = new FileReader();
              r.onload = () =>
                resolve({ name: f.name, url: r.result as string, mime: f.type });
              r.onerror = () =>
                reject(new ApiError("upload_failed", "Could not read file.", 422));
              r.readAsDataURL(f);
            },
          ),
      ),
    );
    const userMsg: AssistantMessage = {
      id: uid("amsg"),
      role: "user",
      text,
      attachments,
      actions: [],
      action_status: null,
      created_at: new Date().toISOString(),
    };
    db.assistant.push(userMsg);

    // Canned proposal so the flow demos offline: one menu action, one theme
    // action, grounded in the current state.
    const defaultMenu = db.menus.find((m) => m.is_default) ?? db.menus[0];
    const firstCat = db.categories
      .filter((c) => c.menu_id === defaultMenu?.id)
      .sort((a, b) => a.position - b.position)[0];
    const actions: AssistantAction[] = [];
    if (firstCat)
      actions.push({
        type: "create_item",
        category_id: firstCat.id,
        name: "Caesar salad",
        description: "Gem lettuce, anchovy dressing, aged parmesan, croutons.",
        price_cents: 1200,
        allergens: ["fish", "milk", "gluten", "eggs"],
      });
    actions.push({ type: "update_theme", accent: "Wine", bold: db.theme.bold });
    const reply: AssistantMessage = {
      id: uid("amsg"),
      role: "assistant",
      text: `Here's what I'd do. I read your message${attachments.length ? ` and ${attachments.length} attachment${attachments.length === 1 ? "" : "s"}` : ""} against the current menu (${db.menus.length} menu${db.menus.length === 1 ? "" : "s"}, ${db.items.length} items). Two proposals below — nothing is applied until you confirm. Note: this is the demo assistant; the real one answers your actual request.`,
      attachments: [],
      actions,
      action_status: null,
      created_at: new Date().toISOString(),
    };
    db.assistant.push(reply);
    save();
    await new Promise((r) => setTimeout(r, 1600));
    return { ...reply };
  },

  async applyAssistantActions(
    id: string,
    msgId: string,
    indexes?: number[],
  ): Promise<{ results: AssistantApplyResult[] }> {
    requireRestaurant(id);
    const msg = db.assistant.find((m) => m.id === msgId);
    if (!msg) throw new ApiError("not_found", "Message not found.", 404);
    if (msg.action_status)
      throw new ApiError("conflict", "This proposal was already resolved.", 409);
    const picked =
      indexes ?? msg.actions.map((_, i) => i);
    const results: AssistantApplyResult[] = picked.map((i) => {
      const a = msg.actions[i];
      if (!a) return { index: i, ok: false, detail: "No such action." };
      try {
        return { index: i, ok: true, detail: execAction(a) };
      } catch (e) {
        return {
          index: i,
          ok: false,
          detail: e instanceof Error ? e.message : "Failed.",
        };
      }
    });
    msg.action_status = "applied";
    save();
    return delay({ results });
  },

  async discardAssistantActions(id: string, msgId: string): Promise<void> {
    requireRestaurant(id);
    const msg = db.assistant.find((m) => m.id === msgId);
    if (!msg) throw new ApiError("not_found", "Message not found.", 404);
    msg.action_status = "discarded";
    save();
    return delay(undefined);
  },

  async listGuests(id: string, query?: string): Promise<GuestSummary[]> {
    requireRestaurant(id);
    const q = (query ?? "").trim().toLowerCase();
    return delay(
      db.guests
        .filter(
          (g) =>
            !q ||
            g.customer.name.toLowerCase().includes(q) ||
            g.customer.email.toLowerCase().includes(q) ||
            g.tags.some((t) => t.toLowerCase().includes(q)),
        )
        .sort((a, b) => b.last_seen.localeCompare(a.last_seen))
        .map((g) => ({
          customer: { ...g.customer },
          visits: g.visits,
          total_spent_cents: g.total_spent_cents,
          last_seen: g.last_seen,
          tags: [...g.tags],
        })),
    );
  },

  async getGuest(id: string, customerId: string): Promise<GuestDetail> {
    requireRestaurant(id);
    const g = db.guests.find((x) => x.customer.id === customerId);
    if (!g) throw new ApiError("not_found", "Guest not found.", 404);
    return delay(JSON.parse(JSON.stringify(g)) as GuestDetail);
  },

  async patchGuest(
    id: string,
    customerId: string,
    patch: { notes?: string; tags?: string[] },
  ): Promise<GuestDetail> {
    requireRestaurant(id);
    const g = db.guests.find((x) => x.customer.id === customerId);
    if (!g) throw new ApiError("not_found", "Guest not found.", 404);
    if (patch.notes !== undefined) g.notes = patch.notes;
    if (patch.tags !== undefined) g.tags = [...patch.tags];
    save();
    return delay(JSON.parse(JSON.stringify(g)) as GuestDetail);
  },

  async getSubscription(): Promise<Subscription> {
    requireAuth();
    return delay({ ...db.subscription });
  },

  async setSubscription(plan: Plan): Promise<Subscription> {
    requireAuth();
    db.subscription = {
      plan,
      status: "active",
      renews_at: db.subscription.renews_at || "2026-09-14",
    };
    save();
    return delay({ ...db.subscription });
  },
};

// Executes one allowlisted assistant action against the mock db.
// Returns a human-readable result line; throws on validation failure.
function execAction(a: AssistantAction): string {
  switch (a.type) {
    case "create_category": {
      if (!db.menus.some((m) => m.id === a.menu_id))
        throw new Error("Menu not found.");
      db.categories.push({
        id: uid("cat"),
        menu_id: a.menu_id,
        name: a.name,
        position: db.categories.filter((c) => c.menu_id === a.menu_id).length,
      });
      return `Created category "${a.name}".`;
    }
    case "rename_category": {
      const c = db.categories.find((x) => x.id === a.id);
      if (!c) throw new Error("Category not found.");
      c.name = a.name;
      return `Renamed category to "${a.name}".`;
    }
    case "delete_category": {
      if (!db.categories.some((x) => x.id === a.id))
        throw new Error("Category not found.");
      db.categories = db.categories.filter((x) => x.id !== a.id);
      db.items = db.items.filter((i) => i.category_id !== a.id);
      return "Deleted category and its items.";
    }
    case "create_item": {
      if (!db.categories.some((c) => c.id === a.category_id))
        throw new Error("Category not found.");
      if (!Number.isInteger(a.price_cents) || a.price_cents < 0)
        throw new Error("Invalid price.");
      db.items.push({
        id: uid("item"),
        category_id: a.category_id,
        name: a.name,
        description: a.description ?? "",
        price_cents: a.price_cents,
        image_url: a.image_url ?? "",
        allergens: a.allergens ?? [],
        option_groups: [],
        available: true,
      });
      return `Created item "${a.name}".`;
    }
    case "update_item": {
      const item = db.items.find((i) => i.id === a.id);
      if (!item) throw new Error("Item not found.");
      const { type: _, id: __, ...patch } = a;
      if (
        patch.price_cents !== undefined &&
        (!Number.isInteger(patch.price_cents) || patch.price_cents < 0)
      )
        throw new Error("Invalid price.");
      Object.assign(item, patch);
      return `Updated item "${item.name}".`;
    }
    case "delete_item": {
      const item = db.items.find((i) => i.id === a.id);
      if (!item) throw new Error("Item not found.");
      db.items = db.items.filter((i) => i.id !== a.id);
      return `Deleted item "${item.name}".`;
    }
    case "set_item_available": {
      const item = db.items.find((i) => i.id === a.id);
      if (!item) throw new Error("Item not found.");
      item.available = a.available;
      return `${item.name}: ${a.available ? "back on the menu" : "86'd"}.`;
    }
    case "update_theme": {
      const { type: _, ...patch } = a;
      db.theme = { ...db.theme, ...patch };
      return "Theme updated.";
    }
    case "create_menu": {
      if (db.menus.some((m) => m.slug === a.slug))
        throw new Error("A menu with that slug already exists.");
      db.menus.push({
        id: uid("menu"),
        slug: a.slug,
        name: a.name,
        position: db.menus.length,
        is_default: false,
      });
      return `Created menu "${a.name}".`;
    }
  }
}

export type MockApi = typeof mockApi;
