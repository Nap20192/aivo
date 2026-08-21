import { BookUser, Search } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { useRestaurant } from "../auth";
import { formatCents } from "../lib/money";
import { useLoad } from "../lib/useLoad";
import { Badge, EmptyState, ErrorBanner, LoadingPage } from "../ui";

export function fmtDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

export default function Guests() {
  const restaurant = useRestaurant();
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");

  useEffect(() => {
    const t = setTimeout(() => setDebounced(query), 250);
    return () => clearTimeout(t);
  }, [query]);

  const { data, error, loading, reload } = useLoad(
    () => api.listGuests(restaurant.id, debounced || undefined),
    [restaurant.id, debounced],
  );

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Guests</h1>
          <p className="page-sub">
            Everyone who registered and ordered here. Profiles appear
            automatically with their first linked order.
          </p>
        </div>
      </div>

      <div className="stack">
        <div className="row" style={{ maxWidth: 360, position: "relative" }}>
          <Search
            size={15}
            style={{
              position: "absolute",
              left: 12,
              color: "var(--text-subtle)",
              pointerEvents: "none",
            }}
          />
          <input
            className="input"
            style={{ paddingLeft: 34 }}
            placeholder="Search name, email or tag"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>

        {error && <ErrorBanner message={error} onRetry={reload} />}
        {loading && <LoadingPage />}

        {data && data.length === 0 && (
          <div className="card">
            <EmptyState
              icon={BookUser}
              title={debounced ? "No guests match" : "No guests yet"}
              message={
                debounced
                  ? "Try a different name, email or tag."
                  : "When a signed-in diner orders or hands a cart to a waiter, their profile shows up here."
              }
            />
          </div>
        )}

        {data && data.length > 0 && (
          <div className="card" style={{ padding: 0 }}>
            <table className="table-plain">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th style={{ textAlign: "right" }}>Visits</th>
                  <th style={{ textAlign: "right" }}>Total spent</th>
                  <th>Last seen</th>
                  <th>Tags</th>
                </tr>
              </thead>
              <tbody>
                {data.map((g) => (
                  <tr key={g.customer.id}>
                    <td>
                      <Link
                        to={`/guests/${g.customer.id}`}
                        style={{
                          font: "var(--type-label)",
                          borderBottom: "none",
                        }}
                      >
                        {g.customer.name}
                      </Link>
                    </td>
                    <td style={{ color: "var(--text-muted)" }}>
                      {g.customer.email}
                    </td>
                    <td className="num" style={{ textAlign: "right" }}>
                      {g.visits}
                    </td>
                    <td className="num" style={{ textAlign: "right" }}>
                      {formatCents(g.total_spent_cents)}
                    </td>
                    <td className="num" style={{ fontSize: 12 }}>
                      {fmtDate(g.last_seen)}
                    </td>
                    <td>
                      <span className="row" style={{ flexWrap: "wrap", gap: 4 }}>
                        {g.tags.map((t) => (
                          <Badge key={t}>{t}</Badge>
                        ))}
                      </span>
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
