import {
  ChevronDown,
  ChevronUp,
  Pencil,
  Plus,
  Trash2,
  UtensilsCrossed,
} from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { Category, MenuItem } from "../api/types";
import { useRestaurant } from "../auth";
import { formatCents } from "../lib/money";
import { useLoad } from "../lib/useLoad";
import {
  Badge,
  EmptyState,
  ErrorBanner,
  Field,
  LoadingPage,
  Modal,
  Switch,
} from "../ui";
import ItemEditor from "./ItemEditor";

export default function Menu() {
  const restaurant = useRestaurant();
  const { data, setData, error, loading, reload } = useLoad(
    async () => {
      const [categories, items] = await Promise.all([
        api.listCategories(restaurant.id),
        api.listItems(restaurant.id),
      ]);
      return { categories, items };
    },
    [restaurant.id],
  );
  const [selectedCat, setSelectedCat] = useState<string | null>(null);
  const [editing, setEditing] = useState<MenuItem | null | "new">(null);
  const [catModal, setCatModal] = useState<Category | null | "new">(null);
  const [confirmDelete, setConfirmDelete] = useState<Category | MenuItem | null>(
    null,
  );
  const [actionError, setActionError] = useState<string | null>(null);

  if (loading) return <LoadingPage />;
  if (error || !data)
    return (
      <div className="content">
        <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />
      </div>
    );

  const cats = [...data.categories].sort((a, b) => a.position - b.position);
  const activeCat =
    cats.find((c) => c.id === selectedCat) ?? cats[0] ?? null;
  const items = activeCat
    ? data.items.filter((i) => i.category_id === activeCat.id)
    : [];

  async function moveCat(cat: Category, dir: -1 | 1) {
    const idx = cats.findIndex((c) => c.id === cat.id);
    const other = cats[idx + dir];
    if (!other) return;
    setActionError(null);
    // Optimistic swap; positions persist via two PATCHes.
    const swapped = data!.categories.map((c) =>
      c.id === cat.id
        ? { ...c, position: other.position }
        : c.id === other.id
          ? { ...c, position: cat.position }
          : c,
    );
    setData({ ...data!, categories: swapped });
    try {
      await Promise.all([
        api.updateCategory(restaurant.id, cat.id, { position: other.position }),
        api.updateCategory(restaurant.id, other.id, { position: cat.position }),
      ]);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Reorder failed.");
      reload();
    }
  }

  async function toggleAvailable(item: MenuItem) {
    setActionError(null);
    const patched = { ...item, available: !item.available };
    setData({
      ...data!,
      items: data!.items.map((i) => (i.id === item.id ? patched : i)),
    });
    try {
      await api.updateItem(restaurant.id, item.id, {
        available: patched.available,
      });
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Update failed.");
      reload();
    }
  }

  async function doDelete() {
    if (!confirmDelete) return;
    setActionError(null);
    try {
      if ("position" in confirmDelete) {
        await api.deleteCategory(restaurant.id, confirmDelete.id);
      } else {
        await api.deleteItem(restaurant.id, confirmDelete.id);
      }
      setConfirmDelete(null);
      reload();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Delete failed.");
      setConfirmDelete(null);
    }
  }

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Menu</h1>
          <p className="page-sub">
            Categories, items, options. Changes go live on the diner menu
            immediately.
          </p>
        </div>
        <button
          className="btn btn-primary"
          disabled={!activeCat}
          onClick={() => setEditing("new")}
        >
          <Plus size={15} />
          New item
        </button>
      </div>

      {actionError && (
        <div style={{ marginBottom: "var(--gap-stack)" }}>
          <ErrorBanner message={actionError} />
        </div>
      )}

      <div className="menu-layout">
        <div className="card card-tight">
          <div className="row-between" style={{ padding: "4px 6px 10px" }}>
            <span className="aivo-eyebrow">Categories</span>
            <button
              className="btn btn-ghost btn-icon"
              aria-label="Add category"
              onClick={() => setCatModal("new")}
            >
              <Plus size={14} />
            </button>
          </div>
          {cats.length === 0 && (
            <p
              style={{
                font: "var(--type-body)",
                color: "var(--text-muted)",
                padding: "0 6px 8px",
              }}
            >
              No categories yet. Add one to start the menu.
            </p>
          )}
          {cats.map((c, i) => (
            <div
              key={c.id}
              className={
                "cat-row" + (activeCat?.id === c.id ? " active" : "")
              }
              onClick={() => setSelectedCat(c.id)}
            >
              <span style={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis" }}>
                {c.name}
              </span>
              <span
                className="num"
                style={{ fontSize: 11, color: "var(--text-subtle)" }}
              >
                {data.items.filter((x) => x.category_id === c.id).length}
              </span>
              <span className="cat-actions">
                <button
                  className="btn btn-ghost btn-icon"
                  aria-label="Move up"
                  disabled={i === 0}
                  onClick={(e) => {
                    e.stopPropagation();
                    moveCat(c, -1);
                  }}
                >
                  <ChevronUp size={13} />
                </button>
                <button
                  className="btn btn-ghost btn-icon"
                  aria-label="Move down"
                  disabled={i === cats.length - 1}
                  onClick={(e) => {
                    e.stopPropagation();
                    moveCat(c, 1);
                  }}
                >
                  <ChevronDown size={13} />
                </button>
                <button
                  className="btn btn-ghost btn-icon"
                  aria-label="Rename"
                  onClick={(e) => {
                    e.stopPropagation();
                    setCatModal(c);
                  }}
                >
                  <Pencil size={13} />
                </button>
              </span>
            </div>
          ))}
        </div>

        <div className="stack">
          {!activeCat ? (
            <div className="card">
              <EmptyState
                icon={UtensilsCrossed}
                title="Start your menu"
                message="Create a category — Starters, From the grill, Wine — then add items to it."
                action={
                  <button
                    className="btn btn-primary btn-sm"
                    onClick={() => setCatModal("new")}
                  >
                    <Plus size={14} />
                    Add category
                  </button>
                }
              />
            </div>
          ) : items.length === 0 ? (
            <div className="card">
              <EmptyState
                icon={UtensilsCrossed}
                title={`Nothing in ${activeCat.name} yet`}
                message="Add the first item. Photo, allergens and options are all optional — name and price are enough to go live."
                action={
                  <button
                    className="btn btn-primary btn-sm"
                    onClick={() => setEditing("new")}
                  >
                    <Plus size={14} />
                    New item
                  </button>
                }
              />
            </div>
          ) : (
            items.map((item) => (
              <div
                key={item.id}
                className={"card item-card" + (item.available ? "" : " item-86")}
              >
                <div className="item-thumb">
                  {item.image_url ? <img src={item.image_url} alt="" /> : "photo"}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="row-between">
                    <span
                      style={{
                        font: "600 15px/1.25 var(--font-sans)",
                        color: "var(--text-strong)",
                      }}
                    >
                      {item.name}
                    </span>
                    <span className="num">{formatCents(item.price_cents)}</span>
                  </div>
                  <p
                    style={{
                      font: "var(--weight-regular) 13px/1.45 var(--font-sans)",
                      color: "var(--text-muted)",
                      marginTop: 2,
                    }}
                  >
                    {item.description || "No description."}
                  </p>
                  <div
                    className="row"
                    style={{ marginTop: 8, flexWrap: "wrap", gap: 4 }}
                  >
                    {!item.available && <Badge tone="warn">86'd</Badge>}
                    {item.allergens.map((a) => (
                      <Badge key={a}>{a}</Badge>
                    ))}
                    {item.option_groups.length > 0 && (
                      <Badge tone="outline">
                        {item.option_groups.length}{" "}
                        {item.option_groups.length === 1
                          ? "option group"
                          : "option groups"}
                      </Badge>
                    )}
                  </div>
                </div>
                <div className="row" style={{ flex: "none" }}>
                  <Switch
                    checked={item.available}
                    onChange={() => toggleAvailable(item)}
                    label={`${item.name} available`}
                  />
                  <button
                    className="btn btn-ghost btn-icon"
                    aria-label="Edit"
                    onClick={() => setEditing(item)}
                  >
                    <Pencil size={15} />
                  </button>
                  <button
                    className="btn btn-ghost btn-icon"
                    aria-label="Delete"
                    onClick={() => setConfirmDelete(item)}
                  >
                    <Trash2 size={15} />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {editing !== null && activeCat && (
        <ItemEditor
          restaurantId={restaurant.id}
          categoryId={activeCat.id}
          item={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={(saved, created) => {
            setData({
              ...data,
              items: created
                ? [...data.items, saved]
                : data.items.map((i) => (i.id === saved.id ? saved : i)),
            });
            setEditing(null);
          }}
        />
      )}

      {catModal !== null && (
        <CategoryModal
          restaurantId={restaurant.id}
          category={catModal === "new" ? null : catModal}
          onDelete={
            catModal !== "new"
              ? () => {
                  setConfirmDelete(catModal);
                  setCatModal(null);
                }
              : undefined
          }
          onClose={() => setCatModal(null)}
          onSaved={(cat, created) => {
            setData({
              ...data,
              categories: created
                ? [...data.categories, cat]
                : data.categories.map((c) => (c.id === cat.id ? cat : c)),
            });
            setCatModal(null);
            if (created) setSelectedCat(cat.id);
          }}
        />
      )}

      {confirmDelete && (
        <Modal
          title={
            "position" in confirmDelete ? "Delete category" : "Delete item"
          }
          onClose={() => setConfirmDelete(null)}
          footer={
            <>
              <button
                className="btn btn-secondary"
                onClick={() => setConfirmDelete(null)}
              >
                Cancel
              </button>
              <button className="btn btn-danger" onClick={doDelete}>
                Delete
              </button>
            </>
          }
        >
          <p style={{ font: "var(--type-body)" }}>
            {"position" in confirmDelete
              ? `Delete "${confirmDelete.name}" and every item in it? Diners stop seeing them immediately. This can't be undone.`
              : `Delete "${confirmDelete.name}" from the menu? This can't be undone — if it's only sold out tonight, 86 it instead.`}
          </p>
        </Modal>
      )}
    </div>
  );
}

function CategoryModal(props: {
  restaurantId: string;
  category: Category | null;
  onSaved: (cat: Category, created: boolean) => void;
  onDelete?: () => void;
  onClose: () => void;
}) {
  const [name, setName] = useState(props.category?.name ?? "");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function save() {
    if (!name.trim()) {
      setError("Name is required.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      if (props.category) {
        const cat = await api.updateCategory(
          props.restaurantId,
          props.category.id,
          { name: name.trim() },
        );
        props.onSaved(cat, false);
      } else {
        const cat = await api.createCategory(props.restaurantId, {
          name: name.trim(),
        });
        props.onSaved(cat, true);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed.");
      setBusy(false);
    }
  }

  return (
    <Modal
      title={props.category ? "Rename category" : "New category"}
      onClose={props.onClose}
      footer={
        <>
          {props.onDelete && (
            <button
              className="btn btn-danger"
              style={{ marginRight: "auto" }}
              onClick={props.onDelete}
            >
              <Trash2 size={14} />
              Delete
            </button>
          )}
          <button className="btn btn-secondary" onClick={props.onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={save} disabled={busy}>
            {busy ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      <Field label="Name" error={error ?? undefined}>
        <input
          className="input"
          value={name}
          autoFocus
          aria-invalid={!!error}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") save();
          }}
        />
      </Field>
    </Modal>
  );
}
