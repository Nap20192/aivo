import {
  ChevronDown,
  ChevronUp,
  Link as LinkIcon,
  Pencil,
  Plus,
  Settings2,
  Trash2,
  UtensilsCrossed,
} from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { ApiError } from "../api/error";
import type { Category, Menu as MenuType, MenuItem } from "../api/types";
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

export function ItemsTab() {
  const restaurant = useRestaurant();
  const { data, setData, error, loading, reload } = useLoad(
    async () => {
      const [menus, categories, items] = await Promise.all([
        api.listMenus(restaurant.id),
        api.listCategories(restaurant.id),
        api.listItems(restaurant.id),
      ]);
      return { menus, categories, items };
    },
    [restaurant.id],
  );
  const [selectedMenu, setSelectedMenu] = useState<string | null>(null);
  const [menuModal, setMenuModal] = useState<MenuType | null | "new">(null);
  const [linkCopied, setLinkCopied] = useState(false);
  const [selectedCat, setSelectedCat] = useState<string | null>(null);
  const [editing, setEditing] = useState<MenuItem | null | "new">(null);
  const [catModal, setCatModal] = useState<Category | null | "new">(null);
  const [confirmDelete, setConfirmDelete] = useState<Category | MenuItem | null>(
    null,
  );
  const [actionError, setActionError] = useState<string | null>(null);

  if (loading) return <LoadingPage />;
  if (error || !data)
    return <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />;

  const menus = [...data.menus].sort((a, b) => a.position - b.position);
  const activeMenu =
    menus.find((m) => m.id === selectedMenu) ??
    menus.find((m) => m.is_default) ??
    menus[0] ??
    null;
  const cats = data.categories
    .filter((c) => activeMenu && c.menu_id === activeMenu.id)
    .sort((a, b) => a.position - b.position);
  const activeCat =
    cats.find((c) => c.id === selectedCat) ?? cats[0] ?? null;
  const menuLink = activeMenu
    ? `${location.origin}/${restaurant.slug}/m/${activeMenu.slug}`
    : "";
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
    <>
      {actionError && (
        <div style={{ marginBottom: "var(--gap-stack)" }}>
          <ErrorBanner message={actionError} />
        </div>
      )}

      <div
        className="row"
        style={{ flexWrap: "wrap", marginBottom: "var(--gap-stack)" }}
      >
        {menus.map((m) => (
          <button
            key={m.id}
            className={"chip" + (activeMenu?.id === m.id ? " on" : "")}
            onClick={() => {
              setSelectedMenu(m.id);
              setSelectedCat(null);
            }}
          >
            {m.name}
            {m.is_default && <span className="chip-note">default</span>}
          </button>
        ))}
        <button className="chip" onClick={() => setMenuModal("new")}>
          <Plus size={13} style={{ verticalAlign: "-2px", marginRight: 4 }} />
          New menu
        </button>
        {activeMenu && (
          <span className="row" style={{ marginLeft: "auto" }}>
            <button
              className="btn btn-ghost btn-sm"
              title={menuLink}
              onClick={() => {
                navigator.clipboard.writeText(menuLink);
                setLinkCopied(true);
                setTimeout(() => setLinkCopied(false), 2000);
              }}
            >
              <LinkIcon size={14} />
              {linkCopied ? "Link copied" : "Copy link"}
            </button>
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => setMenuModal(activeMenu)}
            >
              <Settings2 size={14} />
              Menu settings
            </button>
            <button
              className="btn btn-primary btn-sm"
              disabled={!activeCat}
              onClick={() => setEditing("new")}
            >
              <Plus size={14} />
              New item
            </button>
          </span>
        )}
      </div>

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

      {menuModal !== null && (
        <MenuModal
          restaurantId={restaurant.id}
          menu={menuModal === "new" ? null : menuModal}
          menuCount={menus.length}
          categoryCount={
            menuModal === "new"
              ? 0
              : data.categories.filter((c) => c.menu_id === menuModal.id).length
          }
          itemCount={
            menuModal === "new"
              ? 0
              : data.items.filter((i) =>
                  data.categories.some(
                    (c) => c.menu_id === menuModal.id && c.id === i.category_id,
                  ),
                ).length
          }
          onClose={() => setMenuModal(null)}
          onSaved={(menu, created) => {
            setData({
              ...data,
              menus: created
                ? [...data.menus, menu]
                : data.menus.map((m) =>
                    m.id === menu.id
                      ? menu
                      : menu.is_default
                        ? { ...m, is_default: false }
                        : m,
                  ),
            });
            setMenuModal(null);
            if (created) {
              setSelectedMenu(menu.id);
              setSelectedCat(null);
            }
          }}
          onDeleted={(menuId) => {
            const deletedCatIds = new Set(
              data.categories
                .filter((c) => c.menu_id === menuId)
                .map((c) => c.id),
            );
            setData({
              ...data,
              menus: data.menus.filter((m) => m.id !== menuId),
              categories: data.categories.filter((c) => c.menu_id !== menuId),
              items: data.items.filter((i) => !deletedCatIds.has(i.category_id)),
            });
            setMenuModal(null);
            setSelectedMenu(null);
            setSelectedCat(null);
          }}
        />
      )}

      {catModal !== null && activeMenu && (
        <CategoryModal
          restaurantId={restaurant.id}
          menuId={activeMenu.id}
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
    </>
  );
}

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/&/g, "and")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

function MenuModal(props: {
  restaurantId: string;
  menu: MenuType | null; // null = create
  menuCount: number;
  categoryCount: number;
  itemCount: number;
  onSaved: (menu: MenuType, created: boolean) => void;
  onDeleted: (menuId: string) => void;
  onClose: () => void;
}) {
  const { menu } = props;
  const [name, setName] = useState(menu?.name ?? "");
  const [slug, setSlug] = useState(menu?.slug ?? "");
  const [slugTouched, setSlugTouched] = useState(!!menu);
  const [makeDefault, setMakeDefault] = useState(menu?.is_default ?? false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  const nonEmpty = props.categoryCount > 0;
  const undeletable = !!menu && (menu.is_default || props.menuCount === 1);

  async function save() {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required.";
    if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(slug))
      errs.slug = "Lowercase letters, numbers and hyphens only.";
    setErrors(errs);
    if (Object.keys(errs).length) return;
    setBusy(true);
    setSubmitError(null);
    try {
      if (menu) {
        const patch: Partial<MenuType> = { name: name.trim(), slug };
        if (makeDefault && !menu.is_default) patch.is_default = true;
        const saved = await api.updateMenu(props.restaurantId, menu.id, patch);
        props.onSaved(saved, false);
      } else {
        let saved = await api.createMenu(props.restaurantId, {
          name: name.trim(),
          slug,
        });
        if (makeDefault)
          saved = await api.updateMenu(props.restaurantId, saved.id, {
            is_default: true,
          });
        props.onSaved(saved, true);
      }
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : "Save failed.");
      setBusy(false);
    }
  }

  async function doDelete() {
    if (!menu) return;
    setBusy(true);
    setSubmitError(null);
    try {
      await api.deleteMenu(props.restaurantId, menu.id, nonEmpty);
      props.onDeleted(menu.id);
    } catch (e) {
      if (e instanceof ApiError) setSubmitError(e.message);
      else setSubmitError(e instanceof Error ? e.message : "Delete failed.");
      setBusy(false);
      setConfirmingDelete(false);
    }
  }

  if (confirmingDelete && menu) {
    return (
      <Modal
        title="Delete menu"
        onClose={() => setConfirmingDelete(false)}
        footer={
          <>
            <button
              className="btn btn-secondary"
              onClick={() => setConfirmingDelete(false)}
            >
              Cancel
            </button>
            <button className="btn btn-danger" onClick={doDelete} disabled={busy}>
              {busy
                ? "Deleting…"
                : nonEmpty
                  ? "Delete menu and contents"
                  : "Delete menu"}
            </button>
          </>
        }
      >
        <div className="stack">
          {submitError && <ErrorBanner message={submitError} />}
          <p style={{ font: "var(--type-body)" }}>
            Delete <strong>{menu.name}</strong>? The shareable link /m/
            {menu.slug} stops working immediately.
          </p>
          {nonEmpty && (
            <div className="notice-banner">
              This menu still has{" "}
              <span className="num">{props.categoryCount}</span>{" "}
              {props.categoryCount === 1 ? "category" : "categories"} and{" "}
              <span className="num">{props.itemCount}</span>{" "}
              {props.itemCount === 1 ? "item" : "items"} — they are deleted with
              it. This can't be undone.
            </div>
          )}
        </div>
      </Modal>
    );
  }

  return (
    <Modal
      title={menu ? "Menu settings" : "New menu"}
      onClose={props.onClose}
      footer={
        <>
          {menu && (
            <button
              className="btn btn-danger"
              style={{ marginRight: "auto" }}
              disabled={undeletable}
              title={
                menu.is_default
                  ? "The default menu can't be deleted — make another menu the default first."
                  : props.menuCount === 1
                    ? "The last menu can't be deleted."
                    : undefined
              }
              onClick={() => setConfirmingDelete(true)}
            >
              <Trash2 size={14} />
              Delete
            </button>
          )}
          <button className="btn btn-secondary" onClick={props.onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={save} disabled={busy}>
            {busy ? "Saving…" : menu ? "Save" : "Create menu"}
          </button>
        </>
      }
    >
      <div className="stack">
        {submitError && <ErrorBanner message={submitError} />}
        <Field label="Name" error={errors.name}>
          <input
            className="input"
            value={name}
            autoFocus={!menu}
            aria-invalid={!!errors.name}
            placeholder="Lunch"
            onChange={(e) => {
              setName(e.target.value);
              if (!slugTouched) setSlug(slugify(e.target.value));
            }}
          />
        </Field>
        <Field
          label="Slug"
          hint={`Shareable at /m/${slug || "…"}`}
          error={errors.slug}
        >
          <input
            className="input input-mono"
            value={slug}
            aria-invalid={!!errors.slug}
            onChange={(e) => {
              setSlugTouched(true);
              setSlug(e.target.value.toLowerCase());
            }}
          />
        </Field>
        <div className="row">
          <Switch
            checked={makeDefault}
            onChange={setMakeDefault}
            label="Default menu"
            disabled={menu?.is_default ?? false}
          />
          <div>
            <div className="field-label">Default menu</div>
            <div className="field-hint">
              {menu?.is_default
                ? "This is the default — diners land on it. Set another menu as default to change it."
                : "Diners land on the default menu when they scan a table QR."}
            </div>
          </div>
        </div>
      </div>
    </Modal>
  );
}

function CategoryModal(props: {
  restaurantId: string;
  menuId: string;
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
          menu_id: props.menuId,
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
