import { UserPlus, Users } from "lucide-react";
import { FormEvent, useState } from "react";
import { api } from "../api/client";
import type { Role } from "../api/types";
import { useRestaurant } from "../auth";
import { useLoad } from "../lib/useLoad";
import {
  Badge,
  EmptyState,
  ErrorBanner,
  Field,
  LoadingPage,
} from "../ui";

const ROLES: { value: Role; label: string; hint: string }[] = [
  { value: "owner", label: "Owner", hint: "Everything, including billing." },
  { value: "manager", label: "Manager", hint: "Menu, tables, staff, settings." },
  { value: "waiter", label: "Waiter", hint: "POS only." },
];

export default function Staff() {
  const restaurant = useRestaurant();
  const { data, setData, error, loading, reload } = useLoad(
    () => api.listStaff(restaurant.id),
    [restaurant.id],
  );
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("waiter");
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [invitedFlash, setInvitedFlash] = useState<string | null>(null);

  if (loading) return <LoadingPage />;
  if (error || !data)
    return (
      <div className="content">
        <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />
      </div>
    );

  async function invite(e: FormEvent) {
    e.preventDefault();
    if (!/^\S+@\S+\.\S+$/.test(email)) {
      setFormError("Enter a valid email.");
      return;
    }
    setBusy(true);
    setFormError(null);
    try {
      const member = await api.inviteStaff(restaurant.id, { email, role });
      setData([...data!, member]);
      setEmail("");
      setInvitedFlash(member.email);
      setTimeout(() => setInvitedFlash(null), 3000);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Invite failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Staff</h1>
          <p className="page-sub">
            Invites go out by email. Waiters sign in to the POS with the same
            account.
          </p>
        </div>
      </div>

      <div className="stack" style={{ maxWidth: 640 }}>
        <form className="card stack" onSubmit={invite} noValidate>
          <h3>Invite someone</h3>
          <div className="row" style={{ alignItems: "flex-end" }}>
            <div style={{ flex: 1 }}>
              <Field label="Email" error={formError ?? undefined}>
                <input
                  className="input"
                  type="email"
                  placeholder="name@example.com"
                  value={email}
                  aria-invalid={!!formError}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </Field>
            </div>
            <div style={{ width: 140 }}>
              <Field label="Role">
                <select
                  className="select"
                  value={role}
                  onChange={(e) => setRole(e.target.value as Role)}
                >
                  {ROLES.map((r) => (
                    <option key={r.value} value={r.value}>
                      {r.label}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
            <button className="btn btn-primary" disabled={busy}>
              <UserPlus size={15} />
              {busy ? "Inviting…" : "Invite"}
            </button>
          </div>
          <span className="field-hint">
            {ROLES.find((r) => r.value === role)?.hint}
          </span>
          {invitedFlash && (
            <span style={{ font: "var(--type-body)", color: "var(--green-700)" }}>
              Invite sent to {invitedFlash}.
            </span>
          )}
        </form>

        {data.length === 0 ? (
          <div className="card">
            <EmptyState
              icon={Users}
              title="No staff yet"
              message="Invite managers and waiters by email."
            />
          </div>
        ) : (
          <div className="card" style={{ padding: 0 }}>
            <table className="table-plain">
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {data.map((s) => (
                  <tr key={s.id}>
                    <td style={{ font: "var(--type-label)", color: "var(--text-strong)" }}>
                      {s.email}
                    </td>
                    <td style={{ textTransform: "capitalize" }}>{s.role}</td>
                    <td>
                      {s.status === "active" ? (
                        <Badge tone="ok">Active</Badge>
                      ) : (
                        <Badge tone="warn">Invited</Badge>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
