// Ember & Bone demo tenant — mirrors docs/prototypes/aivo-menu-prototype.dc.html.
import type {
  Category,
  MenuItem,
  Org,
  Restaurant,
  StaffMember,
  Subscription,
  Table,
  Theme,
  User,
} from "./types";

export const demoOrg: Org = { id: "org-ember", name: "Ember & Bone Group" };

export const demoUser: User = {
  id: "user-owner",
  email: "owner@emberandbone.example",
  role: "owner",
};

export const demoPassword = "firegrill";

export const demoRestaurant: Restaurant = {
  id: "rest-ember",
  org_id: "org-ember",
  slug: "ember-and-bone",
  name: "Ember & Bone",
  hours: [
    { label: "Kitchen", open: "17:00", close: "22:30" },
    { label: "Bar", open: "17:00", close: "00:00" },
  ],
  address: "14 Rue des Bouchers",
  phone: "02 512 33 74",
  instagram: "@emberandbone",
  custom_domain: "",
};

export const demoTheme: Theme = {
  brand_name: "Ember & Bone",
  accent: "Blood red",
  bold: false,
  banner_url: "",
  css_vars: {},
  design_md: `# Ember & Bone — design brief

Live-fire grill, dry-aged beef, one wood oven.

## Voice
Confident, warm, unhurried. Sentence case everywhere. No exclamation marks.

## Palette
Warm paper surfaces, blood red accent. The menu should feel like a
letterpressed card, not an app.

## Type
Newsreader for dish names on detail screens, Hanken Grotesk for UI,
JetBrains Mono for every price.`,
};

export const demoCategories: Category[] = [
  { id: "cat-starters", name: "Starters", position: 0 },
  { id: "cat-grill", name: "From the grill", position: 1 },
  { id: "cat-sides", name: "Sides", position: 2 },
  { id: "cat-wine", name: "Wine", position: 3 },
];

const doneness = {
  id: "grp-doneness",
  name: "Doneness",
  type: "single" as const,
  choices: [
    { id: "ch-rare", name: "Rare", price_delta_cents: 0 },
    { id: "ch-mr", name: "Medium rare", price_delta_cents: 0 },
    { id: "ch-med", name: "Medium", price_delta_cents: 0 },
    { id: "ch-well", name: "Well done", price_delta_cents: 0 },
  ],
};

const sauces = {
  id: "grp-sauces",
  name: "Sauces",
  type: "multi" as const,
  choices: [
    { id: "ch-bear", name: "Béarnaise", price_delta_cents: 300 },
    { id: "ch-marrow", name: "Bone marrow butter", price_delta_cents: 400 },
    { id: "ch-pepper", name: "Peppercorn sauce", price_delta_cents: 300 },
  ],
};

export const demoItems: MenuItem[] = [
  {
    id: "item-marrow",
    category_id: "cat-starters",
    name: "Bone marrow, sourdough",
    description: "Roasted, parsley and caper salt, grilled sourdough.",
    price_cents: 1400,
    image_url: "",
    allergens: ["gluten"],
    option_groups: [],
    available: true,
  },
  {
    id: "item-tartare",
    category_id: "cat-starters",
    name: "Beef tartare, cured yolk",
    description: "Hand-cut sirloin, mustard, shallot, rye crisps.",
    price_cents: 1800,
    image_url: "",
    allergens: ["eggs", "mustard"],
    option_groups: [],
    available: true,
  },
  {
    id: "item-leeks",
    category_id: "cat-starters",
    name: "Charred leeks, olive oil",
    description: "Vadouvan, hazelnut, sheep's curd.",
    price_cents: 1200,
    image_url: "",
    allergens: ["nuts", "milk"],
    option_groups: [],
    available: false,
  },
  {
    id: "item-flatbread",
    category_id: "cat-starters",
    name: "Grilled flatbread, beef fat",
    description: "Wood oven, smoked salt, rosemary.",
    price_cents: 800,
    image_url: "",
    allergens: ["gluten"],
    option_groups: [],
    available: true,
  },
  {
    id: "item-ribeye",
    category_id: "cat-grill",
    name: "Dry-aged ribeye",
    description:
      "45 days on the bone, over vine wood. Rested 10 minutes, carved to order.",
    price_cents: 4600,
    image_url: "",
    allergens: ["milk", "mustard"],
    option_groups: [
      {
        id: "grp-size",
        name: "Size",
        type: "single",
        choices: [
          { id: "ch-300", name: "300 g", price_delta_cents: 0 },
          { id: "ch-400", name: "400 g · centre cut", price_delta_cents: 1200 },
          { id: "ch-600", name: "600 g · to share", price_delta_cents: 2600 },
        ],
      },
      doneness,
      sauces,
    ],
    available: true,
  },
  {
    id: "item-bavette",
    category_id: "cat-grill",
    name: "Bavette, chimichurri",
    description: "Onglet's louder cousin, cooked over embers.",
    price_cents: 3400,
    image_url: "",
    allergens: [],
    option_groups: [doneness, sauces],
    available: true,
  },
  {
    id: "item-lamb",
    category_id: "cat-grill",
    name: "Lamb shoulder, 6 hours",
    description:
      "Slow-roasted over embers, harissa, yoghurt, flatbread. Enough for two.",
    price_cents: 4600,
    image_url: "",
    allergens: ["milk", "gluten"],
    option_groups: [],
    available: false,
  },
  {
    id: "item-chicken",
    category_id: "cat-grill",
    name: "Half chicken, brined",
    description: "Lemon, thyme, pan drippings.",
    price_cents: 2800,
    image_url: "",
    allergens: [],
    option_groups: [],
    available: true,
  },
  {
    id: "item-chips",
    category_id: "cat-sides",
    name: "Triple-cooked chips",
    description: "Beef fat, rosemary salt.",
    price_cents: 900,
    image_url: "",
    allergens: [],
    option_groups: [],
    available: true,
  },
  {
    id: "item-hispi",
    category_id: "cat-sides",
    name: "Hispi cabbage",
    description: "Charred, anchovy cream.",
    price_cents: 800,
    image_url: "",
    allergens: ["fish", "milk"],
    option_groups: [],
    available: true,
  },
  {
    id: "item-salad",
    category_id: "cat-sides",
    name: "Green salad",
    description: "Soft herbs, mustard dressing.",
    price_cents: 700,
    image_url: "",
    allergens: ["mustard"],
    option_groups: [],
    available: true,
  },
  {
    id: "item-malbec",
    category_id: "cat-wine",
    name: "Malbec, glass",
    description: "Mendoza. Dark fruit, holds up to the grill.",
    price_cents: 1400,
    image_url: "",
    allergens: ["sulphites"],
    option_groups: [],
    available: true,
  },
  {
    id: "item-gamay",
    category_id: "cat-wine",
    name: "Gamay, Beaujolais",
    description: "Chilled, light, made for chips.",
    price_cents: 1200,
    image_url: "",
    allergens: ["sulphites"],
    option_groups: [],
    available: true,
  },
  {
    id: "item-ribolla",
    category_id: "cat-wine",
    name: "Ribolla, 2021",
    description: "Orange. Skin contact, apricot, grip.",
    price_cents: 1300,
    image_url: "",
    allergens: ["sulphites"],
    option_groups: [],
    available: true,
  },
];

export const demoTables: Table[] = [
  { id: "table-1", label: "Table 1", token: "xY82kq" },
  { id: "table-2", label: "Table 2", token: "mN31pz" },
  { id: "table-3", label: "Table 3", token: "qR77vd" },
  { id: "table-12", label: "Table 12", token: "aK05wf" },
];

export const demoStaff: StaffMember[] = [
  {
    id: "staff-owner",
    email: "owner@emberandbone.example",
    role: "owner",
    status: "active",
  },
  {
    id: "staff-mira",
    email: "mira@emberandbone.example",
    role: "manager",
    status: "active",
  },
  {
    id: "staff-jules",
    email: "jules@emberandbone.example",
    role: "waiter",
    status: "invited",
  },
];

export const demoSubscription: Subscription = {
  plan: "pro",
  status: "active",
  renews_at: "2026-09-14",
};
