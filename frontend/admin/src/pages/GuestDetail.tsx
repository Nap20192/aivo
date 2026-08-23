import { ArrowLeft, X } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { GuestDetail as Detail } from "../api/types";
import { useRestaurant } from "../auth";
import { formatCents } from "../../../design-system/shared/money";
import { useLoad } from "../lib/useLoad";
import { ErrorBanner, Field, LoadingPage } from "../ui";
import { fmtDate } from "./Guests";

export default function GuestDetail() {
  const restaurant = useRestaurant();
  const { customerId } = useParams<{ customerId: string }>();
  const { data, error, loading, reload } = useLoad(
    () => api.getGuest(restaurant.id, customerId!),
    [restaurant.id, customerId],
  );

  if (loading) return <LoadingPage />;
  if (error || !data)
    return (
      <div className="content">
        <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />
      </div>
    );

  return <Loaded key={data.customer.id} initial={data} restaurantId={restaurant.id} />;
}

function Loaded(props: { initial: Detail; restaurantId: string }) {
  const [guest, setGuest] = useState(props.initial);
  const [notes, setNotes] = useState(props.initial.notes);
  const [tags, setTags] = useState<string[]>(props.initial.tags);
  const [tagInput, setTagInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [savedFlash, setSavedFlash] = useState(false);

  const dirty = useMemo(
    () => notes !== guest.notes || JSON.stringify(tags) !== JSON.stringify(guest.tags),
    [notes, tags, guest],
  );

  function addTag() {
    const t = tagInput.trim().toLowerCase();
    if (t && !tags.includes(t)) setTags([...tags, t]);
    setTagInput("");
  }

  async function save() {
    setBusy(true);
    setSaveError(null);
    try {
      const updated = await api.patchGuest(props.restaurantId, guest.customer.id, {
        notes,
        tags,
      });
      setGuest(updated);
      setNotes(updated.notes);
      setTags(updated.tags);
      setSavedFlash(true);
      setTimeout(() => setSavedFlash(false), 2000);
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : "Save failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="content">
      <Link
        to="/guests"
        className="btn btn-ghost btn-sm"
        style={{ marginBottom: "var(--space-5)", borderBottom: "none" }}
      >
        <ArrowLeft size={14} />
        Guests
      </Link>
      <div className="page-head">
        <div>
          <h1 className="page-title">{guest.customer.name}</h1>
          <p className="page-sub">
            {guest.customer.email}
            {guest.customer.phone ? ` · ${guest.customer.phone}` : ""}
          </p>
        </div>
        <button className="btn btn-primary" disabled={!dirty || busy} onClick={save}>
          {busy ? "Saving…" : savedFlash ? "Saved" : "Save changes"}
        </button>
      </div>

      {saveError && (
        <div style={{ marginBottom: "var(--gap-stack)" }}>
          <ErrorBanner message={saveError} />
        </div>
      )}

      <div className="stack" style={{ maxWidth: 720 }}>
        <div className="stat-grid">
          <div className="card">
            <div className="stat-label">Visits</div>
            <div className="stat-value">{guest.visits}</div>
          </div>
          <div className="card">
            <div className="stat-label">Total spent</div>
            <div className="stat-value">{formatCents(guest.total_spent_cents)}</div>
          </div>
          <div className="card">
            <div className="stat-label">First seen</div>
            <div className="stat-value" style={{ fontSize: "var(--text-title-md)" }}>
              {fmtDate(guest.first_seen)}
            </div>
          </div>
          <div className="card">
            <div className="stat-label">Last seen</div>
            <div className="stat-value" style={{ fontSize: "var(--text-title-md)" }}>
              {fmtDate(guest.last_seen)}
            </div>
          </div>
        </div>

        <div className="card stack">
          <Field
            label="Notes"
            hint="Visible to managers and owners only — never to the guest."
          >
            <textarea
              className="textarea"
              rows={3}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </Field>
          <div className="field">
            <span className="field-label">Tags</span>
            <div className="row" style={{ flexWrap: "wrap", gap: 4 }}>
              {tags.map((t) => (
                <span key={t} className="attach-chip">
                  <span className="name">{t}</span>
                  <button
                    className="btn btn-ghost btn-icon"
                    style={{ width: 16, height: 16 }}
                    aria-label={`Remove tag ${t}`}
                    onClick={() => setTags(tags.filter((x) => x !== t))}
                  >
                    <X size={11} />
                  </button>
                </span>
              ))}
              <input
                className="input"
                style={{ width: 160, height: "var(--control-h-sm)" }}
                placeholder="Add tag, press Enter"
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addTag();
                  }
                }}
                onBlur={addTag}
              />
            </div>
          </div>
        </div>

        <div className="card" style={{ padding: 0 }}>
          <div style={{ padding: "var(--space-6) var(--pad-card) 0" }}>
            <h3>Order history</h3>
          </div>
          {guest.orders.length === 0 ? (
            <p
              style={{
                padding: "var(--space-5) var(--pad-card) var(--space-6)",
                font: "var(--type-body)",
                color: "var(--text-muted)",
              }}
            >
              No orders at this restaurant yet.
            </p>
          ) : (
            guest.orders.map((o, i) => (
              <div
                key={i}
                style={{
                  padding: "var(--space-5) var(--pad-card)",
                  borderTop: i === 0 ? "none" : "1px solid var(--border-subtle)",
                }}
              >
                <div className="row-between">
                  <span style={{ font: "var(--type-label)" }}>
                    {o.table_label}
                    <span style={{ color: "var(--text-muted)", fontWeight: 400 }}>
                      {" "}
                      · <span className="num" style={{ fontSize: 12 }}>{fmtDate(o.created_at)}</span>
                    </span>
                  </span>
                  <span className="num">{formatCents(o.total_cents)}</span>
                </div>
                <div
                  style={{
                    marginTop: 6,
                    display: "flex",
                    flexDirection: "column",
                    gap: 2,
                  }}
                >
                  {o.lines.map((l, li) => (
                    <div
                      key={li}
                      className="row-between"
                      style={{ font: "var(--type-body)", color: "var(--ink-600)" }}
                    >
                      <span>
                        {l.name}
                        {l.qty > 1 && (
                          <span className="num" style={{ fontSize: 12 }}>
                            {" "}
                            ×{l.qty}
                          </span>
                        )}
                      </span>
                      <span className="num" style={{ fontSize: 12 }}>
                        {formatCents(l.total_cents)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ))
          )}
        </div>

        <p style={{ font: "var(--weight-regular) var(--text-caption)/1.5 var(--font-sans)", color: "var(--text-subtle)" }}>
          Only orders placed at this restaurant are shown. Guest contact
          details are visible to managers and owners.
        </p>
      </div>
    </div>
  );
}
