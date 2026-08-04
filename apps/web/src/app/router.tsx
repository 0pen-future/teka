import { createBrowserRouter } from "react-router";

import { NotFound } from "@/components/shared/not-found";
import { ProtectedRoute } from "@/features/auth";
import { authRoutes } from "@/features/auth/routes";
import { dashboardRoutes } from "@/features/dashboard/routes";
import { AuthLayout } from "@/layouts/auth-layout";
import { DashboardLayout } from "@/layouts/dashboard-layout";
import { RootLayout } from "@/layouts/root-layout";

// Features export their route arrays; this file owns the tree (layouts,
// guards, and where each feature mounts). Feature routes code-split via
// route.lazy; the HydrateFallback renders while the first chunk loads.
export const router = createBrowserRouter([
  {
    element: <RootLayout />,
    HydrateFallback: () => null,
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
        children: [...dashboardRoutes],
      },
      { path: "*", element: <NotFound /> },
    ],
  },
]);
