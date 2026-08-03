import type { RouteObject } from "react-router";

import { LoginPage } from "./pages/login-page";
import { RegisterPage } from "./pages/register-page";

/** Public-only routes; the app router mounts these under the auth layout. */
export const authRoutes: RouteObject[] = [
  { path: "/login", element: <LoginPage /> },
  { path: "/register", element: <RegisterPage /> },
];
