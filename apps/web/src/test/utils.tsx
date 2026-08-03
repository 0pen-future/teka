import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { createMemoryRouter, RouterProvider, type RouteObject } from "react-router";

import { ThemeProvider } from "@/components/shared/theme-provider";
import { Toaster } from "@/components/ui/sonner";
import { useAuthStore } from "@/features/auth";
import type { User } from "@/features/users";

import { adminUser, aliceUser } from "./msw/handlers";

/** Seeds the auth store as if the user had just logged in. */
export function signInAs(user: User): void {
  useAuthStore.getState().setSession(user, "test-access-token");
}

export const testAdmin = adminUser;
export const testUser = aliceUser;

interface RenderWithProvidersOptions {
  /** Initial URL, including any search params. */
  route?: string;
  /** Route pattern the element mounts at (e.g. "/users/:id"). */
  path?: string;
  /** Additional routes for asserting navigation targets. */
  extraRoutes?: RouteObject[];
}

export function renderWithProviders(
  ui: ReactElement,
  { route = "/", path = "/", extraRoutes = [] }: RenderWithProvidersOptions = {},
) {
  // Fresh client per test: no cross-test cache bleed, no retries hiding errors.
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const router = createMemoryRouter([{ path, element: ui }, ...extraRoutes], {
    initialEntries: [route],
  });
  const result = render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <RouterProvider router={router} />
        <Toaster />
      </ThemeProvider>
    </QueryClientProvider>,
  );
  return { ...result, router, queryClient };
}
