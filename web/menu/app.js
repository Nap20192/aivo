// Diner-facing web menu. Plain vanilla JS, no build step, no framework —
// one page, ephemeral client-side Cart (never persisted, per CONTEXT.md).
(() => {
  "use strict";

  const ALLERGEN_LABELS = {
    cereals_gluten: "Gluten",
    crustaceans: "Crustaceans",
    eggs: "Eggs",
    fish: "Fish",
    peanuts: "Peanuts",
    soybeans: "Soy",
    milk: "Milk",
    nuts: "Nuts",
    celery: "Celery",
    mustard: "Mustard",
    sesame: "Sesame",
    sulphur_dioxide_sulphites: "Sulphites",
    lupin: "Lupin",
    molluscs: "Molluscs",
  };

  const SOCIAL_LABELS = {
    instagram: "Instagram",
    facebook: "Facebook",
    tiktok: "TikTok",
    twitter: "Twitter/X",
    x: "Twitter/X",
  };

  // --- Route: {slug}/t/{token} (see src/menu/CONTEXT.md "Table link") ----
  function parseRoute() {
    const m = location.pathname.match(/^\/([^/]+)\/t\/([^/]+)\/?$/);
    if (!m) return null;
    return { slug: decodeURIComponent(m[1]), token: decodeURIComponent(m[2]) };
  }

  function money(cents) {
    return "€" + (cents / 100).toFixed(2);
  }

  // --- API -----------------------------------------------------------------
  async function apiGet(path) {
    const res = await fetch(path, { credentials: "same-origin" });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw { status: res.status, message: body.error || "Something went wrong" };
    return body;
  }

  async function apiPost(path, payload) {
    const res = await fetch(path, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw { status: res.status, message: body.error || "Something went wrong" };
    return body;
  }

  // --- State -----------------------------------------------------------------
  const state = {
    route: null,
    restaurant: null,
    categories: [],
    items: [],
    itemsById: new Map(),
    // cart line key -> { itemId, name, unitPriceCents, qty, options: [{id,label,priceDeltaCents}] }
    cart: new Map(),
  };

  function lineKey(itemId, optionIds) {
    return itemId + "|" + [...optionIds].sort().join(",");
  }

  function lineUnitPrice(line) {
    return line.unitPriceCents + line.options.reduce((s, o) => s + o.priceDeltaCents, 0);
  }

  function cartTotalCents() {
    let total = 0;
    for (const line of state.cart.values()) total += lineUnitPrice(line) * line.qty;
    return total;
  }

  function cartCount() {
    let n = 0;
    for (const line of state.cart.values()) n += line.qty;
    return n;
  }

  // --- DOM refs ----------------------------------------------------------
  const el = {
    loadMessage: document.getElementById("load-message"),
    landing: document.getElementById("landing"),
    menu: document.getElementById("menu"),
    restaurantName: document.getElementById("restaurant-name"),
    tableLabel: document.getElementById("table-label"),
    bannerSlot: document.getElementById("banner-slot"),
    cartBar: document.getElementById("cart-bar"),
    cartCount: document.getElementById("cart-count"),
    cartTotal: document.getElementById("cart-total"),
    cartToggle: document.getElementById("cart-toggle"),
    cartPanel: document.getElementById("cart-panel"),
    cartClose: document.getElementById("cart-close"),
    cartLines: document.getElementById("cart-lines"),
    cartComment: document.getElementById("cart-comment"),
    cartPanelTotal: document.getElementById("cart-panel-total"),
    submitOrder: document.getElementById("submit-order"),
    toast: document.getElementById("toast"),
  };

  let toastTimer = null;
  function showToast(tone, title, message) {
    clearTimeout(toastTimer);
    el.toast.className = "toast tone-" + tone;
    el.toast.innerHTML =
      '<div><span class="toast-title"></span><span class="toast-message"></span></div>';
    el.toast.querySelector(".toast-title").textContent = title;
    el.toast.querySelector(".toast-message").textContent = message || "";
    el.toast.hidden = false;
    toastTimer = setTimeout(() => { el.toast.hidden = true; }, 4200);
  }

  function showFatalError(message) {
    el.loadMessage.textContent = message;
    el.loadMessage.classList.add("error");
    el.loadMessage.hidden = false;
    el.landing.hidden = true;
    el.menu.hidden = true;
  }

  // --- Landing rendering ---------------------------------------------------
  function renderLanding(blocks) {
    const sorted = [...blocks].sort((a, b) => a.position - b.position);
    el.landing.innerHTML = "";
    el.bannerSlot.innerHTML = "";

    for (const block of sorted) {
      const node = renderBlock(block);
      if (!node) continue;
      if (block.type === "banner") {
        el.bannerSlot.appendChild(node); // full-bleed, above the sticky topbar content flow
      } else {
        el.landing.appendChild(node);
      }
    }
    el.landing.hidden = false;
  }

  function renderBlock(block) {
    const d = block.data || {};
    switch (block.type) {
      case "banner": {
        const wrap = document.createElement("div");
        wrap.className = "block block-banner" + (d.image_url ? "" : " no-image");
        if (d.image_url) {
          const img = document.createElement("img");
          img.src = d.image_url;
          img.alt = d.title || "";
          wrap.appendChild(img);
        }
        if (d.title) {
          const t = document.createElement("div");
          t.className = "block-banner-title";
          t.textContent = d.title;
          wrap.appendChild(t);
        }
        return d.image_url || d.title ? wrap : null;
      }
      case "free_text": {
        if (!d.body) return null;
        const wrap = document.createElement("div");
        wrap.className = "block";
        const body = document.createElement("div");
        body.className = "block-body";
        const text = document.createElement("p");
        text.className = "block-text";
        text.textContent = d.body;
        body.appendChild(text);
        wrap.appendChild(body);
        return wrap;
      }
      case "opening_hours": {
        if (!d.text) return null;
        return simpleTextBlock("Opening hours", d.text);
      }
      case "location": {
        if (!d.address && !d.map_url) return null;
        const wrap = document.createElement("div");
        wrap.className = "block";
        const body = document.createElement("div");
        body.className = "block-body";
        const title = document.createElement("div");
        title.className = "block-title";
        title.textContent = "Location";
        body.appendChild(title);
        if (d.address) {
          const addr = document.createElement("p");
          addr.className = "block-text";
          addr.textContent = d.address;
          body.appendChild(addr);
        }
        if (d.map_url) {
          const a = document.createElement("a");
          a.className = "location-map";
          a.href = d.map_url;
          a.target = "_blank";
          a.rel = "noopener noreferrer";
          a.textContent = "Open in maps →";
          body.appendChild(a);
        }
        wrap.appendChild(body);
        return wrap;
      }
      case "social_links": {
        const entries = Object.entries(d).filter(([, v]) => v);
        if (entries.length === 0) return null;
        const wrap = document.createElement("div");
        wrap.className = "block";
        const body = document.createElement("div");
        body.className = "block-body";
        const title = document.createElement("div");
        title.className = "block-title";
        title.textContent = "Follow us";
        body.appendChild(title);
        const list = document.createElement("div");
        list.className = "social-links";
        for (const [platform, url] of entries) {
          const a = document.createElement("a");
          a.href = url;
          a.target = "_blank";
          a.rel = "noopener noreferrer";
          a.textContent = SOCIAL_LABELS[platform] || platform;
          list.appendChild(a);
        }
        body.appendChild(list);
        wrap.appendChild(body);
        return wrap;
      }
      case "contact": {
        if (!d.phone) return null;
        const wrap = document.createElement("div");
        wrap.className = "block";
        const body = document.createElement("div");
        body.className = "block-body";
        const title = document.createElement("div");
        title.className = "block-title";
        title.textContent = "Contact";
        body.appendChild(title);
        const a = document.createElement("a");
        a.className = "contact-link";
        a.href = "tel:" + d.phone.replace(/\s+/g, "");
        a.textContent = d.phone;
        body.appendChild(a);
        wrap.appendChild(body);
        return wrap;
      }
      default:
        return null;
    }
  }

  function simpleTextBlock(title, text) {
    const wrap = document.createElement("div");
    wrap.className = "block";
    const body = document.createElement("div");
    body.className = "block-body";
    const t = document.createElement("div");
    t.className = "block-title";
    t.textContent = title;
    body.appendChild(t);
    const p = document.createElement("p");
    p.className = "block-text";
    p.textContent = text;
    body.appendChild(p);
    wrap.appendChild(body);
    return wrap;
  }

  // --- Menu rendering ------------------------------------------------------
  function renderMenu(categories, items) {
    el.menu.innerHTML = "";
    const byCategory = new Map();
    for (const it of items) {
      if (!byCategory.has(it.category_id)) byCategory.set(it.category_id, []);
      byCategory.get(it.category_id).push(it);
    }
    const sortedCats = [...categories].sort((a, b) => a.position - b.position);
    for (const cat of sortedCats) {
      const catItems = byCategory.get(cat.id);
      if (!catItems || catItems.length === 0) continue;
      const section = document.createElement("section");
      section.className = "category";
      const h = document.createElement("h2");
      h.textContent = cat.name;
      section.appendChild(h);
      for (const item of catItems) section.appendChild(renderItem(item));
      el.menu.appendChild(section);
    }
    el.menu.hidden = false;
  }

  function renderItem(item) {
    const card = document.createElement("article");
    card.className = "item" + (item.available ? "" : " unavailable");

    const head = document.createElement("div");
    head.className = "item-head";
    const name = document.createElement("div");
    name.className = "item-name";
    name.textContent = item.name;
    const price = document.createElement("div");
    price.className = "item-price aivo-num";
    price.textContent = money(item.price_cents);
    head.appendChild(name);
    head.appendChild(price);
    card.appendChild(head);

    if (item.description) {
      const desc = document.createElement("p");
      desc.className = "item-desc";
      desc.textContent = item.description;
      card.appendChild(desc);
    }

    if (item.allergens && item.allergens.length > 0) {
      const tags = document.createElement("div");
      tags.className = "item-allergens";
      for (const a of item.allergens) {
        const tag = document.createElement("span");
        tag.className = "allergen-tag";
        tag.textContent = ALLERGEN_LABELS[a] || a;
        tags.appendChild(tag);
      }
      card.appendChild(tags);
    }

    if (!item.available) {
      const badge = document.createElement("span");
      badge.className = "unavailable-tag";
      badge.textContent = "Currently unavailable";
      card.appendChild(badge);
      return card;
    }

    const groupState = new Map(); // groupId -> Set(optionId) for multi, or single id for single-select

    if (item.option_groups && item.option_groups.length > 0) {
      const groupsWrap = document.createElement("div");
      groupsWrap.className = "option-groups";
      for (const group of item.option_groups) {
        groupsWrap.appendChild(renderOptionGroup(item.id, group, groupState));
      }
      card.appendChild(groupsWrap);
    }

    const foot = document.createElement("div");
    foot.className = "item-foot";
    let qty = 1;
    const stepper = document.createElement("div");
    stepper.className = "qty-stepper";
    const minus = document.createElement("button");
    minus.type = "button";
    minus.textContent = "−";
    minus.setAttribute("aria-label", "Decrease quantity");
    const qtyLabel = document.createElement("span");
    qtyLabel.textContent = String(qty);
    const plus = document.createElement("button");
    plus.type = "button";
    plus.textContent = "+";
    plus.setAttribute("aria-label", "Increase quantity");
    minus.addEventListener("click", () => { qty = Math.max(1, qty - 1); qtyLabel.textContent = String(qty); });
    plus.addEventListener("click", () => { qty = Math.min(99, qty + 1); qtyLabel.textContent = String(qty); });
    stepper.appendChild(minus);
    stepper.appendChild(qtyLabel);
    stepper.appendChild(plus);

    const addBtn = document.createElement("button");
    addBtn.type = "button";
    addBtn.className = "add-btn";
    addBtn.textContent = "Add";
    addBtn.addEventListener("click", () => {
      const addedQty = qty;
      const optionIds = [];
      const optionDetails = [];
      for (const group of item.option_groups || []) {
        const picked = groupState.get(group.id);
        const ids = picked instanceof Set ? [...picked] : picked ? [picked] : [];
        for (const oid of ids) {
          const opt = group.options.find((o) => o.id === oid);
          if (opt) { optionIds.push(oid); optionDetails.push({ id: oid, label: opt.label, priceDeltaCents: opt.price_delta_cents }); }
        }
      }
      addToCart(item, addedQty, optionIds, optionDetails);
      qty = 1;
      qtyLabel.textContent = "1";
      showToast("success", "Added to order", item.name + " × " + addedQty);
    });

    foot.appendChild(stepper);
    foot.appendChild(addBtn);
    card.appendChild(foot);

    return card;
  }

  function renderOptionGroup(itemId, group, groupState) {
    const wrap = document.createElement("div");
    wrap.className = "option-group";
    const label = document.createElement("div");
    label.className = "option-group-name";
    label.textContent = group.name + (group.multi ? " (choose any)" : " (choose one)");
    wrap.appendChild(label);

    if (group.multi) {
      groupState.set(group.id, new Set());
    }

    group.options.forEach((opt, idx) => {
      const row = document.createElement("div");
      row.className = "option-row";
      const l = document.createElement("label");
      const input = document.createElement("input");
      input.type = group.multi ? "checkbox" : "radio";
      input.name = "group-" + itemId + "-" + group.id;
      if (!group.multi && idx === 0) {
        input.checked = true;
        groupState.set(group.id, opt.id); // single-select defaults to first option
      }
      input.addEventListener("change", () => {
        if (group.multi) {
          const set = groupState.get(group.id);
          if (input.checked) set.add(opt.id); else set.delete(opt.id);
        } else {
          groupState.set(group.id, opt.id);
        }
      });
      const span = document.createElement("span");
      span.textContent = opt.label;
      l.appendChild(input);
      l.appendChild(span);
      row.appendChild(l);
      if (opt.price_delta_cents !== 0) {
        const delta = document.createElement("span");
        delta.className = "option-delta";
        delta.textContent = (opt.price_delta_cents > 0 ? "+" : "") + money(opt.price_delta_cents);
        row.appendChild(delta);
      }
      wrap.appendChild(row);
    });
    return wrap;
  }

  // --- Cart ------------------------------------------------------------------
  function addToCart(item, qty, optionIds, optionDetails) {
    const key = lineKey(item.id, optionIds);
    const existing = state.cart.get(key);
    if (existing) {
      existing.qty += qty;
    } else {
      state.cart.set(key, {
        itemId: item.id,
        name: item.name,
        unitPriceCents: item.price_cents,
        qty,
        options: optionDetails,
      });
    }
    renderCartSummary();
  }

  function setLineQty(key, qty) {
    const line = state.cart.get(key);
    if (!line) return;
    if (qty <= 0) state.cart.delete(key);
    else line.qty = qty;
    renderCartSummary();
    renderCartPanel();
  }

  function renderCartSummary() {
    const count = cartCount();
    const total = cartTotalCents();
    el.cartCount.textContent = count + (count === 1 ? " item" : " items");
    el.cartTotal.textContent = money(total);
    el.cartPanelTotal.textContent = money(total);
    el.cartBar.hidden = count === 0;
    el.submitOrder.disabled = count === 0;
    if (count === 0) el.cartPanel.hidden = true;
  }

  function renderCartPanel() {
    el.cartLines.innerHTML = "";
    if (state.cart.size === 0) {
      const empty = document.createElement("div");
      empty.className = "cart-empty";
      empty.textContent = "Your order is empty.";
      el.cartLines.appendChild(empty);
      return;
    }
    for (const [key, line] of state.cart.entries()) {
      const row = document.createElement("div");
      row.className = "cart-line";

      const info = document.createElement("div");
      const name = document.createElement("div");
      name.className = "cart-line-name";
      name.textContent = line.name;
      info.appendChild(name);
      if (line.options.length > 0) {
        const opts = document.createElement("div");
        opts.className = "cart-line-opts";
        opts.textContent = line.options.map((o) => o.label).join(", ");
        info.appendChild(opts);
      }
      const controls = document.createElement("div");
      controls.className = "cart-line-controls";
      const stepper = document.createElement("div");
      stepper.className = "qty-stepper";
      const minus = document.createElement("button");
      minus.type = "button";
      minus.textContent = "−";
      minus.addEventListener("click", () => setLineQty(key, line.qty - 1));
      const qtyLabel = document.createElement("span");
      qtyLabel.textContent = String(line.qty);
      const plus = document.createElement("button");
      plus.type = "button";
      plus.textContent = "+";
      plus.addEventListener("click", () => setLineQty(key, line.qty + 1));
      stepper.appendChild(minus);
      stepper.appendChild(qtyLabel);
      stepper.appendChild(plus);
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "cart-line-remove";
      remove.textContent = "Remove";
      remove.addEventListener("click", () => setLineQty(key, 0));
      controls.appendChild(stepper);
      controls.appendChild(remove);
      info.appendChild(controls);

      const price = document.createElement("div");
      price.className = "cart-line-price aivo-num";
      price.textContent = money(lineUnitPrice(line) * line.qty);

      row.appendChild(info);
      row.appendChild(price);
      el.cartLines.appendChild(row);
    }
  }

  async function submitOrder() {
    if (state.cart.size === 0) return;
    el.submitOrder.disabled = true;
    el.submitOrder.textContent = "Submitting…";
    try {
      const lines = [...state.cart.values()].map((line) => ({
        menu_item_id: line.itemId,
        option_ids: line.options.map((o) => o.id),
        qty: line.qty,
      }));
      await apiPost("/api/orders", {
        restaurant_slug: state.route.slug,
        table_token: state.route.token,
        lines,
        comment: el.cartComment.value.trim(),
      });
      state.cart.clear();
      el.cartComment.value = "";
      renderCartSummary();
      renderCartPanel();
      el.cartPanel.hidden = true;
      showToast("success", "Order sent", "The kitchen has received your order.");
    } catch (err) {
      if (err.status === 429) {
        showToast("warn", "Please wait", "You can only submit an order every 30 seconds. Try again shortly.");
      } else {
        showToast("error", "Order failed", err.message || "Please try again.");
      }
    } finally {
      el.submitOrder.textContent = "Submit order";
      el.submitOrder.disabled = state.cart.size === 0;
    }
  }

  async function sendServiceRequest(kind) {
    try {
      await apiPost("/api/service-requests", {
        restaurant_slug: state.route.slug,
        table_token: state.route.token,
        kind,
      });
      showToast(
        "success",
        kind === "call_waiter" ? "Waiter on the way" : "Bill requested",
        "Staff have been notified."
      );
    } catch (err) {
      if (err.status === 429) {
        showToast("warn", "Please wait", "A request of this kind is already on its way to your table.");
      } else {
        showToast("error", "Couldn't send request", err.message || "Please try again.");
      }
    }
  }

  // --- Wiring ------------------------------------------------------------
  function wireStaticControls() {
    el.cartToggle.addEventListener("click", () => {
      renderCartPanel();
      el.cartPanel.hidden = false;
    });
    el.cartClose.addEventListener("click", () => { el.cartPanel.hidden = true; });
    el.submitOrder.addEventListener("click", submitOrder);
    document.querySelectorAll("[data-service]").forEach((btn) => {
      btn.addEventListener("click", () => sendServiceRequest(btn.dataset.service));
    });
  }

  async function init() {
    wireStaticControls();
    const route = parseRoute();
    if (!route) {
      showFatalError("This link isn't valid. Ask staff for a fresh QR code.");
      return;
    }
    state.route = route;

    try {
      const [landing, menu] = await Promise.all([
        apiGet(`/api/landing/${encodeURIComponent(route.slug)}/${encodeURIComponent(route.token)}`),
        apiGet(`/api/menu/${encodeURIComponent(route.slug)}`),
      ]);

      state.restaurant = landing.restaurant;
      el.restaurantName.textContent = landing.restaurant.name;
      el.tableLabel.textContent = landing.table.label;
      document.title = landing.restaurant.name;

      state.categories = menu.categories;
      state.items = menu.items;

      el.loadMessage.hidden = true;
      renderLanding(landing.landing_blocks || []);
      renderMenu(menu.categories || [], menu.items || []);
      renderCartSummary();
    } catch (err) {
      if (err.status === 429) {
        showFatalError("Too many requests right now — please wait a moment and reload.");
      } else if (err.status === 404) {
        showFatalError("This link isn't valid. Ask staff for a fresh QR code.");
      } else {
        showFatalError(err.message || "Couldn't load the menu. Please try again.");
      }
    }
  }

  init();
})();
