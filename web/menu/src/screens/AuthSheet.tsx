import { useState, type CSSProperties } from "react";
import { ApiError, type Client } from "../api";
import type { Customer } from "../types";
import { Button, Icon, Input } from "../ui";

/** Bottom sheet with sign in / create account tabs. Optional — anonymous flow stays. */
export function AuthSheet({
  client,
  onDone,
  onClose,
}: {
  client: Client;
  onDone: (customer: Customer) => void;
  onClose: () => void;
}) {
  const [tab, setTab] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const customer =
        tab === "login"
          ? await client.login(email.trim(), password)
          : await client.register(email.trim(), password, name);
      onDone(customer);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Something went wrong. Try again.");
    } finally {
      setBusy(false);
    }
  }

  const pill = (selected: boolean): CSSProperties =>
    selected
      ? { flex: 1, textAlign: "center", padding: "8px 13px", borderRadius: 999, background: "var(--ink-900)", color: "var(--paper-0)", font: "var(--type-label)", cursor: "pointer" }
      : { flex: 1, textAlign: "center", padding: "8px 13px", borderRadius: 999, background: "var(--paper-2)", border: "1px solid var(--border-default)", color: "var(--ink-700)", font: "var(--type-label)", cursor: "pointer" };

  return (
    <div
      style={{ position: "absolute", inset: 0, background: "var(--scrim)", display: "flex", flexDirection: "column", justifyContent: "flex-end", zIndex: 10 }}
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{ background: "var(--paper-1)", borderRadius: "14px 14px 0 0", border: "1px solid var(--border-default)", borderBottom: "none", padding: "16px 18px 20px" }}
      >
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 14 }}>
          <h2 style={{ margin: 0, font: "var(--weight-regular) 22px/1.1 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
            Your account
          </h2>
          <span onClick={onClose} style={{ cursor: "pointer", color: "var(--ink-500)", display: "grid", placeItems: "center" }} aria-label="Close">
            <Icon name="x" size={18} />
          </span>
        </div>
        <p style={{ margin: "0 0 14px", font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)" }}>
          Keeps your order history across visits. Ordering works fine without one.
        </p>
        <div style={{ display: "flex", gap: 6, marginBottom: 14 }}>
          <span style={pill(tab === "login")} onClick={() => setTab("login")}>
            Sign in
          </span>
          <span style={pill(tab === "register")} onClick={() => setTab("register")}>
            Create account
          </span>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          {tab === "register" ? (
            <Input label="Name" value={name} onChange={setName} placeholder="How the waiter greets you" autoComplete="name" />
          ) : null}
          <Input label="Email" type="email" value={email} onChange={setEmail} placeholder="you@example.com" autoComplete="email" />
          <Input
            label="Password"
            type="password"
            value={password}
            onChange={setPassword}
            placeholder={tab === "register" ? "8+ characters" : ""}
            autoComplete={tab === "register" ? "new-password" : "current-password"}
          />
          {error ? (
            <div style={{ background: "var(--red-100)", border: "1px solid var(--red-200)", borderRadius: 10, padding: "12px 14px", font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--red-700)" }}>
              {error}
            </div>
          ) : null}
          <Button variant="primary" size="touch" fullWidth disabled={busy} onClick={submit}>
            {busy ? "One moment…" : tab === "login" ? "Sign in" : "Create account"}
          </Button>
        </div>
      </div>
    </div>
  );
}
