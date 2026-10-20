import type { ReactNode } from "react";
import { createBrowserRouter, Navigate } from "react-router-dom";
import { Layout } from "./components/Layout";
import { useAuth } from "./features/auth/AuthContext";
import { FloristPage } from "./pages/florist/FloristPage";
import { RecipientPage } from "./pages/giver/RecipientPage";
import { RecipientsPage } from "./pages/giver/RecipientsPage";
import { RunPage } from "./pages/giver/RunPage";
import { SettingsPage } from "./pages/giver/SettingsPage";
import { EvaluationTab, ExpertTab } from "./pages/manager/ResearchTabs";
import { ManagerPage } from "./pages/manager/ManagerPage";
import { AuthPage } from "./pages/public/LoginPage";

import { homeFor, type Role } from "./routing";

function RequireRole({ role, children }: { role: Role; children: ReactNode }) {
  const { session } = useAuth();
  if (!session) return <Navigate to="/login" replace />;
  if (session.user.role !== role) return <Navigate to={homeFor(session.user.role)} replace />;
  return children;
}

function Home() {
  const { session } = useAuth();
  return <Navigate to={session ? homeFor(session.user.role) : "/login"} replace />;
}

const managerResearchTabs = [
  { id: "expert", label: "Экспертная разметка", render: () => <ExpertTab /> },
  { id: "evaluation", label: "Оценка моделей", render: () => <EvaluationTab /> },
];

export const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { path: "/", element: <Home /> },
      { path: "/login", element: <AuthPage mode="login" /> },
      { path: "/register", element: <AuthPage mode="register" /> },
      {
        path: "/recipients",
        element: (
          <RequireRole role="giver">
            <RecipientsPage />
          </RequireRole>
        ),
      },
      {
        path: "/recipients/:id",
        element: (
          <RequireRole role="giver">
            <RecipientPage />
          </RequireRole>
        ),
      },
      {
        path: "/runs/:id",
        element: (
          <RequireRole role="giver">
            <RunPage />
          </RequireRole>
        ),
      },
      {
        path: "/settings",
        element: (
          <RequireRole role="giver">
            <SettingsPage />
          </RequireRole>
        ),
      },
      {
        path: "/florist",
        element: (
          <RequireRole role="florist">
            <FloristPage />
          </RequireRole>
        ),
      },
      {
        path: "/manager",
        element: (
          <RequireRole role="manager">
            <ManagerPage extraTabs={managerResearchTabs} />
          </RequireRole>
        ),
      },
      { path: "*", element: <Home /> },
    ],
  },
]);
