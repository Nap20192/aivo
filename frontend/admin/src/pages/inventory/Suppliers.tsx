import { Plus, Truck } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { Supplier } from "../../api/types";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { Badge, EmptyState, ErrorBanner, Field, LoadingPage, Modal } from "../../ui";

export default function Suppliers() {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listSuppliers(r.id), [r.id]);
  const [edit, setEdit] = useState<Supplier | null | undefined>(undefined); // null = new

  if (error) return <ErrorBanner message={error} onRetry={reload} />;
  if (loading || !data) return <LoadingPage />;

  const contactStr = (c: Record<string, string>) =>
    Object.entries(c).map(([k, v]) => `${k}: ${v}`).join(" · ") || "—";

  return (
    <div className="stack">
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn btn-primary" onClick={() => setEdit(null)}>
          <Plus size={16} /> New supplier
        </button>
      </div>
      {data.length === 0 ? (
        <div className="card">
          <EmptyState icon={Truck} title="No suppliers" message="Add the vendors you receive goods from." />
        </div>
      ) : (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Name</th>
                <th>Contacts</th>
                <th></th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {data.map((s) => (
                <tr key={s.id} style={s.archived ? { opacity: 0.55 } : undefined}>
                  <td style={{ font: "var(--type-label)" }}>{s.name}</td>
                  <td style={{ color: "var(--text-muted)" }}>{contactStr(s.contacts)}</td>
                  <td>{s.archived && <Badge tone="danger">archived</Badge>}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-ghost btn-sm" onClick={() => setEdit(s)}>
                      Edit
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {edit !== undefined && <SupplierModal supplier={edit} onClose={() => setEdit(undefined)} onSaved={reload} />}
    </div>
  );
}

function SupplierModal({ supplier, onClose, onSaved }: { supplier: Supplier | null; onClose: () => void; onSaved: () => void }) {
  const r = useRestaurant();
  const editing = !!supplier;
  const [name, setName] = useState(supplier?.name ?? "");
  const [phone, setPhone] = useState(supplier?.contacts.phone ?? "");
  const [email, setEmail] = useState(supplier?.contacts.email ?? "");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const save = () => {
    if (!name.trim()) {
      setErr("Name is required.");
      return;
    }
    setBusy(true);
    setErr(null);
    const contacts: Record<string, string> = {};
    if (phone.trim()) contacts.phone = phone.trim();
    if (email.trim()) contacts.email = email.trim();
    const done = () => {
      onSaved();
      onClose();
    };
    const fail = (e: { message?: string }) => {
      setErr(e.message ?? "Could not save.");
      setBusy(false);
    };
    if (editing) api.updateSupplier(r.id, supplier!.id, { name, contacts }).then(done).catch(fail);
    else api.createSupplier(r.id, { name, contacts }).then(done).catch(fail);
  };

  return (
    <Modal
      title={editing ? `Edit ${supplier!.name}` : "New supplier"}
      onClose={onClose}
      footer={
        <div className="row" style={{ justifyContent: "space-between", width: "100%" }}>
          {editing && (
            <button
              className="btn btn-ghost"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                api
                  .updateSupplier(r.id, supplier!.id, { archived: !supplier!.archived })
                  .then(() => {
                    onSaved();
                    onClose();
                  })
                  .catch((e: { message?: string }) => {
                    setErr(e.message ?? "Could not archive.");
                    setBusy(false);
                  });
              }}
            >
              {supplier!.archived ? "Restore" : "Archive"}
            </button>
          )}
          <button className="btn btn-primary" disabled={busy} onClick={save} style={{ marginLeft: "auto" }}>
            {editing ? "Save" : "Create"}
          </button>
        </div>
      }
    >
      <div className="stack">
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Smithfield Meats" />
        </Field>
        <div className="row" style={{ gap: 12 }}>
          <Field label="Phone">
            <input className="input" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+44 …" />
          </Field>
          <div style={{ flex: 1 }}>
            <Field label="Email">
              <input className="input" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="orders@…" />
            </Field>
          </div>
        </div>
        {err && <ErrorBanner message={err} />}
      </div>
    </Modal>
  );
}
