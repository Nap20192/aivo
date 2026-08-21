import { ImagePlus, Plus, Trash2 } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../api/client";
import type { MenuItem, OptionGroup } from "../api/types";
import { EU_ALLERGENS } from "../lib/allergens";
import { formatCents, parseMoney } from "../lib/money";
import { ErrorBanner, Field, Modal, Switch } from "../ui";

interface Props {
  restaurantId: string;
  categoryId: string;
  item: MenuItem | null; // null = create
  onSaved: (item: MenuItem, created: boolean) => void;
  onClose: () => void;
}

let localId = 0;
function newId(prefix: string) {
  return `${prefix}-new-${++localId}`;
}

export default function ItemEditor(props: Props) {
  const { item } = props;
  const [name, setName] = useState(item?.name ?? "");
  const [description, setDescription] = useState(item?.description ?? "");
  const [price, setPrice] = useState(
    item ? (item.price_cents / 100).toFixed(2) : "",
  );
  const [imageUrl, setImageUrl] = useState(item?.image_url ?? "");
  const [allergens, setAllergens] = useState<string[]>(item?.allergens ?? []);
  const [groups, setGroups] = useState<OptionGroup[]>(
    item?.option_groups.map((g) => ({
      ...g,
      choices: g.choices.map((c) => ({ ...c })),
    })) ?? [],
  );
  const [available, setAvailable] = useState(item?.available ?? true);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  function toggleAllergen(a: string) {
    setAllergens((cur) =>
      cur.includes(a) ? cur.filter((x) => x !== a) : [...cur, a],
    );
  }

  function patchGroup(gi: number, patch: Partial<OptionGroup>) {
    setGroups((gs) => gs.map((g, i) => (i === gi ? { ...g, ...patch } : g)));
  }

  async function upload(file: File) {
    setUploading(true);
    setSubmitError(null);
    try {
      const { url } = await api.uploadImage(props.restaurantId, file);
      setImageUrl(url);
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : "Upload failed.");
    } finally {
      setUploading(false);
    }
  }

  async function save() {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required.";
    const cents = parseMoney(price);
    if (cents === null) errs.price = "Enter a price like 12.50.";
    for (const g of groups) {
      if (!g.name.trim()) errs.groups = "Every option group needs a name.";
      if (g.choices.length === 0)
        errs.groups = "Every option group needs at least one choice.";
      for (const c of g.choices)
        if (!c.name.trim()) errs.groups = "Every choice needs a name.";
    }
    setErrors(errs);
    if (Object.keys(errs).length) return;

    setBusy(true);
    setSubmitError(null);
    const payload = {
      category_id: props.categoryId,
      name: name.trim(),
      description: description.trim(),
      price_cents: cents!,
      image_url: imageUrl,
      allergens,
      option_groups: groups,
      available,
    };
    try {
      if (item) {
        const saved = await api.updateItem(props.restaurantId, item.id, payload);
        props.onSaved(saved, false);
      } else {
        const saved = await api.createItem(props.restaurantId, payload);
        props.onSaved(saved, true);
      }
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : "Save failed.");
      setBusy(false);
    }
  }

  const previewCents = parseMoney(price);

  return (
    <Modal
      title={item ? "Edit item" : "New item"}
      wide
      onClose={props.onClose}
      footer={
        <>
          <button className="btn btn-secondary" onClick={props.onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={save} disabled={busy}>
            {busy ? "Saving…" : item ? "Save changes" : "Create item"}
          </button>
        </>
      }
    >
      <div className="stack">
        {submitError && <ErrorBanner message={submitError} />}

        <div style={{ display: "flex", gap: "var(--space-6)" }}>
          <div>
            <div className="item-thumb" style={{ width: 96, height: 96 }}>
              {imageUrl ? <img src={imageUrl} alt="" /> : "photo"}
            </div>
            <input
              ref={fileRef}
              type="file"
              accept="image/*"
              hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) upload(f);
                e.target.value = "";
              }}
            />
            <button
              className="btn btn-ghost btn-sm"
              style={{ marginTop: 6, width: "100%" }}
              onClick={() => fileRef.current?.click()}
              disabled={uploading}
            >
              <ImagePlus size={14} />
              {uploading ? "Uploading…" : imageUrl ? "Replace" : "Upload"}
            </button>
          </div>
          <div className="stack" style={{ flex: 1 }}>
            <Field label="Name" error={errors.name}>
              <input
                className="input"
                value={name}
                aria-invalid={!!errors.name}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <Field label="Description">
              <textarea
                className="textarea"
                rows={2}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </Field>
            <div className="form-grid">
              <Field
                label="Price"
                error={errors.price}
                hint={
                  previewCents !== null
                    ? `Stored as ${previewCents} cents · shows as ${formatCents(previewCents)}`
                    : undefined
                }
              >
                <input
                  className="input input-mono"
                  inputMode="decimal"
                  placeholder="12.50"
                  value={price}
                  aria-invalid={!!errors.price}
                  onChange={(e) => setPrice(e.target.value)}
                />
              </Field>
              <div className="field">
                <span className="field-label">Available</span>
                <div className="row" style={{ height: "var(--control-h-md)" }}>
                  <Switch
                    checked={available}
                    onChange={setAvailable}
                    label="Available"
                  />
                  <span
                    style={{
                      font: "var(--type-body)",
                      color: available ? "var(--ink-700)" : "var(--yellow-800)",
                    }}
                  >
                    {available ? "On the menu" : "86'd — shown as sold out"}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="field">
          <span className="field-label">Allergens (EU 14)</span>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(120px, 1fr))",
              gap: 6,
            }}
          >
            {EU_ALLERGENS.map((a) => (
              <label key={a} className="check-row">
                <input
                  type="checkbox"
                  checked={allergens.includes(a)}
                  onChange={() => toggleAllergen(a)}
                />
                {a}
              </label>
            ))}
          </div>
        </div>

        <div className="field">
          <div className="row-between">
            <span className="field-label">Option groups</span>
            <button
              className="btn btn-ghost btn-sm"
              onClick={() =>
                setGroups((gs) => [
                  ...gs,
                  {
                    id: newId("grp"),
                    name: "",
                    type: "single",
                    choices: [
                      { id: newId("ch"), name: "", price_delta_cents: 0 },
                    ],
                  },
                ])
              }
            >
              <Plus size={14} />
              Add group
            </button>
          </div>
          {errors.groups && <span className="field-error">{errors.groups}</span>}
          {groups.length === 0 && (
            <span className="field-hint">
              Optional. Sizes, doneness, sauces — single or multi-select, each
              choice with a price delta.
            </span>
          )}
          {groups.map((g, gi) => (
            <div key={g.id} className="card card-tight stack" style={{ gap: 8 }}>
              <div className="row">
                <input
                  className="input"
                  placeholder="Group name, e.g. Size"
                  value={g.name}
                  onChange={(e) => patchGroup(gi, { name: e.target.value })}
                  style={{ flex: 1 }}
                />
                <div className="seg">
                  <button
                    type="button"
                    className={g.type === "single" ? "on" : ""}
                    onClick={() => patchGroup(gi, { type: "single" })}
                  >
                    Pick one
                  </button>
                  <button
                    type="button"
                    className={g.type === "multi" ? "on" : ""}
                    onClick={() => patchGroup(gi, { type: "multi" })}
                  >
                    Any number
                  </button>
                </div>
                <button
                  className="btn btn-ghost btn-icon"
                  aria-label="Remove group"
                  onClick={() =>
                    setGroups((gs) => gs.filter((_, i) => i !== gi))
                  }
                >
                  <Trash2 size={14} />
                </button>
              </div>
              {g.choices.map((c, ci) => (
                <div key={c.id} className="row">
                  <input
                    className="input"
                    placeholder="Choice, e.g. 300 g"
                    value={c.name}
                    onChange={(e) =>
                      patchGroup(gi, {
                        choices: g.choices.map((x, i) =>
                          i === ci ? { ...x, name: e.target.value } : x,
                        ),
                      })
                    }
                    style={{ flex: 1 }}
                  />
                  <span
                    style={{ font: "var(--type-body)", color: "var(--text-muted)" }}
                  >
                    +
                  </span>
                  <input
                    className="input input-mono"
                    style={{ width: 90 }}
                    inputMode="decimal"
                    placeholder="0.00"
                    value={
                      c.price_delta_cents
                        ? (c.price_delta_cents / 100).toFixed(2)
                        : ""
                    }
                    onChange={(e) => {
                      const cents = parseMoney(e.target.value) ?? 0;
                      patchGroup(gi, {
                        choices: g.choices.map((x, i) =>
                          i === ci ? { ...x, price_delta_cents: cents } : x,
                        ),
                      });
                    }}
                  />
                  <button
                    className="btn btn-ghost btn-icon"
                    aria-label="Remove choice"
                    onClick={() =>
                      patchGroup(gi, {
                        choices: g.choices.filter((_, i) => i !== ci),
                      })
                    }
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
              <button
                className="btn btn-ghost btn-sm"
                style={{ alignSelf: "flex-start" }}
                onClick={() =>
                  patchGroup(gi, {
                    choices: [
                      ...g.choices,
                      { id: newId("ch"), name: "", price_delta_cents: 0 },
                    ],
                  })
                }
              >
                <Plus size={14} />
                Add choice
              </button>
            </div>
          ))}
        </div>
      </div>
    </Modal>
  );
}
