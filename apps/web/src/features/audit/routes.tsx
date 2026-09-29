import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

export const auditRoutes: RouteObject[] = [
  {
    path: "audit",
    handle: { title: "Nhật ký hoạt động" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/audit-page")).AuditPage }),
  },
];
