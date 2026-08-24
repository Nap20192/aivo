import { ReactNode } from "react";
import {
  createBrowserRouter,
  Navigate,
  RouterProvider,
} from "react-router-dom";
import { AuthProvider, useAuth } from "./auth";
import { Shell } from "./layout";
import { LoadingPage } from "./ui";
import Assistant from "./pages/Assistant";
import Dashboard from "./pages/Dashboard";
import GuestDetail from "./pages/GuestDetail";
import Guests from "./pages/Guests";
import Inventory from "./pages/Inventory";
import Ledger from "./pages/Ledger";
import Login from "./pages/Login";
import MenuScreen from "./pages/MenuScreen";
import Register from "./pages/Register";
import Settings from "./pages/Settings";
import ShiftAcceptance from "./pages/ShiftAcceptance";
import Shifts from "./pages/Shifts";
import Staff from "./pages/Staff";
import Subscription from "./pages/Subscription";
import Tables from "./pages/Tables";

function RequireAuth(props: { children: ReactNode }) {
  const { me, loading } = useAuth();
  if (loading) return <LoadingPage />;
  if (!me) return <Navigate to="/login" replace />;
  return <>{props.children}</>;
}

function RequireGuest(props: { children: ReactNode }) {
  const { me, loading } = useAuth();
  if (loading) return <LoadingPage />;
  if (me) return <Navigate to="/" replace />;
  return <>{props.children}</>;
}

const router = createBrowserRouter(
  [
    {
      path: "/login",
      element: (
        <RequireGuest>
          <Login />
        </RequireGuest>
      ),
    },
    {
      path: "/register",
      element: (
        <RequireGuest>
          <Register />
        </RequireGuest>
      ),
    },
    {
      path: "/",
      element: (
        <RequireAuth>
          <Shell />
        </RequireAuth>
      ),
      children: [
        { index: true, element: <Dashboard /> },
        { path: "menu", element: <MenuScreen tab="items" /> },
        { path: "menu/design", element: <MenuScreen tab="design" /> },
        { path: "menu/brief", element: <MenuScreen tab="brief" /> },
        { path: "design", element: <Navigate to="/menu/design" replace /> },
        { path: "assistant", element: <Assistant /> },
        { path: "guests", element: <Guests /> },
        { path: "guests/:customerId", element: <GuestDetail /> },
        { path: "shifts", element: <Shifts /> },
        { path: "shifts/:shiftId", element: <ShiftAcceptance /> },
        { path: "ledger", element: <Ledger /> },
        { path: "inventory", element: <Inventory /> },
        { path: "tables", element: <Tables /> },
        { path: "staff", element: <Staff /> },
        { path: "settings", element: <Settings /> },
        { path: "subscription", element: <Subscription /> },
        { path: "*", element: <Navigate to="/" replace /> },
      ],
    },
  ],
  { basename: "/admin" },
);

export default function App() {
  return (
    <AuthProvider>
      <RouterProvider router={router} />
    </AuthProvider>
  );
}
