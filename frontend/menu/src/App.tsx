import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError, httpClient, mockClient, preferMock, type Client } from "./api";
import {
  clearHandoff,
  handoffExpired,
  loadCart,
  loadCooldownUntil,
  loadHandoff,
  loadSentAt,
  saveCart,
  saveCooldownUntil,
  saveHandoff,
  saveSentAt,
  type CartLine,
  type StoredHandoff,
} from "./cart";
import { formatCents } from "../../design-system/shared/money";
import { themeVars } from "../../design-system/shared/theme";
import { countdownStr, hhmm } from "./format";
import { AccountScreen } from "./screens/AccountScreen";
import { AuthSheet } from "./screens/AuthSheet";
import { CartScreen } from "./screens/CartScreen";
import { ExpiredScreen, NotFoundScreen } from "./screens/ExpiredScreen";
import { HandoffScreen } from "./screens/HandoffScreen";
import { ItemScreen } from "./screens/ItemScreen";
import { Landing } from "./screens/Landing";
import { MenuScreen } from "./screens/MenuScreen";
import { SentScreen } from "./screens/SentScreen";
import { ServiceScreen } from "./screens/ServiceScreen";
import type { CustomerMe, TableSession } from "./types";

/** Fallback when the server sends neither cooldown_seconds nor retry_after_seconds. */
const FALLBACK_COOLDOWN_S = 30;

type Screen = "landing" | "menu" | "item" | "cart" | "sent" | "service" | "account" | "handoff";

export interface SentOrder {
  lines: { name: string; qty: number }[];
  count: number;
  time: string;
}

type Link =
  | { mode: "table"; slug: string; token: string }
  | { mode: "browse"; slug: string; menuSlug: string };

/** Diner entry /{restaurant_slug}/t/{table_token}, or public browse /{restaurant_slug}/m/{menu_slug}. */
function parseLink(): Link | null {
  let m = location.pathname.match(/^\/([^/]+)\/t\/([^/]+)/);
  if (m) return { mode: "table", slug: m[1], token: m[2] };
  m = location.pathname.match(/^\/([^/]+)\/m\/([^/]+)/);
  if (m) return { mode: "browse", slug: m[1], menuSlug: m[2] };
  return null;
}

export default function App() {
  const link = useMemo(parseLink, []);
  const browse = link?.mode === "browse";
  // In mock mode a bare URL still demos with the fixture tenant.
  const token = link?.mode === "table" ? link.token : !link && preferMock ? "demo" : null;

  const [client, setClient] = useState<Client>(() => (preferMock ? mockClient : httpClient));
  const [phase, setPhase] = useState<"loading" | "ready" | "expired" | "notfound">("loading");
  const [session, setSession] = useState<TableSession | null>(null);
  const [screen, setScreen] = useState<Screen>("landing");
  const [menuIdx, setMenuIdx] = useState(0);
  const [cat, setCat] = useState(0);
  const [itemId, setItemId] = useState<string | null>(null);
  const [cart, setCart] = useState<CartLine[]>(() => (token ? loadCart(token) : []));
  const [note, setNote] = useState("");
  const [sent, setSent] = useState<SentOrder | null>(null);
  const [lastSentAt, setLastSentAt] = useState<number | null>(() => (token ? loadSentAt(token) : null));
  const [cooldownUntil, setCooldownUntil] = useState<number | null>(() => (token ? loadCooldownUntil(token) : null));
  const [waiterAt, setWaiterAt] = useState<string | null>(null);
  const [billAt, setBillAt] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [cartError, setCartError] = useState<string | null>(null);
  const [cartNotice, setCartNotice] = useState<string | null>(null);
  const [me, setMe] = useState<CustomerMe | null>(null);
  const [authOpen, setAuthOpen] = useState(false);
  const [handoff, setHandoff] = useState<StoredHandoff | null>(() => (token ? loadHandoff(token) : null));
  const [now, setNow] = useState(() => Date.now());

  // A refresh mid-handoff returns to the code screen.
  useEffect(() => {
    if (handoff && !handoffExpired(handoff, Date.now())) setScreen("handoff");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Customer session — optional; anonymous flow untouched.
  useEffect(() => {
    if (browse) return;
    let live = true;
    client
      .me()
      .then((m) => {
        if (live) setMe(m);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [client, browse]);

  // Expired unused pickup code puts the cart back.
  useEffect(() => {
    if (!token || !handoff) return;
    if (handoffExpired(handoff, now)) {
      // Storage doubles as the idempotency guard (StrictMode runs effects twice).
      if (loadHandoff(token)) {
        setCart((c) => [...handoff.backup, ...c]);
        setCartNotice("Code expired unused — your cart is back.");
        clearHandoff(token);
      }
      setHandoff(null);
      setScreen((s) => (s === "handoff" ? "cart" : s));
    }
  }, [now, handoff, token]);

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    if (!browse && !token) {
      setPhase("expired");
      return;
    }
    let live = true;
    const load =
      link?.mode === "browse"
        ? // Browse mode reuses the table-session shape with a single menu and no table.
          client.getBrowse(link.slug, link.menuSlug).then(
            (b): TableSession => ({
              restaurant: b.restaurant,
              table: { id: "", label: "" },
              theme: b.theme,
              menus: [b.menu],
              open_requests: [],
            }),
          )
        : client.getSession(token!);
    load
      .then((s) => {
        if (!live) return;
        setSession(s);
        for (const r of s.open_requests ?? []) {
          if (r.type === "waiter") setWaiterAt(hhmm(r.created_at));
          if (r.type === "bill") setBillAt(hhmm(r.created_at));
        }
        document.title = s.theme.brand_name;
        setPhase("ready");
      })
      .catch((e) => {
        if (!live) return;
        if (e instanceof ApiError && e.status === 0 && client !== mockClient) {
          console.warn("AIVO menu: API unreachable, falling back to mock fixtures");
          setClient(mockClient); // effect reruns with the mock
        } else {
          setPhase(browse ? "notfound" : "expired");
        }
      });
    return () => {
      live = false;
    };
  }, [token, client, link, browse]);

  useEffect(() => {
    if (token) saveCart(token, cart);
  }, [token, cart]);

  const cooldownLeft = cooldownUntil ? Math.max(0, cooldownUntil - now) : 0;
  const rateLimited = cooldownLeft > 0;

  const setSentTimestamp = useCallback(
    (at: number) => {
      setLastSentAt(at);
      if (token) saveSentAt(token, at);
    },
    [token],
  );

  // The server owns the debounce window; the client only mirrors it.
  const startCooldown = useCallback(
    (seconds: number) => {
      const until = Date.now() + seconds * 1000;
      setCooldownUntil(until);
      if (token) saveCooldownUntil(token, until);
    },
    [token],
  );

  const handle429 = useCallback(
    (e: ApiError) => {
      startCooldown(e.retryAfterSeconds ?? FALLBACK_COOLDOWN_S);
      // Another phone at the table may have sent it; approximate for the card copy.
      setLastSentAt((t) => t ?? Date.now());
    },
    [startCooldown],
  );

  async function sendOrder() {
    if (!token || sending || rateLimited || cart.length === 0) return;
    setSending(true);
    setCartError(null);
    try {
      await client.submitOrder(token, {
        lines: cart.map((l) => ({ menu_item_id: l.menuItemId, qty: l.qty, options: l.options })),
        note: note.trim() || undefined,
      });
      const at = Date.now();
      setSent({
        lines: cart.map((l) => ({ name: l.name, qty: l.qty })),
        count: cart.reduce((t, l) => t + l.qty, 0),
        time: hhmm(at),
      });
      setCart([]);
      setNote("");
      setCartNotice(null);
      setSentTimestamp(at);
      startCooldown(session?.cooldown_seconds ?? FALLBACK_COOLDOWN_S);
      setScreen("sent");
    } catch (e) {
      if (e instanceof ApiError && e.status === 429) {
        handle429(e);
      } else {
        setCartError(e instanceof ApiError ? e.message : "Something went wrong — nothing was sent. Try again.");
      }
    } finally {
      setSending(false);
    }
  }

  async function sendHandoff() {
    if (!token || sending || cart.length === 0) return;
    setSending(true);
    setCartError(null);
    try {
      const h = await client.submitHandoff(token, {
        lines: cart.map((l) => ({ menu_item_id: l.menuItemId, qty: l.qty, options: l.options })),
        note: note.trim() || undefined,
      });
      const stored = { handoff: h, backup: cart };
      saveHandoff(token, stored);
      setHandoff(stored);
      setCart([]);
      setNote("");
      setCartNotice(null);
      setScreen("handoff");
    } catch (e) {
      if (e instanceof ApiError && e.status === 429) {
        handle429(e);
      } else {
        setCartError(e instanceof ApiError ? e.message : "Something went wrong — nothing was sent. Try again.");
      }
    } finally {
      setSending(false);
    }
  }

  async function doLogout() {
    try {
      await client.logout();
    } catch {
      // best effort
    }
    setMe(null);
    setScreen("menu");
  }

  async function submitRequest(type: "waiter" | "bill") {
    if (!token) return;
    try {
      const r = await client.submitRequest(token, type);
      const at = hhmm(r.created_at);
      if (type === "waiter") setWaiterAt(at);
      else setBillAt(at);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        // Someone at the table already asked — reflect the open state.
        const at = hhmm(Date.now());
        if (type === "waiter") setWaiterAt(at);
        else setBillAt(at);
      }
    }
  }

  const theme = session?.theme;
  const vars = theme ? themeVars(theme) : {};
  const accountLabel = browse ? null : me ? me.customer.name.split(" ")[0] : "Sign in";
  const onAccount = () => (me ? setScreen("account") : setAuthOpen(true));
  const cartCount = cart.reduce((t, l) => t + l.qty, 0);
  const cartTotal = cart.reduce((t, l) => t + l.unitCents * l.qty, 0);
  const item =
    itemId && session
      ? session.menus
          .flatMap((m) => m.categories)
          .flatMap((c) => c.items)
          .find((i) => i.id === itemId)
      : null;

  let body;
  if (phase === "loading") {
    body = (
      <div style={{ flex: 1, display: "grid", placeItems: "center", font: "var(--type-body)", color: "var(--ink-400)" }}>
        Loading the menu…
      </div>
    );
  } else if (phase === "notfound") {
    body = <NotFoundScreen />;
  } else if (phase === "expired" || !session) {
    body = <ExpiredScreen />;
  } else if (screen === "landing") {
    body = (
      <Landing
        session={session}
        browse={browse}
        accountLabel={accountLabel}
        onAccount={onAccount}
        onMenu={() => setScreen("menu")}
      />
    );
  } else if (screen === "account" && me) {
    body = <AccountScreen me={me} onLogout={doLogout} onBack={() => setScreen("menu")} />;
  } else if (screen === "handoff" && handoff) {
    body = (
      <HandoffScreen
        handoff={handoff.handoff}
        now={now}
        tableLabel={session.table.label}
        onMenu={() => setScreen("menu")}
      />
    );
  } else if (screen === "item" && item) {
    body = (
      <ItemScreen
        item={item}
        browse={browse}
        onBack={() => setScreen("menu")}
        onAdd={(line) => {
          setCart([...cart, line]);
          setScreen("menu");
        }}
      />
    );
  } else if (screen === "cart") {
    body = (
      <CartScreen
        tableLabel={session.table.label}
        lines={cart}
        onSetQty={(i, qty) => setCart(cart.map((l, li) => (li === i ? { ...l, qty } : l)))}
        onRemove={(i) => setCart(cart.filter((_, li) => li !== i))}
        note={note}
        onNote={setNote}
        rateLimited={rateLimited}
        countdown={countdownStr(cooldownLeft)}
        lastSentTime={lastSentAt ? hhmm(lastSentAt) : ""}
        error={cartError}
        notice={cartNotice}
        sending={sending}
        onSend={sendOrder}
        onHandoff={sendHandoff}
        onMenu={() => setScreen("menu")}
      />
    );
  } else if (screen === "sent" && sent) {
    body = (
      <SentScreen
        sent={sent}
        tableLabel={session.table.label}
        onMenu={() => setScreen("menu")}
        onService={() => setScreen("service")}
      />
    );
  } else if (screen === "service") {
    body = (
      <ServiceScreen
        tableLabel={session.table.label}
        waiterAt={waiterAt}
        billAt={billAt}
        onCallWaiter={() => submitRequest("waiter")}
        onRequestBill={() => submitRequest("bill")}
        onMenu={() => setScreen("menu")}
      />
    );
  } else {
    body = (
      <MenuScreen
        session={session}
        browse={browse}
        menuIdx={menuIdx}
        onPickMenu={(i) => {
          setMenuIdx(i);
          setCat(0);
        }}
        cat={cat}
        onPickCat={setCat}
        cartLabel={cartCount ? "Cart · " + formatCents(cartTotal) : "Cart"}
        accountLabel={accountLabel}
        onAccount={onAccount}
        onOpenItem={(it) => {
          setItemId(it.id);
          setScreen("item");
        }}
        onLanding={() => setScreen("landing")}
        onCart={() => setScreen("cart")}
        onService={() => setScreen("service")}
      />
    );
  }

  return (
    <div className={theme?.bold ? "theme-bold" : undefined} style={vars as React.CSSProperties}>
      <div style={{ minHeight: "100dvh", background: "var(--paper-2)", display: "flex", justifyContent: "center", fontFamily: "var(--font-sans)" }}>
        <div
          style={{
            position: "relative",
            width: "100%",
            maxWidth: 430,
            height: "100dvh",
            display: "flex",
            flexDirection: "column",
            background: "var(--paper-1)",
            borderLeft: "1px solid var(--border-default)",
            borderRight: "1px solid var(--border-default)",
            overflow: "hidden",
          }}
        >
          {body}
          {authOpen ? (
            <AuthSheet
              client={client}
              onClose={() => setAuthOpen(false)}
              onDone={async () => {
                setAuthOpen(false);
                setMe(await client.me());
              }}
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}
