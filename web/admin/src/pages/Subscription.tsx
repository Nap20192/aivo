import { Check } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { Plan } from "../api/types";
import { useLoad } from "../lib/useLoad";
import { Badge, ErrorBanner, LoadingPage, Modal, NoticeBanner } from "../ui";

// Plan limits from docs/PLATFORM.md "Subscriptions".
const PLANS: {
  id: Plan;
  name: string;
  price_cents: number;
  features: string[];
}[] = [
  {
    id: "free",
    name: "Free",
    price_cents: 0,
    features: ["1 restaurant", "30 menu items", "QR menu & table ordering"],
  },
  {
    id: "pro",
    name: "Pro",
    price_cents: 2900,
    features: [
      "Unlimited menu items",
      "Custom domain",
      "Full theming & design.md",
    ],
  },
  {
    id: "business",
    name: "Business",
    price_cents: 7900,
    features: ["Everything in Pro", "Multiple restaurants", "POS seats"],
  },
];

const STATUS_LABEL: Record<string, string> = {
  trialing: "Trial",
  active: "Active",
  past_due: "Past due",
  canceled: "Canceled",
};

export default function Subscription() {
  const { data, setData, error, loading, reload } = useLoad(
    () => api.getSubscription(),
    [],
  );
  const [confirm, setConfirm] = useState<Plan | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  if (loading) return <LoadingPage />;
  if (error || !data)
    return (
      <div className="content">
        <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />
      </div>
    );

  async function switchPlan() {
    if (!confirm) return;
    setBusy(true);
    setActionError(null);
    try {
      setData(await api.setSubscription(confirm));
      setConfirm(null);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Plan change failed.");
    } finally {
      setBusy(false);
    }
  }

  const current = PLANS.find((p) => p.id === data.plan);

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Subscription</h1>
          <p className="page-sub">
            You're on <strong>{current?.name}</strong>
            {data.renews_at && ` · renews ${data.renews_at}`}
          </p>
        </div>
        <Badge tone={data.status === "past_due" ? "danger" : data.status === "canceled" ? "neutral" : "ok"}>
          {STATUS_LABEL[data.status] ?? data.status}
        </Badge>
      </div>

      <div className="stack">
        <NoticeBanner>
          Billing is not wired to a payment provider yet — plan changes apply
          instantly, no card required.
        </NoticeBanner>

        {actionError && <ErrorBanner message={actionError} />}

        <div className="plan-grid">
          {PLANS.map((p) => {
            const isCurrent = p.id === data.plan;
            return (
              <div
                key={p.id}
                className={"card plan-card" + (isCurrent ? " current" : "")}
              >
                <div className="row-between">
                  <span className="plan-name">{p.name}</span>
                  {isCurrent && <Badge tone="ok">Current plan</Badge>}
                </div>
                <div className="plan-price">
                  {p.price_cents === 0 ? (
                    "$0"
                  ) : (
                    <>
                      ${(p.price_cents / 100).toFixed(0)}
                      <small> / month</small>
                    </>
                  )}
                </div>
                <ul className="plan-features">
                  {p.features.map((f) => (
                    <li key={f}>
                      <Check
                        size={14}
                        style={{ color: "var(--accent-greens)", flex: "none" }}
                      />
                      {f}
                    </li>
                  ))}
                </ul>
                <button
                  className={
                    "btn " + (isCurrent ? "btn-secondary" : "btn-primary")
                  }
                  disabled={isCurrent}
                  onClick={() => setConfirm(p.id)}
                  style={{ marginTop: "auto" }}
                >
                  {isCurrent ? "Current plan" : `Switch to ${p.name}`}
                </button>
              </div>
            );
          })}
        </div>
      </div>

      {confirm && (
        <Modal
          title={`Switch to ${PLANS.find((p) => p.id === confirm)?.name}`}
          onClose={() => setConfirm(null)}
          footer={
            <>
              <button
                className="btn btn-secondary"
                onClick={() => setConfirm(null)}
              >
                Cancel
              </button>
              <button
                className="btn btn-primary"
                onClick={switchPlan}
                disabled={busy}
              >
                {busy ? "Switching…" : "Confirm switch"}
              </button>
            </>
          }
        >
          <p style={{ font: "var(--type-body)" }}>
            The new plan's limits apply immediately. No payment is taken in this
            version.
          </p>
        </Modal>
      )}
    </div>
  );
}
