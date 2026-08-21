import { useCallback, useEffect, useState } from "react";
import { api } from "./api.ts";
import type { HandoffPreview, Me, NewLine, PosRequest, PosState, PostedShift, Table } from "./types.ts";
import { fmt, parseDollars, timeHM, waiting } from "./format.ts";
import { Badge, Button, EmptyState, Icon, StatusPill } from "./ui.tsx";

type Route =
  | { name: "floor" }
  | { name: "ticket"; tableId: string }
  | { name: "requests" }
  | { name: "pick" }
  | { name: "order"; tableId: string }
  | { name: "handoff" }
  | { name: "close" };

const ticketTotal = (t: Table) => (t.ticket ? t.ticket.lines.reduce((a, l) => a + l.unit_price_cents * l.qty, 0) : 0);

export default function App() {
  const [auth, setAuth] = useState<"loading" | "login" | Me>("loading");
  const [pos, setPos] = useState<PosState | null>(null);
  const [posted, setPosted] = useState<PostedShift | null>(null);
  const [route, setRoute] = useState<Route>({ name: "floor" });
  const [now, setNow] = useState(Date.now());

  const refresh = useCallback(() => api.state().then(setPos).catch(() => {}), []);

  useEffect(() => {
    api
      .me()
      .then((me) => {
        setAuth(me);
        return refresh();
      })
      .catch(() => setAuth("login"));
  }, [refresh]);

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);

  // poll every 5s while visible
  useEffect(() => {
    if (typeof auth === "string") return;
    const tick = () => {
      if (document.visibilityState === "visible") refresh();
    };
    const t = setInterval(tick, 5000);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(t);
      document.removeEventListener("visibilitychange", tick);
    };
  }, [auth, refresh]);

  // optimistic update: apply locally, call the API, reconcile with server state
  const mutate = useCallback(
    (apply: (p: PosState) => void, call: () => Promise<unknown>) => {
      setPos((p) => {
        if (!p) return p;
        const c = structuredClone(p);
        apply(c);
        return c;
      });
      call().then(refresh, refresh);
    },
    [refresh]
  );

  if (auth === "loading") return <Frame clock={timeHM(now)} context="aivo · waiter" />;
  if (auth === "login")
    return (
      <Frame clock={timeHM(now)} context="aivo · waiter">
        <Login
          onDone={(me) => {
            setAuth(me);
            refresh();
          }}
        />
      </Frame>
    );
  if (!pos) return <Frame clock={timeHM(now)} context="aivo · waiter" />;

  if (posted) {
    const variance = posted.declared_cents - posted.expected_cents;
    return (
      <Frame clock={timeHM(now)} context={`${posted.number} · accepted`}>
        <div className="screen">
          <div className="screen-body" style={{ display: "flex", flexDirection: "column", justifyContent: "center", padding: "0 22px" }}>
            <div
              style={{
                width: 52,
                height: 52,
                borderRadius: "50%",
                background: "var(--green-100)",
                border: "1px solid var(--green-200)",
                display: "grid",
                placeItems: "center",
                marginBottom: 20,
                color: "var(--green-700)",
              }}
            >
              <Icon name="check" size={24} />
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 8 }}>
              <h2 style={{ margin: 0, font: "var(--weight-regular) 27px/1.15 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
                {posted.number} accepted
              </h2>
              <StatusPill status="accepted" label="accepted" />
            </div>
            <p style={{ margin: "0 0 20px", font: "var(--weight-regular) 15px/1.55 var(--font-sans)", color: "var(--ink-600)" }}>
              Posted at {posted.posted_at} · {posted.gl_lines} GL lines posted. This shift cannot be cancelled or reopened.
            </p>
            <div className="card kv-card">
              <div className="kv-row" style={{ padding: "11px 0" }}>
                <span className="kv-key">Expected cash</span>
                <span className="aivo-num" style={{ color: "var(--ink-900)" }}>{fmt(posted.expected_cents)}</span>
              </div>
              <div className="kv-row" style={{ padding: "11px 0" }}>
                <span className="kv-key">Declared cash</span>
                <span className="aivo-num" style={{ color: "var(--ink-900)" }}>{fmt(posted.declared_cents)}</span>
              </div>
              <div className="kv-row" style={{ padding: "11px 0" }}>
                <span className="kv-key-strong">Variance</span>
                <VarianceValue variance={variance} />
              </div>
            </div>
          </div>
          <div className="screen-footer" style={{ padding: "12px 14px 16px" }}>
            <Button
              fullWidth
              onClick={() => {
                setPosted(null);
                setRoute({ name: "floor" });
                refresh();
              }}
            >
              Open the next shift
            </Button>
          </div>
        </div>
      </Frame>
    );
  }

  if (!pos.shift)
    return (
      <Frame clock={timeHM(now)} context="aivo · waiter">
        <OpenShift pos={pos} onOpened={refresh} />
      </Frame>
    );

  const shift = pos.shift;
  const contexts: Record<Route["name"], string> = {
    floor: `${shift.number} · till ${shift.till}`,
    ticket: route.name === "ticket" ? `table ${pos.tables.find((t) => t.id === route.tableId)?.number ?? ""}` : "",
    pick: "new order",
    order: route.name === "order" ? `table ${pos.tables.find((t) => t.id === route.tableId)?.number ?? ""} · new order` : "",
    requests: "service requests",
    handoff: "add from code",
    close: shift.number,
  };

  const goFloor = () => setRoute({ name: "floor" });
  const startOrder = (tableId: string) => setRoute({ name: "order", tableId });

  const ackRequest = (r: PosRequest) =>
    mutate(
      (p) => {
        p.requests = p.requests.filter((x) => x.id !== r.id);
      },
      () => api.ack(r.id)
    );

  let screen: React.ReactNode = null;

  if (route.name === "floor") {
    screen = (
      <Floor
        pos={pos}
        onTable={(t) => (t.ticket ? setRoute({ name: "ticket", tableId: t.id }) : startOrder(t.id))}
        onRequests={() => setRoute({ name: "requests" })}
        onAddFromCode={() => setRoute({ name: "handoff" })}
        onNewOrder={() => setRoute({ name: "pick" })}
        onClose={() => setRoute({ name: "close" })}
      />
    );
  } else if (route.name === "ticket") {
    const table = pos.tables.find((t) => t.id === route.tableId);
    screen = table ? (
      <Ticket
        table={table}
        onBack={goFloor}
        onAdd={() => startOrder(table.id)}
        onFire={() =>
          mutate(
            (p) => {
              const tb = p.tables.find((x) => x.id === table.id);
              if (tb?.ticket) tb.ticket.fired_at = timeHM();
            },
            () => api.fire(table.ticket!.id)
          )
        }
      />
    ) : null;
    if (!table) goFloor();
  } else if (route.name === "requests") {
    screen = (
      <Requests
        pos={pos}
        now={now}
        onBack={goFloor}
        onDismiss={(r) =>
          mutate(
            (p) => {
              p.requests = p.requests.filter((x) => x.id !== r.id);
            },
            () => api.dismiss(r.id)
          )
        }
        onAck={(r) => {
          ackRequest(r);
          setRoute({ name: "floor" });
        }}
        onTakeBill={(r) => {
          ackRequest(r);
          setRoute({ name: "ticket", tableId: r.table_id });
        }}
      />
    );
  } else if (route.name === "pick") {
    screen = <PickTable pos={pos} onBack={goFloor} onPick={(t) => startOrder(t.id)} />;
  } else if (route.name === "order") {
    const table = pos.tables.find((t) => t.id === route.tableId);
    screen = table ? (
      <TakeOrder
        pos={pos}
        table={table}
        onCancel={() => {
          if (table.ticket && table.ticket.lines.length) setRoute({ name: "ticket", tableId: table.id });
          else goFloor();
        }}
        onCommit={(lines) => {
          mutate(
            (p) => {
              const tb = p.tables.find((x) => x.id === table.id);
              if (!tb) return;
              const items = p.menu.flatMap((c) => c.items);
              const t = timeHM();
              if (!tb.ticket) {
                tb.covers = tb.covers ?? 2;
                tb.ticket = { id: "optimistic", lines: [], note: null, source: "", placed_at: null, fired_at: null };
              }
              for (const l of lines) {
                const item = items.find((i) => i.id === l.menu_item_id);
                if (!item) continue;
                tb.ticket.lines.push({
                  id: "optimistic-" + l.menu_item_id,
                  menu_item_id: item.id,
                  name: item.name,
                  qty: l.qty,
                  options: l.options,
                  unit_price_cents: item.price_cents,
                });
              }
              tb.ticket.source = "taken at the table · " + t;
              tb.ticket.placed_at = t;
              tb.ticket.fired_at = null;
            },
            () => api.addLines(table.id, lines)
          );
          setRoute({ name: "ticket", tableId: table.id });
        }}
      />
    ) : null;
  } else if (route.name === "handoff") {
    screen = (
      <Handoff
        pos={pos}
        onBack={goFloor}
        onAccept={(preview, tableId) => {
          mutate(
            (p) => {
              const tb = p.tables.find((x) => x.id === tableId);
              if (!tb) return;
              const t = timeHM();
              if (!tb.ticket) {
                tb.covers = tb.covers ?? 2;
                tb.ticket = { id: "optimistic", lines: [], note: null, source: "", placed_at: null, fired_at: null };
              }
              tb.ticket.lines.push(...preview.lines.map((l, i) => ({ ...l, id: "optimistic-h" + i })));
              if (preview.note) tb.ticket.note = tb.ticket.note ? tb.ticket.note + " " + preview.note : preview.note;
              tb.ticket.source = "from the diner's phone · " + t;
              tb.ticket.placed_at = t;
              tb.ticket.fired_at = null;
            },
            () => api.acceptHandoff(preview.code, tableId)
          );
          setRoute({ name: "ticket", tableId });
        }}
      />
    );
  } else if (route.name === "close") {
    screen = (
      <CloseShift
        pos={pos}
        onBack={goFloor}
        onPosted={(p) => {
          setPosted(p);
          refresh();
        }}
      />
    );
  }

  return (
    <Frame clock={timeHM(now)} context={contexts[route.name]}>
      {screen}
    </Frame>
  );
}

function Frame({ clock, context, children }: { clock: string; context: string; children?: React.ReactNode }) {
  return (
    <div className="app-outer">
      <div className="app-frame">
        <div className="statusbar">
          <span>{clock}</span>
          <span>{context}</span>
        </div>
        {children}
      </div>
    </div>
  );
}

function Login({ onDone }: { onDone: (me: Me) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = () => {
    if (!email || !password || busy) return;
    setBusy(true);
    setErr("");
    api
      .login(email, password)
      .then(onDone)
      .catch((e: { status?: number; message?: string }) => {
        setErr(e.status === 401 ? "Wrong email or password." : (e.message ?? "Could not sign in."));
        setBusy(false);
      });
  };
  return (
    <div className="screen">
      <form
        className="screen-body"
        style={{ display: "flex", flexDirection: "column", justifyContent: "center", padding: "0 22px", gap: 10 }}
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <h2 style={{ margin: 0, font: "var(--weight-regular) 27px/1.15 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>aivo</h2>
        <p style={{ margin: "0 0 14px", font: "var(--weight-regular) 14px/1.5 var(--font-sans)", color: "var(--ink-600)" }}>
          Waiter POS. Sign in with your staff account.
        </p>
        <input
          className="login-input"
          type="email"
          placeholder="Email"
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <input
          className="login-input"
          type="password"
          placeholder="Password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {err ? <div style={{ font: "var(--weight-regular) 13px/1.4 var(--font-sans)", color: "var(--red-700)" }}>{err}</div> : null}
        <button type="submit" className="btn btn-primary btn-touch btn-full" disabled={!email || !password || busy}>
          Sign in
        </button>
      </form>
    </div>
  );
}

function OpenShift({ pos, onOpened }: { pos: PosState; onOpened: () => void }) {
  const [float, setFloat] = useState("150.00");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const cents = parseDollars(float);
  const open = () => {
    if (cents === null || busy) return;
    setBusy(true);
    setErr("");
    api
      .openShift(cents)
      .then(onOpened)
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not open the shift.");
        setBusy(false);
      });
  };
  return (
    <div className="screen">
      <div className="screen-body" style={{ padding: "22px 18px" }}>
        <h2 style={{ margin: 0, font: "var(--weight-regular) 27px/1.15 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
          Open a shift
        </h2>
        <p style={{ margin: "8px 0 20px", font: "var(--weight-regular) 14px/1.5 var(--font-sans)", color: "var(--ink-600)" }}>
          Nothing can be ordered or tendered until a shift is open on your till.
        </p>
        <div className="card kv-card" style={{ marginBottom: 12 }}>
          <div className="kv-row" style={{ padding: "14px 0" }}>
            <span className="kv-key">Cashier</span>
            <span style={{ font: "var(--type-label)", color: "var(--ink-900)" }}>{pos.cashier}</span>
          </div>
          <div className="kv-row" style={{ padding: "14px 0" }}>
            <span className="kv-key">Till</span>
            <Badge>Till {pos.till}</Badge>
          </div>
          <div className="kv-row" style={{ padding: "10px 0", gap: 12 }}>
            <span className="kv-key" style={{ flex: "none" }}>Opening float</span>
            <input
              className="money-input"
              type="text"
              inputMode="decimal"
              placeholder="0.00"
              value={float}
              onChange={(e) => setFloat(e.target.value.replace(/[^0-9.]/g, "").slice(0, 9))}
            />
          </div>
        </div>
        {pos.other_till_shift ? (
          <div className="hint-card">
            <div className="hint-card-title">
              <Icon name="lock" size={15} />
              <span>Till {pos.other_till_shift.till} already has an open shift</span>
            </div>
            <div className="hint-card-body">
              {pos.other_till_shift.shift_number}, opened {pos.other_till_shift.opened_at} by {pos.other_till_shift.cashier}. One open shift per till and
              per cashier.
            </div>
          </div>
        ) : null}
        {err ? <div style={{ marginTop: 12, font: "var(--weight-regular) 13px/1.4 var(--font-sans)", color: "var(--red-700)" }}>{err}</div> : null}
      </div>
      <div className="screen-footer" style={{ padding: "12px 14px 16px" }}>
        <Button variant="primary" fullWidth disabled={cents === null || busy} onClick={open}>
          Open shift on till {pos.till}
        </Button>
      </div>
    </div>
  );
}

function Floor({
  pos,
  onTable,
  onRequests,
  onAddFromCode,
  onNewOrder,
  onClose,
}: {
  pos: PosState;
  onTable: (t: Table) => void;
  onRequests: () => void;
  onAddFromCode: () => void;
  onNewOrder: () => void;
  onClose: () => void;
}) {
  const reqCount = pos.requests.length;
  return (
    <div className="screen">
      <div
        className="screen-header"
        style={{ padding: "14px 18px 12px", display: "flex", alignItems: "center", justifyContent: "space-between" }}
      >
        <h2 style={{ margin: 0, font: "var(--weight-regular) 22px/1.1 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>Floor</h2>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <StatusPill status="open" label="shift open" dot />
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
      <div className="screen-body" style={{ padding: "14px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
        {pos.tables.map((t) => {
          const req = pos.requests.find((r) => r.table_id === t.id);
          const free = !t.ticket;
          const called = req?.kind === "waiter";
          const bill = req?.kind === "bill";
          const lines = t.ticket?.lines ?? [];
          const total = ticketTotal(t);
          let title = "Ordering";
          let sub = t.covers ? `${t.covers} covers` : "";
          let icon = "";
          if (free) {
            title = "Free";
            sub = "";
          } else if (called) {
            title = `Waiter called · ${req.asked_at}`;
            sub = `${t.covers} covers · ${t.ticket?.placed_at ? `order sent ${t.ticket.placed_at} · ` : ""}${fmt(total)}`;
            icon = "bell-ring";
          } else if (bill) {
            title = `Bill requested · ${req.asked_at}`;
            sub = `${t.covers} covers · ${fmt(total)} open`;
            icon = "receipt";
          } else if (lines.length) {
            title = t.ticket?.fired_at ? "Fired · eating" : "Order to fire";
            sub = `${t.covers} covers · ${fmt(total)} open`;
          } else {
            sub = `${t.covers} covers · nothing fired yet`;
          }
          return (
            <div key={t.id} className={`table-card${free ? " free" : ""}${called ? " called" : ""}`} onClick={() => onTable(t)}>
              <span className="table-num">{t.number}</span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="table-card-title">{title}</div>
                {sub ? <div className="table-card-sub">{sub}</div> : null}
              </div>
              {icon ? (
                <Icon name={icon} size={18} style={{ color: called ? "var(--orange-700)" : "var(--ink-500)" }} />
              ) : null}
            </div>
          );
        })}
      </div>
      <div className="screen-footer footer-grid">
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
          <Button fullWidth iconLeft="bell" onClick={onRequests}>
            {reqCount ? `Requests · ${reqCount}` : "Requests"}
          </Button>
          <Button fullWidth iconLeft="qr-code" onClick={onAddFromCode}>
            Add from code
          </Button>
        </div>
        <Button variant="primary" fullWidth onClick={onNewOrder}>
          New order
        </Button>
      </div>
    </div>
  );
}

function Ticket({ table, onBack, onAdd, onFire }: { table: Table; onBack: () => void; onAdd: () => void; onFire: () => void }) {
  const tk = table.ticket;
  const lines = tk?.lines ?? [];
  const fired = !!tk?.fired_at;
  const total = ticketTotal(table);
  return (
    <div className="screen">
      <div className="back-row">
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onBack}>
          Floor
        </Button>
      </div>
      <div className="screen-header" style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <div>
          <div className="screen-title">Table {table.number}</div>
          <div className="screen-sub">{tk?.source ?? ""}</div>
        </div>
        <StatusPill status={fired ? "open" : "closed"} label={fired ? "fired" : "to fire"} />
      </div>
      <div className="screen-body" style={{ padding: "14px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
        {lines.length === 0 ? (
          <div style={{ flex: 1, display: "grid", placeItems: "center", padding: 20 }}>
            <EmptyState
              icon="utensils"
              title="Nothing ordered yet"
              message="Take the order here, or wait for the diner order to arrive from their phone."
            />
          </div>
        ) : (
          <>
            <div className="card" style={{ padding: "4px 14px" }}>
              {lines.map((l, ix) => (
                <div key={l.id} style={{ padding: "12px 0", borderBottom: ix === lines.length - 1 ? "none" : "1px dashed var(--border-default)" }}>
                  <div style={{ display: "flex", justifyContent: "space-between", gap: 10 }}>
                    <span style={{ font: "600 15px/1.2 var(--font-sans)", color: "var(--ink-900)" }}>
                      {l.qty} × {l.name}
                    </span>
                    <span className="aivo-num" style={{ color: "var(--ink-900)" }}>{fmt(l.unit_price_cents * l.qty)}</span>
                  </div>
                  {l.options.length ? (
                    <div style={{ font: "var(--weight-regular) 13px/1.45 var(--font-sans)", color: "var(--ink-500)", marginTop: 3 }}>
                      {l.options.join(" · ")}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
            {tk?.note ? (
              <div style={{ background: "var(--yellow-100)", border: "1px solid var(--yellow-200)", borderRadius: 10, padding: "13px 15px" }}>
                <div
                  style={{
                    font: "600 12px/1.2 var(--font-sans)",
                    letterSpacing: "0.06em",
                    textTransform: "uppercase",
                    color: "var(--yellow-800)",
                    marginBottom: 5,
                  }}
                >
                  Note from the table
                </div>
                <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-800)" }}>{tk.note}</div>
              </div>
            ) : null}
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "2px 4px" }}>
              <span style={{ font: "var(--type-label)", color: "var(--ink-600)" }}>Open on this table</span>
              <span className="aivo-num" style={{ font: "600 18px/1.3 var(--font-mono)", color: "var(--ink-900)" }}>{fmt(total)}</span>
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
              <Button fullWidth iconLeft="plus" onClick={onAdd}>
                Add item
              </Button>
              <Button fullWidth iconLeft="split">
                Split
              </Button>
            </div>
            {fired ? (
              <div className="hint-card" style={{ padding: "13px 15px" }}>
                <span className="hint-card-body">
                  Fired to the kitchen at <span className="aivo-num">{tk?.fired_at}</span>. New items fire separately.
                </span>
              </div>
            ) : null}
          </>
        )}
      </div>
      {lines.length === 0 ? (
        <div className="screen-footer">
          <Button variant="primary" fullWidth iconLeft="plus" onClick={onAdd}>
            Take the order
          </Button>
        </div>
      ) : (
        <div className="screen-footer footer-grid" style={{ gridTemplateColumns: "1fr 1.3fr" }}>
          <Button fullWidth>Tender</Button>
          <Button variant="primary" fullWidth iconLeft="flame" disabled={fired} onClick={onFire}>
            {fired ? "Fired" : "Fire to kitchen"}
          </Button>
        </div>
      )}
    </div>
  );
}

function Requests({
  pos,
  now,
  onBack,
  onDismiss,
  onAck,
  onTakeBill,
}: {
  pos: PosState;
  now: number;
  onBack: () => void;
  onDismiss: (r: PosRequest) => void;
  onAck: (r: PosRequest) => void;
  onTakeBill: (r: PosRequest) => void;
}) {
  const waiter = pos.requests.filter((r) => r.kind === "waiter");
  const bills = pos.requests.filter((r) => r.kind === "bill");
  return (
    <div className="screen">
      <div className="back-row">
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onBack}>
          Floor
        </Button>
      </div>
      <div className="screen-header">
        <div className="screen-title">Requests</div>
        <div className="screen-sub">Also pushed to the Telegram group. Clearing here clears it for everyone.</div>
      </div>
      <div className="screen-body" style={{ padding: "14px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
        {pos.requests.length === 0 ? (
          <div style={{ flex: 1, display: "grid", placeItems: "center", padding: 20 }}>
            <EmptyState icon="bell" title="No open requests" message="Diner calls and bill requests from table phones land here." />
          </div>
        ) : null}
        {waiter.map((r) => (
          <div key={r.id} className="warn-card" style={{ padding: "15px 16px" }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
              <span style={{ font: "600 15px/1.2 var(--font-sans)", color: "var(--ink-900)" }}>Table {r.table_number} · waiter</span>
              <span className="aivo-num" style={{ fontSize: 12, color: "var(--orange-700)" }}>{waiting(r.created_at, now)}</span>
            </div>
            <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-600)", marginBottom: 12 }}>
              Asked at {r.asked_at}. No reason given.
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
              <Button fullWidth onClick={() => onDismiss(r)}>
                Dismiss
              </Button>
              <Button variant="primary" fullWidth onClick={() => onAck(r)}>
                On my way
              </Button>
            </div>
          </div>
        ))}
        {bills.map((r) => (
          <div key={r.id} className="card" style={{ padding: "15px 16px" }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
              <span style={{ font: "600 15px/1.2 var(--font-sans)", color: "var(--ink-900)" }}>Table {r.table_number} · bill</span>
              <span className="aivo-num" style={{ fontSize: 12, color: "var(--ink-500)" }}>{waiting(r.created_at, now)}</span>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
              <span style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-600)" }}>Open total</span>
              <span className="aivo-num" style={{ font: "600 15px/1.3 var(--font-mono)", color: "var(--ink-900)" }}>
                {fmt(r.open_total_cents ?? 0)}
              </span>
            </div>
            <Button variant="primary" fullWidth iconLeft="receipt" onClick={() => onTakeBill(r)}>
              Print and take over
            </Button>
          </div>
        ))}
        <div
          style={{
            background: "var(--paper-1)",
            border: "1px dashed var(--border-strong)",
            borderRadius: 10,
            padding: "13px 15px",
            font: "var(--weight-regular) 13px/1.5 var(--font-sans)",
            color: "var(--ink-500)",
          }}
        >
          One open request per table. A second tap from the same table does not create a second card.
        </div>
      </div>
    </div>
  );
}

function PickTable({ pos, onBack, onPick }: { pos: PosState; onBack: () => void; onPick: (t: Table) => void }) {
  return (
    <div className="screen">
      <div className="back-row">
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onBack}>
          Floor
        </Button>
      </div>
      <div className="screen-header">
        <div className="screen-title">New order</div>
        <div className="screen-sub">Pick a table. A table with an open ticket gets the new items added to it.</div>
      </div>
      <div
        className="screen-body"
        style={{ padding: "14px 16px", display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10, alignContent: "start" }}
      >
        {pos.tables.map((t) => {
          const free = !t.ticket;
          const open = ticketTotal(t);
          return (
            <div key={t.id} className={`pick-tile${free ? " free" : ""}`} onClick={() => onPick(t)}>
              <span className="aivo-num pick-num">{t.number}</span>
              <div style={{ font: "600 13px/1.2 var(--font-sans)", color: "var(--ink-900)" }}>
                {free ? "Free" : open ? "Open ticket" : "Seated"}
              </div>
              <div style={{ font: "var(--weight-regular) 12px/1.4 var(--font-sans)", color: "var(--ink-500)" }}>
                {free ? "start a new ticket" : open ? `${fmt(open)} · adds to it` : `${t.covers} covers · nothing yet`}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function TakeOrder({
  pos,
  table,
  onCancel,
  onCommit,
}: {
  pos: PosState;
  table: Table;
  onCancel: () => void;
  onCommit: (lines: NewLine[]) => void;
}) {
  const [cat, setCat] = useState(0);
  const [draft, setDraft] = useState<{ itemId: string; qty: number; mod: string | null }[]>([]);
  const items = pos.menu.flatMap((c) => c.items);
  const priceOf = (id: string) => items.find((i) => i.id === id)?.price_cents ?? 0;
  const draftTotal = draft.reduce((a, l) => a + priceOf(l.itemId) * l.qty, 0);
  const draftCount = draft.reduce((a, l) => a + l.qty, 0);
  const existing = ticketTotal(table);
  const category = pos.menu[cat] ?? pos.menu[0];
  return (
    <div className="screen">
      <div className="back-row">
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onCancel}>
          Back
        </Button>
      </div>
      <div
        style={{
          flex: "none",
          padding: "8px 18px 10px",
          background: "var(--paper-0)",
          display: "flex",
          alignItems: "baseline",
          justifyContent: "space-between",
        }}
      >
        <div className="screen-title">Table {table.number}</div>
        <span className="screen-sub" style={{ marginTop: 0 }}>
          {existing ? `on the ticket · ${fmt(existing)}` : "tap to add · tap − to remove"}
        </span>
      </div>
      <div
        style={{
          flex: "none",
          display: "flex",
          gap: 6,
          padding: "0 16px 12px",
          background: "var(--paper-0)",
          borderBottom: "1px solid var(--border-default)",
          overflowX: "auto",
        }}
      >
        {pos.menu.map((c, i) => (
          <span key={c.id} className={`cat-chip${i === cat ? " active" : ""}`} onClick={() => setCat(i)}>
            {c.name}
          </span>
        ))}
      </div>
      <div className="screen-body" style={{ padding: "10px 16px 14px", display: "flex", flexDirection: "column", gap: 8 }}>
        {(category?.items ?? []).map((it) => {
          const li = draft.findIndex((l) => l.itemId === it.id);
          const line = li >= 0 ? draft[li] : null;
          return (
            <div key={it.id} className={`order-row${line ? " in-draft" : ""}`}>
              <div
                style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 44 }}
                onClick={() =>
                  setDraft(
                    line
                      ? draft.map((l, x) => (x === li ? { ...l, qty: l.qty + 1 } : l))
                      : [...draft, { itemId: it.id, qty: 1, mod: it.mods ? "Medium rare" : null }]
                  )
                }
              >
                <span style={{ flex: 1, font: "600 15px/1.25 var(--font-sans)", color: "var(--ink-900)" }}>{it.name}</span>
                <span className="aivo-num" style={{ color: "var(--ink-700)" }}>{fmt(it.price_cents)}</span>
                {line ? <span className="qty-pill aivo-num">{line.qty}</span> : null}
              </div>
              {line ? (
                <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap", padding: "2px 0 8px" }}>
                  <span
                    className="minus-btn"
                    onClick={() =>
                      setDraft(line.qty > 1 ? draft.map((l, x) => (x === li ? { ...l, qty: l.qty - 1 } : l)) : draft.filter((_, x) => x !== li))
                    }
                  >
                    −
                  </span>
                  {(it.mods ?? []).map((m) => (
                    <span
                      key={m}
                      className={`mod-chip${line.mod === m ? " active" : ""}`}
                      onClick={() => setDraft(draft.map((l, x) => (x === li ? { ...l, mod: m } : l)))}
                    >
                      {m.toLowerCase()}
                    </span>
                  ))}
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
      <div className="screen-footer footer-grid" style={{ gridTemplateColumns: "1fr 1.6fr" }}>
        <Button fullWidth onClick={onCancel}>
          Cancel
        </Button>
        <Button
          variant="primary"
          fullWidth
          disabled={draftCount === 0}
          onClick={() =>
            onCommit(
              draft.map((l) => ({
                menu_item_id: l.itemId,
                qty: l.qty,
                options: l.mod ? [l.mod.toLowerCase()] : [],
              }))
            )
          }
        >
          {draftCount ? `Add to ticket · ${fmt(draftTotal)}` : "Add to ticket"}
        </Button>
      </div>
    </div>
  );
}

function Handoff({
  pos,
  onBack,
  onAccept,
}: {
  pos: PosState;
  onBack: () => void;
  onAccept: (preview: HandoffPreview, tableId: string) => void;
}) {
  const [code, setCode] = useState("");
  const [preview, setPreview] = useState<HandoffPreview | null>(null);
  const [targetId, setTargetId] = useState<string | null>(null);
  const [picking, setPicking] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const lookup = () => {
    if (code.length !== 6 || busy) return;
    setBusy(true);
    setErr("");
    api
      .handoff(code)
      .then((h) => {
        setPreview(h);
        setTargetId(h.table_id);
        setBusy(false);
      })
      .catch((e: { status?: number; message?: string }) => {
        setErr(e.status === 404 ? "Code not found. Codes expire after 15 minutes and work once." : (e.message ?? "Could not look up the code."));
        setBusy(false);
      });
  };

  const target = pos.tables.find((t) => t.id === targetId);
  const total = preview ? preview.lines.reduce((a, l) => a + l.unit_price_cents * l.qty, 0) : 0;

  if (preview && picking) {
    return (
      <div className="screen">
        <div className="back-row">
          <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={() => setPicking(false)}>
            Back
          </Button>
        </div>
        <div className="screen-header">
          <div className="screen-title">Add to which table?</div>
          <div className="screen-sub">The diner sat at table {preview.table_number}. Pick another only if they moved.</div>
        </div>
        <div
          className="screen-body"
          style={{ padding: "14px 16px", display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10, alignContent: "start" }}
        >
          {pos.tables.map((t) => {
            const free = !t.ticket;
            const open = ticketTotal(t);
            return (
              <div
                key={t.id}
                className={`pick-tile${free ? " free" : ""}`}
                style={t.id === targetId ? { borderColor: "var(--accent-solid)", borderStyle: "solid" } : undefined}
                onClick={() => {
                  setTargetId(t.id);
                  setPicking(false);
                }}
              >
                <span className="aivo-num pick-num">{t.number}</span>
                <div style={{ font: "600 13px/1.2 var(--font-sans)", color: "var(--ink-900)" }}>
                  {free ? "Free" : open ? "Open ticket" : "Seated"}
                </div>
                <div style={{ font: "var(--weight-regular) 12px/1.4 var(--font-sans)", color: "var(--ink-500)" }}>
                  {free ? "start a new ticket" : open ? `${fmt(open)} · adds to it` : `${t.covers} covers · nothing yet`}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    );
  }

  return (
    <div className="screen">
      <div className="back-row">
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onBack}>
          Floor
        </Button>
      </div>
      <div className="screen-header">
        <div className="screen-title">Add from code</div>
        <div className="screen-sub">Type the code from the diner's phone. Their cart lands on a table ticket.</div>
      </div>
      <div className="screen-body" style={{ padding: "14px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            lookup();
          }}
        >
          <input
            className="code-input"
            type="text"
            placeholder="CODE"
            autoCapitalize="characters"
            autoComplete="off"
            spellCheck={false}
            maxLength={6}
            value={code}
            onChange={(e) => {
              setCode(e.target.value.toUpperCase().replace(/[^A-Z2-9]/g, "").slice(0, 6));
              setPreview(null);
              setErr("");
            }}
          />
        </form>
        {err ? (
          <div className="hint-card" style={{ padding: "13px 15px" }}>
            <span className="hint-card-body">{err}</span>
          </div>
        ) : null}
        {preview ? (
          <>
            <div className="card" style={{ padding: "4px 14px" }}>
              <div style={{ padding: "12px 0", borderBottom: "1px dashed var(--border-default)" }}>
                <div style={{ font: "600 15px/1.2 var(--font-sans)", color: "var(--ink-900)" }}>Table {preview.table_number}</div>
                <div style={{ font: "var(--weight-regular) 12px/1.4 var(--font-sans)", color: "var(--ink-500)", marginTop: 2 }}>
                  {preview.customer_name ? `${preview.customer_name} · from the diner's phone` : "from the diner's phone"}
                </div>
              </div>
              {preview.lines.map((l, ix) => (
                <div
                  key={l.id}
                  style={{ padding: "12px 0", borderBottom: ix === preview.lines.length - 1 ? "none" : "1px dashed var(--border-default)" }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", gap: 10 }}>
                    <span style={{ font: "600 15px/1.2 var(--font-sans)", color: "var(--ink-900)" }}>
                      {l.qty} × {l.name}
                    </span>
                    <span className="aivo-num" style={{ color: "var(--ink-900)" }}>{fmt(l.unit_price_cents * l.qty)}</span>
                  </div>
                  {l.options.length ? (
                    <div style={{ font: "var(--weight-regular) 13px/1.45 var(--font-sans)", color: "var(--ink-500)", marginTop: 3 }}>
                      {l.options.join(" · ")}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
            {preview.note ? (
              <div style={{ background: "var(--yellow-100)", border: "1px solid var(--yellow-200)", borderRadius: 10, padding: "13px 15px" }}>
                <div
                  style={{
                    font: "600 12px/1.2 var(--font-sans)",
                    letterSpacing: "0.06em",
                    textTransform: "uppercase",
                    color: "var(--yellow-800)",
                    marginBottom: 5,
                  }}
                >
                  Note from the table
                </div>
                <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-800)" }}>{preview.note}</div>
              </div>
            ) : null}
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "2px 4px" }}>
              <span style={{ font: "var(--type-label)", color: "var(--ink-600)" }}>On the code</span>
              <span className="aivo-num" style={{ font: "600 18px/1.3 var(--font-mono)", color: "var(--ink-900)" }}>{fmt(total)}</span>
            </div>
          </>
        ) : null}
      </div>
      {preview && target ? (
        <div className="screen-footer footer-grid" style={{ gridTemplateColumns: "1fr 1.3fr" }}>
          <Button fullWidth onClick={() => setPicking(true)}>
            Change table
          </Button>
          <Button variant="primary" fullWidth iconLeft="plus" onClick={() => onAccept(preview, target.id)}>
            Add to table {target.number}
          </Button>
        </div>
      ) : (
        <div className="screen-footer">
          <Button variant="primary" fullWidth disabled={code.length !== 6 || busy} onClick={lookup}>
            Look up code
          </Button>
        </div>
      )}
    </div>
  );
}

function VarianceValue({ variance }: { variance: number | null }) {
  return (
    <span
      className="aivo-num"
      style={{
        font: "600 14px/1.3 var(--font-mono)",
        color: variance === null ? "var(--ink-400)" : variance === 0 ? "var(--green-700)" : "var(--red-700)",
      }}
    >
      {variance === null ? "—" : fmt(variance)}
    </span>
  );
}

function CloseShift({ pos, onBack, onPosted }: { pos: PosState; onBack: () => void; onPosted: (p: PostedShift) => void }) {
  const [declared, setDeclared] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const shift = pos.shift!;
  const declaredCents = parseDollars(declared);
  const variance = declaredCents === null ? null : declaredCents - shift.expected_cents;
  const needsManager = variance !== null && Math.abs(variance) > 1000;
  const accept = () => {
    if (declaredCents === null || busy) return;
    setBusy(true);
    setErr("");
    api
      .closeShift(shift.id, declaredCents)
      .then(onPosted)
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not post the shift.");
        setBusy(false);
      });
  };
  return (
    <div className="screen">
      <div className="back-row">
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onBack}>
          Floor
        </Button>
      </div>
      <div className="screen-body" style={{ padding: "14px 18px" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 14 }}>
          <h2 style={{ margin: 0, font: "var(--weight-regular) 24px/1.1 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
            Close shift
          </h2>
          <StatusPill status="closed" label="cash frozen" />
        </div>
        <div className="card kv-card" style={{ marginBottom: 12 }}>
          <div className="kv-row">
            <span className="kv-key">Expected cash</span>
            <span className="aivo-num" style={{ font: "500 14px/1.3 var(--font-mono)", color: "var(--ink-900)" }}>{fmt(shift.expected_cents)}</span>
          </div>
          <div className="kv-row" style={{ padding: "10px 0", gap: 12 }}>
            <span className="kv-key" style={{ flex: "none" }}>Declared cash</span>
            <input
              className="money-input"
              type="text"
              inputMode="decimal"
              placeholder="0.00"
              value={declared}
              onChange={(e) => setDeclared(e.target.value.replace(/[^0-9.]/g, "").slice(0, 9))}
            />
          </div>
          <div className="kv-row">
            <span className="kv-key-strong">Variance</span>
            <VarianceValue variance={variance} />
          </div>
        </div>
        {needsManager ? (
          <div className="warn-card" style={{ marginBottom: 12 }}>
            <div className="hint-card-title" style={{ color: "var(--orange-700)" }}>
              <Icon name="triangle-alert" size={15} />
              <span style={{ color: "var(--orange-700)" }}>Variance over $10.00</span>
            </div>
            <div className="hint-card-body">A manager accepts this posting. Recount first — most variances are a miscounted float.</div>
          </div>
        ) : null}
        <div className="hint-card">
          <div className="hint-card-title">
            <Icon name="lock" size={15} />
            <span>Posting is immutable</span>
          </div>
          <div className="hint-card-body">An accepted shift cannot be cancelled or reopened. A manager accepts variances over $10.00.</div>
        </div>
        {err ? <div style={{ marginTop: 12, font: "var(--weight-regular) 13px/1.4 var(--font-sans)", color: "var(--red-700)" }}>{err}</div> : null}
      </div>
      <div className="screen-footer footer-grid" style={{ gridTemplateColumns: "1fr 1.3fr" }}>
        <Button fullWidth onClick={() => setDeclared("")}>
          Recount
        </Button>
        <Button variant="primary" fullWidth iconLeft="check" disabled={declaredCents === null || busy} onClick={accept}>
          Accept and post
        </Button>
      </div>
    </div>
  );
}
