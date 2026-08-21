// Ember & Bone demo tenant — mirrors docs/prototypes/aivo-menu-prototype.dc.html
// and the backend seed (cmd/aivo-seed).

import type { OptionGroup, TableSession } from "./types";

const doneness: OptionGroup = {
  id: "doneness",
  name: "Doneness",
  select: "single",
  options: [
    { id: "rare", label: "Rare", price_delta_cents: 0 },
    { id: "medium-rare", label: "Medium rare", price_delta_cents: 0 },
    { id: "medium", label: "Medium", price_delta_cents: 0 },
    { id: "well-done", label: "Well done", price_delta_cents: 0 },
  ],
};

const sauces: OptionGroup = {
  id: "sauces",
  name: "Add-ons",
  select: "multi",
  options: [
    { id: "bearnaise", label: "Béarnaise", price_delta_cents: 300 },
    { id: "marrow-butter", label: "Bone marrow butter", price_delta_cents: 400 },
    { id: "peppercorn", label: "Peppercorn sauce", price_delta_cents: 300 },
  ],
};

export const demoSession: TableSession = {
  restaurant: {
    name: "Ember & Bone",
    slug: "ember-and-bone",
    tagline:
      "Live-fire grill, dry-aged beef, one wood oven. Everything comes out when it's ready — we'll pace it for you.",
    hours: [
      { label: "Kitchen", open: "17:00", close: "22:30" },
      { label: "Bar", open: "17:00", close: "00:00" },
    ],
    address: "14 Rue des Bouchers",
    map_url: "https://maps.google.com/?q=14+Rue+des+Bouchers",
    phone: "02 512 33 74",
    instagram: "emberandbone",
  },
  table: { id: "table-12", label: "Table 12" },
  theme: { brand_name: "Ember & Bone", accent: "Blood red", bold: false },
  open_requests: [],
  menus: [
    {
      id: "menu-dinner",
      slug: "dinner",
      name: "Dinner",
      is_default: true,
      categories: [
    {
      id: "starters",
      name: "Starters",
      items: [
        {
          id: "marrow",
          name: "Bone marrow, sourdough",
          description: "Roasted, parsley and caper salt, grilled sourdough.",
          price_cents: 1400,
          allergens: ["gluten"],
          option_groups: [],
          available: true,
        },
        {
          id: "tartare",
          name: "Beef tartare, cured yolk",
          description: "Hand-cut sirloin, mustard, shallot, rye crisps.",
          price_cents: 1800,
          allergens: ["egg", "mustard"],
          option_groups: [],
          available: true,
        },
        {
          id: "leeks",
          name: "Charred leeks, olive oil",
          description: "Vadouvan, hazelnut, sheep's curd.",
          price_cents: 1200,
          allergens: [],
          option_groups: [],
          available: false,
          sold_out_at: "20:04",
        },
        {
          id: "flatbread",
          name: "Grilled flatbread, beef fat",
          description: "Wood oven, smoked salt, rosemary.",
          price_cents: 800,
          allergens: [],
          option_groups: [],
          available: true,
        },
      ],
    },
    {
      id: "grill",
      name: "From the grill",
      items: [
        {
          id: "ribeye",
          name: "Dry-aged ribeye",
          description:
            "45 days on the bone, over vine wood. Rested 10 minutes, carved to order.",
          price_cents: 4600,
          allergens: ["dairy", "mustard"],
          option_groups: [
            {
              id: "size",
              name: "Size",
              select: "single",
              options: [
                { id: "300g", label: "300 g", price_delta_cents: 0 },
                { id: "400g", label: "400 g · centre cut", price_delta_cents: 1200 },
                { id: "600g", label: "600 g · to share", price_delta_cents: 2600 },
              ],
            },
            doneness,
            sauces,
          ],
          available: true,
        },
        {
          id: "bavette",
          name: "Bavette, chimichurri",
          description: "Onglet's louder cousin, cooked over embers.",
          price_cents: 3400,
          allergens: [],
          option_groups: [doneness, sauces],
          available: true,
        },
        {
          id: "lamb",
          name: "Lamb shoulder, 6 hours",
          description:
            "Slow-roasted over embers, harissa, yoghurt, flatbread. Enough for two.",
          price_cents: 4600,
          allergens: [],
          option_groups: [],
          available: false,
          sold_out_at: "20:04",
        },
        {
          id: "chicken",
          name: "Half chicken, brined",
          description: "Lemon, thyme, pan drippings.",
          price_cents: 2800,
          allergens: [],
          option_groups: [],
          available: true,
        },
      ],
    },
    {
      id: "sides",
      name: "Sides",
      items: [
        {
          id: "chips",
          name: "Triple-cooked chips",
          description: "Beef fat, rosemary salt.",
          price_cents: 900,
          allergens: [],
          option_groups: [],
          available: true,
        },
        {
          id: "hispi",
          name: "Hispi cabbage",
          description: "Charred, anchovy cream.",
          price_cents: 800,
          allergens: ["fish", "dairy"],
          option_groups: [],
          available: true,
        },
        {
          id: "salad",
          name: "Green salad",
          description: "Soft herbs, mustard dressing.",
          price_cents: 700,
          allergens: ["mustard"],
          option_groups: [],
          available: true,
        },
      ],
    },
    {
      id: "wine",
      name: "Wine",
      items: [
        {
          id: "malbec",
          name: "Malbec, glass",
          description: "Mendoza. Dark fruit, holds up to the grill.",
          price_cents: 1400,
          allergens: [],
          option_groups: [],
          available: true,
        },
        {
          id: "gamay",
          name: "Gamay, Beaujolais",
          description: "Chilled, light, made for chips.",
          price_cents: 1200,
          allergens: [],
          option_groups: [],
          available: true,
        },
        {
          id: "ribolla",
          name: "Ribolla, 2021",
          description: "Orange. Skin contact, apricot, grip.",
          price_cents: 1300,
          allergens: [],
          option_groups: [],
          available: true,
        },
      ],
    },
      ],
    },
    {
      id: "menu-bar",
      slug: "bar",
      name: "Bar",
      is_default: false,
      categories: [
        {
          id: "cocktails",
          name: "Cocktails",
          items: [
            {
              id: "negroni",
              name: "Negroni, barrel-aged",
              description: "Six weeks in oak. Stirred, orange peel.",
              price_cents: 1200,
              allergens: [],
              option_groups: [],
              available: true,
            },
            {
              id: "boulevardier",
              name: "Boulevardier",
              description: "Rye, Campari, sweet vermouth.",
              price_cents: 1300,
              allergens: [],
              option_groups: [],
              available: true,
            },
            {
              id: "amaro",
              name: "Amaro, after dinner",
              description: "Rotating pour. Ask what's open.",
              price_cents: 900,
              allergens: [],
              option_groups: [],
              available: false,
              sold_out_at: "21:15",
            },
          ],
        },
        {
          id: "beer",
          name: "Beer & cider",
          items: [
            {
              id: "saison",
              name: "Saison, 33 cl",
              description: "Farmhouse, dry, local.",
              price_cents: 700,
              allergens: ["gluten"],
              option_groups: [],
              available: true,
            },
            {
              id: "cider",
              name: "Basque cider",
              description: "Still, sharp, poured from height.",
              price_cents: 800,
              allergens: [],
              option_groups: [],
              available: true,
            },
          ],
        },
      ],
    },
  ],
};
