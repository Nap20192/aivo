import {
  BookUser,
  CreditCard,
  LayoutDashboard,
  LogOut,
  QrCode,
  Settings,
  Sparkles,
  UtensilsCrossed,
  Users,
} from "lucide-react";
import { useEffect, useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { isMocked, onMockChange } from "./api/client";
import { useAuth } from "./auth";
import { Badge } from "./ui";

const nav = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/menu", label: "Menu", icon: UtensilsCrossed },
  { to: "/assistant", label: "Assistant", icon: Sparkles },
  { to: "/tables", label: "Tables & QR", icon: QrCode },
  { to: "/guests", label: "Guests", icon: BookUser },
  { to: "/staff", label: "Staff", icon: Users },
  { to: "/settings", label: "Settings", icon: Settings },
  { to: "/subscription", label: "Subscription", icon: CreditCard },
];

export function Shell() {
  const { me, logout } = useAuth();
  const navigate = useNavigate();
  const [mock, setMock] = useState(isMocked());
  useEffect(() => onMockChange(() => setMock(true)), []);

  const restaurant = me?.restaurants[0];

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          aivo<span className="dot">.</span>
        </div>
        <nav className="sidebar-nav">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.end}
              className={({ isActive }) =>
                "nav-link" + (isActive ? " active" : "")
              }
            >
              <n.icon size={16} strokeWidth={1.75} />
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-foot">
          <button
            className="btn btn-ghost btn-sm"
            style={{ width: "100%", justifyContent: "flex-start" }}
            onClick={async () => {
              await logout();
              navigate("/login");
            }}
          >
            <LogOut size={15} strokeWidth={1.75} />
            Sign out
          </button>
        </div>
      </aside>
      <div className="main">
        <header className="topbar">
          <div className="row">
            <span style={{ font: "var(--type-section-title)" }}>
              {restaurant?.name ?? ""}
            </span>
            {mock && <Badge tone="warn">Demo mode — no backend</Badge>}
          </div>
          <span style={{ font: "var(--type-body)", color: "var(--text-muted)" }}>
            {me?.user.email}
          </span>
        </header>
        <Outlet />
      </div>
    </div>
  );
}
