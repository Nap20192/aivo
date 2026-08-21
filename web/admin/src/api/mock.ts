// In-browser mock backend. Seeded with the Ember & Bone demo tenant,
// persisted to localStorage so demo edits survive reload.
import {
  demoCategories,
  demoItems,
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
  Category,
  Me,
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
  categories: Category[];
  items: MenuItem[];
  tables: Table[];
  staff: StaffMember[];
  subscription: Subscription;
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
    categories: demoCategories,
    items: demoItems,
    tables: demoTables,
    staff: demoStaff,
    subscription: demoSubscription,
  };
}

function load(): Db {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) return JSON.parse(raw) as Db;
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
    db.categories = [];
    db.items = [];
    db.tables = [];
    db.staff = [
      { id: uid("staff"), email: input.email, role: "owner", status: "active" },
    ];
    db.subscription = { plan: "free", status: "trialing", renews_at: "" };
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

  async listCategories(id: string): Promise<Category[]> {
    requireRestaurant(id);
    return delay([...db.categories].sort((a, b) => a.position - b.position));
  },

  async createCategory(id: string, input: { name: string }): Promise<Category> {
    requireRestaurant(id);
    const cat: Category = {
      id: uid("cat"),
      name: input.name,
      position: db.categories.length,
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
    db.categories
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

export type MockApi = typeof mockApi;
