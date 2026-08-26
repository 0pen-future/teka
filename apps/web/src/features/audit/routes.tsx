import type { RouteObject } from "react-router";

export const auditRoutes: RouteObject[] = [
  {
    path: "audit",
    lazy: async () => ({ Component: (await import("./pages/audit-page")).AuditPage }),
  },
];
