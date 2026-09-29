import { expect, type Page } from "@playwright/test";

export interface Viewport {
  width: number;
  height: number;
}

export const VIEWPORTS = {
  desktop: { width: 1440, height: 900 },
  tablet: { width: 768, height: 1024 },
  phone: { width: 375, height: 812 },
  narrow: { width: 320, height: 720 },
} as const satisfies Record<string, Viewport>;

export type ViewportName = keyof typeof VIEWPORTS;

/** Viewports where the axe contrast/target-size scan runs (kept to two for run time). */
export const AXE_VIEWPORTS: readonly ViewportName[] = ["desktop", "phone"];

export interface UxRoute {
  /** Test and screenshot name. */
  name: string;
  /** Static path, or a resolver for routes whose id only the running app knows. */
  path: string | ((page: Page) => Promise<string>);
  auth: "owner" | "public";
  /** Also audit at 320×720 (routes that overflowed at reflow width in round 1). */
  narrow?: boolean;
  /** A tab of another audited page, so it shares that page's tab title. */
  tabOf?: string;
}

/** `/billing` redirects to the current period; follow it instead of hardcoding an id. */
async function currentBillingPath(page: Page): Promise<string> {
  await page.goto("/billing");
  await expect(page).toHaveURL(/\/billing\/[^/]+$/);
  return new URL(page.url()).pathname;
}

export const UX_ROUTES: readonly UxRoute[] = [
  { name: "home", path: "/", auth: "owner" },
  { name: "sessions", path: "/sessions", auth: "owner" },
  { name: "classbook", path: "/classbook", auth: "owner", narrow: true },
  { name: "records", path: "/records", auth: "owner" },
  { name: "contacts", path: "/students?tab=contacts", auth: "owner", tabOf: "students" },
  { name: "classes", path: "/classes", auth: "owner", narrow: true },
  { name: "classes-recruiting", path: "/classes/recruiting", auth: "owner" },
  { name: "class-invitations", path: "/class-invitations", auth: "owner" },
  { name: "paths", path: "/paths", auth: "owner" },
  { name: "courses", path: "/courses", auth: "owner" },
  { name: "library", path: "/library", auth: "owner" },
  { name: "library-materials", path: "/library/materials", auth: "owner" },
  { name: "library-exercises", path: "/library/exercises", auth: "owner" },
  { name: "billing", path: currentBillingPath, auth: "owner" },
  { name: "lesson-plans", path: "/lesson-plans", auth: "owner", narrow: true },
  { name: "students", path: "/students", auth: "owner" },
  { name: "center", path: "/center", auth: "owner" },
  { name: "center-permissions", path: "/center/permissions", auth: "owner" },
  { name: "center-class-config", path: "/center/class-config", auth: "owner" },
  { name: "audit", path: "/audit", auth: "owner" },
  { name: "tasks", path: "/tasks", auth: "owner" },
  { name: "reports", path: "/reports", auth: "owner" },
  { name: "profile", path: "/profile", auth: "owner", narrow: true },
  { name: "login", path: "/login", auth: "public" },
  { name: "forgot-password", path: "/forgot-password", auth: "public" },
  { name: "not-found", path: "/khong-ton-tai", auth: "public" },
];

/** Viewports a route is audited at. */
export function viewportsFor(route: UxRoute): ViewportName[] {
  const base: ViewportName[] = ["desktop", "tablet", "phone"];
  return route.narrow ? [...base, "narrow"] : base;
}
