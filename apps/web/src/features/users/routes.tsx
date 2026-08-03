import type { RouteObject } from "react-router";

import { UsersPage } from "./pages/users-page";

/** Mounted by the app router inside the protected dashboard layout. */
export const usersRoutes: RouteObject[] = [{ path: "users", element: <UsersPage /> }];
