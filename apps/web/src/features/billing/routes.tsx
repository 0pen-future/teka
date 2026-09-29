import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

import { BillingIndexRedirect } from "./components/billing-index-redirect";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`.
 */
export const billingRoutes: RouteObject[] = [
  {
    path: "billing",
    handle: { title: "Chốt sổ" } satisfies RouteHandle,
    Component: BillingIndexRedirect,
  },
  {
    path: "billing/:periodId",
    handle: { title: "Chốt sổ" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/billing-review-page")).BillingReviewPage,
    }),
  },
];
