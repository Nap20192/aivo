import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { api } from "./api/client";
import { ApiError } from "./api/error";
import type { Me, Restaurant } from "./api/types";

interface AuthState {
  me: Me | null;
  loading: boolean;
  restaurant: Restaurant | null;
  setMe: (me: Me | null) => void;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider(props: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setMe(await api.me());
    } catch (e) {
      if (e instanceof ApiError && (e.status === 401 || e.status === 0)) {
        setMe(null);
      } else {
        throw e;
      }
    }
  }, []);

  useEffect(() => {
    refresh().finally(() => setLoading(false));
  }, [refresh]);

  const logout = useCallback(async () => {
    await api.logout();
    setMe(null);
  }, []);

  return (
    <AuthContext.Provider
      value={{
        me,
        loading,
        restaurant: me?.restaurants[0] ?? null,
        setMe,
        refresh,
        logout,
      }}
    >
      {props.children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth outside AuthProvider");
  return ctx;
}

// Restaurant is guaranteed on authed routes (guard redirects otherwise).
export function useRestaurant(): Restaurant {
  const { restaurant } = useAuth();
  if (!restaurant) throw new Error("no restaurant in session");
  return restaurant;
}
