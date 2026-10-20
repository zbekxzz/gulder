import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { api, call } from "../../api/client";
import { authStore, type Session } from "./store";

type AuthValue = {
  session: Session | null;
  login: (email: string, password: string) => Promise<Session>;
  register: (email: string, password: string, displayName: string) => Promise<Session>;
  logout: () => void;
};

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(authStore.get());
  useEffect(() => {
    const unsubscribe = authStore.subscribe(setSession);
    return () => {
      unsubscribe();
    };
  }, []);

  const value: AuthValue = {
    session,
    async login(email, password) {
      const s = await call(api.POST("/auth/login", { body: { email, password } }));
      authStore.set(s);
      return s;
    },
    async register(email, password, displayName) {
      const s = await call(
        api.POST("/auth/register", { body: { email, password, display_name: displayName } }),
      );
      authStore.set(s);
      return s;
    },
    logout: () => authStore.clear(),
  };
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth outside AuthProvider");
  return ctx;
}
