import type { RouteObject } from "react-router";

import { DashboardPage } from "./pages/dashboard-page";

/** Mounted by the app router inside the protected dashboard layout. */
export const dashboardRoutes: RouteObject[] = [{ index: true, element: <DashboardPage /> }];
