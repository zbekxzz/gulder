import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../features/auth/AuthContext";

const links = {
  giver: [
    { to: "/recipients", label: "Получатели" },
    { to: "/settings", label: "Настройки" },
  ],
  florist: [{ to: "/florist", label: "Каталог" }],
  manager: [{ to: "/manager", label: "Управление" }],
} as const;

export function Layout() {
  const { session, logout } = useAuth();
  const navigate = useNavigate();
  const role = session?.user.role;
  return (
    <div className="min-h-screen">
      <header className="border-b border-stone-200 bg-white">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-3 px-4 py-3">
          <NavLink to="/" className="text-lg font-semibold text-brand-700">
            Gulder
          </NavLink>
          <nav className="flex gap-1">
            {role &&
              links[role].map((l) => (
                <NavLink
                  key={l.to}
                  to={l.to}
                  className={({ isActive }) =>
                    `rounded-lg px-3 py-1.5 text-sm ${isActive ? "bg-brand-50 text-brand-700" : "text-stone-600 hover:bg-stone-100"}`
                  }
                >
                  {l.label}
                </NavLink>
              ))}
          </nav>
          {session && (
            <div className="ml-auto flex items-center gap-2 text-sm text-stone-600">
              <span className="hidden sm:inline">
                {session.user.display_name || session.user.email}
              </span>
              <button
                className="rounded-lg px-2 py-1 hover:bg-stone-100"
                onClick={() => {
                  logout();
                  navigate("/login");
                }}
              >
                Выйти
              </button>
            </div>
          )}
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}
