import type { RouteObject } from "react-router";

import { UserDetailPage } from "./pages/user-detail-page";
import { UsersPage } from "./pages/users-page";

/** Mounted by the app router inside the protected dashboard layout. */
export const usersRoutes: RouteObject[] = [
  { path: "users", element: <UsersPage /> },
  { path: "users/:id", element: <UserDetailPage /> },
];
