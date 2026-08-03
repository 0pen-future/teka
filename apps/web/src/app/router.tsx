import { createBrowserRouter } from "react-router";

import { NotFound } from "@/components/shared/not-found";
import { ProtectedRoute } from "@/features/auth";
import { authRoutes } from "@/features/auth/routes";
import { dashboardRoutes } from "@/features/dashboard/routes";
import { usersRoutes } from "@/features/users/routes";
import { AuthLayout } from "@/layouts/auth-layout";
import { DashboardLayout } from "@/layouts/dashboard-layout";
import { RootLayout } from "@/layouts/root-layout";

// Features export their route arrays; this file owns the tree (layouts,
// guards, and where each feature mounts).
export const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      {
        element: <AuthLayout />,
        children: authRoutes,
      },
      {
        element: (
          <ProtectedRoute>
            <DashboardLayout />
          </ProtectedRoute>
        ),
        path: "/",
        children: [...dashboardRoutes, ...usersRoutes],
      },
      { path: "*", element: <NotFound /> },
    ],
  },
]);
