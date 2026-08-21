import { Bell, ExternalLink, QrCode, UtensilsCrossed } from "lucide-react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { useRestaurant } from "../auth";
import { useLoad } from "../lib/useLoad";
import { EmptyState, ErrorBanner, LoadingPage } from "../ui";

export default function Dashboard() {
  const restaurant = useRestaurant();
  const { data, error, loading, reload } = useLoad(
    async () => {
      const [items, tables] = await Promise.all([
        api.listItems(restaurant.id),
        api.listTables(restaurant.id),
      ]);
      return { items, tables };
    },
    [restaurant.id],
  );

  if (loading) return <LoadingPage />;

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">{restaurant.name}</h1>
          <p className="page-sub">
            Today at a glance. Live orders arrive once diners start scanning.
          </p>
        </div>
        <a
          className="btn btn-secondary"
          href={`/${restaurant.slug}/menu`}
          target="_blank"
          rel="noreferrer"
        >
          <ExternalLink size={15} />
          View live menu
        </a>
      </div>

      {error && <ErrorBanner message={error} onRetry={reload} />}

      {data && (
        <div className="stack">
          <div className="stat-grid">
            <div className="card">
              <div className="stat-label">Orders today</div>
              <div className="stat-value">0</div>
              <div className="stat-hint">
                Orders land here when diners send them from the table.
              </div>
            </div>
            <div className="card">
              <div className="stat-label">Open requests</div>
              <div className="stat-value">0</div>
              <div className="stat-hint">Waiter calls and bill requests.</div>
            </div>
            <div className="card">
              <div className="stat-label">Menu items</div>
              <div className="stat-value">{data.items.length}</div>
              <div className="stat-hint">
                {data.items.filter((i) => !i.available).length} currently 86'd
              </div>
            </div>
            <div className="card">
              <div className="stat-label">Tables</div>
              <div className="stat-value">{data.tables.length}</div>
              <div className="stat-hint">Each with its own QR link.</div>
            </div>
          </div>

          <div className="card">
            <h3 style={{ marginBottom: "var(--space-5)" }}>Today's orders</h3>
            <EmptyState
              icon={UtensilsCrossed}
              title="No orders yet"
              message="When a diner sends an order from their phone, it shows up here and in the POS."
            />
          </div>

          <div className="card">
            <h3 style={{ marginBottom: "var(--space-5)" }}>Service requests</h3>
            <EmptyState
              icon={Bell}
              title="No open requests"
              message="Waiter calls and bill requests from tables appear here."
              action={
                data.tables.length === 0 ? (
                  <Link to="/tables" className="btn btn-secondary btn-sm">
                    <QrCode size={14} />
                    Set up tables
                  </Link>
                ) : undefined
              }
            />
          </div>
        </div>
      )}
    </div>
  );
}
