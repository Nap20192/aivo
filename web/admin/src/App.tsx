import { ReactNode } from "react";
import {
  createBrowserRouter,
  Navigate,
  RouterProvider,
} from "react-router-dom";
import { AuthProvider, useAuth } from "./auth";
import { Shell } from "./layout";
import { LoadingPage } from "./ui";
import Dashboard from "./pages/Dashboard";
import Design from "./pages/Design";
import Login from "./pages/Login";
import Menu from "./pages/Menu";
import Register from "./pages/Register";
import Settings from "./pages/Settings";
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
        { path: "menu", element: <Menu /> },
        { path: "design", element: <Design /> },
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
