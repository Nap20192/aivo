import { Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import type { HoursRow, Restaurant } from "../api/types";
import { useAuth, useRestaurant } from "../auth";
import { Badge, ErrorBanner, Field } from "../ui";

export default function Settings() {
  const restaurant = useRestaurant();
  const { me, setMe } = useAuth();
  const [form, setForm] = useState<Restaurant>({ ...restaurant });
  const [savedForm, setSavedForm] = useState<Restaurant>({ ...restaurant });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saveError, setSaveError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [savedFlash, setSavedFlash] = useState(false);

  const dirty = useMemo(
    () => JSON.stringify(form) !== JSON.stringify(savedForm),
    [form, savedForm],
  );

  function patch(p: Partial<Restaurant>) {
    setForm((f) => ({ ...f, ...p }));
  }

  function patchHours(i: number, p: Partial<HoursRow>) {
    patch({
      hours: form.hours.map((h, hi) => (hi === i ? { ...h, ...p } : h)),
    });
  }

  async function save() {
    const errs: Record<string, string> = {};
    if (!form.name.trim()) errs.name = "Name is required.";
    if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(form.slug))
      errs.slug = "Lowercase letters, numbers and hyphens only.";
    if (
      form.custom_domain &&
      !/^[a-z0-9.-]+\.[a-z]{2,}$/i.test(form.custom_domain)
    )
      errs.custom_domain = "Enter a domain like menu.example.com.";
    for (const h of form.hours) {
      if (!h.label.trim() || !h.open || !h.close)
        errs.hours = "Every hours row needs a label, open and close time.";
    }
    setErrors(errs);
    if (Object.keys(errs).length) return;

    setBusy(true);
    setSaveError(null);
    try {
      const updated = await api.patchRestaurant(restaurant.id, form);
      setForm(updated);
      setSavedForm(updated);
      if (me)
        setMe({
          ...me,
          restaurants: me.restaurants.map((r) =>
            r.id === updated.id ? updated : r,
          ),
        });
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
      <div className="page-head">
        <div>
          <h1 className="page-title">Restaurant settings</h1>
          <p className="page-sub">
            Name, address and hours show on the diner landing page.
          </p>
        </div>
        <button
          className="btn btn-primary"
          disabled={!dirty || busy}
          onClick={save}
        >
          {busy ? "Saving…" : savedFlash ? "Saved" : "Save changes"}
        </button>
      </div>

      {saveError && (
        <div style={{ marginBottom: "var(--gap-stack)" }}>
          <ErrorBanner message={saveError} />
        </div>
      )}

      <div className="stack" style={{ maxWidth: 640 }}>
        <div className="card stack">
          <h3>Identity</h3>
          <div className="form-grid">
            <Field label="Restaurant name" error={errors.name}>
              <input
                className="input"
                value={form.name}
                aria-invalid={!!errors.name}
                onChange={(e) => patch({ name: e.target.value })}
              />
            </Field>
            <Field
              label="Slug"
              hint={`Menu lives at /${form.slug || "…"}`}
              error={errors.slug}
            >
              <input
                className="input input-mono"
                value={form.slug}
                aria-invalid={!!errors.slug}
                onChange={(e) => patch({ slug: e.target.value.toLowerCase() })}
              />
            </Field>
          </div>
          <Field
            label="Custom domain"
            hint="Point a CNAME at aivo, then save. Certificate automation lands later."
            error={errors.custom_domain}
          >
            <div className="row">
              <input
                className="input input-mono"
                placeholder="menu.example.com"
                value={form.custom_domain}
                aria-invalid={!!errors.custom_domain}
                onChange={(e) =>
                  patch({ custom_domain: e.target.value.trim().toLowerCase() })
                }
                style={{ flex: 1 }}
              />
              {savedForm.custom_domain && (
                <Badge tone="warn">Verification pending</Badge>
              )}
            </div>
          </Field>
        </div>

        <div className="card stack">
          <div className="row-between">
            <h3>Hours</h3>
            <button
              className="btn btn-ghost btn-sm"
              onClick={() =>
                patch({
                  hours: [...form.hours, { label: "", open: "", close: "" }],
                })
              }
            >
              <Plus size={14} />
              Add row
            </button>
          </div>
          {errors.hours && <span className="field-error">{errors.hours}</span>}
          {form.hours.length === 0 && (
            <span className="field-hint">
              No hours listed. Diners see the landing page without an "Open
              today" card.
            </span>
          )}
          {form.hours.map((h, i) => (
            <div className="row" key={i}>
              <input
                className="input"
                placeholder="Kitchen"
                value={h.label}
                onChange={(e) => patchHours(i, { label: e.target.value })}
                style={{ flex: 1 }}
              />
              <input
                className="input input-mono"
                type="time"
                value={h.open}
                onChange={(e) => patchHours(i, { open: e.target.value })}
                style={{ width: 120 }}
              />
              <span style={{ color: "var(--text-muted)" }}>–</span>
              <input
                className="input input-mono"
                type="time"
                value={h.close}
                onChange={(e) => patchHours(i, { close: e.target.value })}
                style={{ width: 120 }}
              />
              <button
                className="btn btn-ghost btn-icon"
                aria-label="Remove row"
                onClick={() =>
                  patch({ hours: form.hours.filter((_, hi) => hi !== i) })
                }
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
        </div>

        <div className="card stack">
          <h3>Contact</h3>
          <Field label="Address">
            <input
              className="input"
              value={form.address}
              onChange={(e) => patch({ address: e.target.value })}
            />
          </Field>
          <div className="form-grid">
            <Field label="Phone">
              <input
                className="input input-mono"
                value={form.phone}
                onChange={(e) => patch({ phone: e.target.value })}
              />
            </Field>
            <Field label="Instagram">
              <input
                className="input"
                placeholder="@handle"
                value={form.instagram}
                onChange={(e) => patch({ instagram: e.target.value })}
              />
            </Field>
          </div>
        </div>
      </div>
    </div>
  );
}
