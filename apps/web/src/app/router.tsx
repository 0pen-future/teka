import { createBrowserRouter } from "react-router";

import { NotFound } from "@/components/shared/not-found";
import { ProtectedRoute } from "@/features/auth";
import { authRoutes } from "@/features/auth/routes";
import { attendanceRoutes } from "@/features/attendance/routes";
import { billingRoutes } from "@/features/billing/routes";
import { centerRoutes } from "@/features/center/routes";
import { collectionsRoutes } from "@/features/collections/routes";
import { dashboardRoutes } from "@/features/dashboard/routes";
import { profileRoutes } from "@/features/profile/routes";
import { rosterRoutes } from "@/features/roster/routes";
import { statementRoutes } from "@/features/statement";
import { AuthLayout } from "@/layouts/auth-layout";
import { DashboardLayout } from "@/layouts/dashboard-layout";
import { PublicLayout } from "@/layouts/public-layout";
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
        children: [
          ...dashboardRoutes,
          ...rosterRoutes,
          ...attendanceRoutes,
          ...billingRoutes,
          ...collectionsRoutes,
          ...profileRoutes,
          ...centerRoutes,
        ],
      },
      {
        element: <PublicLayout />,
        children: statementRoutes,
      },
      { path: "*", element: <NotFound /> },
    ],
  },
]);
