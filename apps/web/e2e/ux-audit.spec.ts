import { AxeBuilder } from "@axe-core/playwright";
import { expect, test, type Browser, type Page } from "@playwright/test";

import { loginAsOwner } from "./helpers/auth.js";
import {
  AXE_VIEWPORTS,
  UX_ROUTES,
  VIEWPORTS,
  viewportsFor,
  type UxRoute,
  type ViewportName,
} from "./helpers/ux-routes.js";

/**
 * UX guard for the web app's DONE contract: no horizontal page overflow,
 * exactly one h1, a per-route tab title, and no axe color-contrast or
 * target-size violations. Layout-dependent checks (overflow from absolutely
 * positioned descendants escaping a scroll box) only reproduce in a real
 * browser, which is why this lives in e2e rather than jsdom.
 *
 * Signs in once and reuses the storage state for every owner route.
 */

const STORAGE_STATE = "test-results/.ux-audit-owner.json";
const SCREENSHOT_DIR = "test-results/ux-audit";

async function saveOwnerSession(browser: Browser) {
  const context = await browser.newContext();
  const page = await context.newPage();
  await loginAsOwner(page);
  await context.storageState({ path: STORAGE_STATE });
  await context.close();
}

async function resolvePath(page: Page, route: UxRoute): Promise<string> {
  return typeof route.path === "string" ? route.path : route.path(page);
}

async function openRoute(page: Page, route: UxRoute) {
  const path = await resolvePath(page, route);
  await page.goto(path);
  await page.waitForLoadState("networkidle");
  // Lazy route chunks and first queries settle before the checks read layout.
  await expect(page.locator("#root")).not.toBeEmpty();
}

async function newRoutePage(browser: Browser, route: UxRoute, viewport: ViewportName) {
  const context = await browser.newContext({
    viewport: VIEWPORTS[viewport],
    storageState: route.auth === "owner" ? STORAGE_STATE : undefined,
  });
  const page = await context.newPage();
  return { context, page };
}

test.beforeAll(async ({ browser }) => {
  await saveOwnerSession(browser);
});

for (const route of UX_ROUTES) {
  for (const viewport of viewportsFor(route)) {
    const runAxe = AXE_VIEWPORTS.includes(viewport);
    const checks = [
      "overflow",
      "h1",
      "title",
      ...(runAxe ? ["color-contrast", "target-size"] : []),
    ];

    test(`${route.name} @ ${viewport}: ${checks.join(", ")}`, async ({ browser }) => {
      const { context, page } = await newRoutePage(browser, route, viewport);
      try {
        await openRoute(page, route);
        await page.screenshot({
          path: `${SCREENSHOT_DIR}/${viewport}-${route.name}.png`,
          fullPage: true,
        });

        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
        );
        expect.soft(overflow, "horizontal page overflow (px)").toBe(0);

        await expect.soft(page.locator("h1"), "exactly one h1").toHaveCount(1);

        const title = await page.title();
        expect.soft(title, "route-specific tab title").not.toBe("Teka");
        expect.soft(title, "tab title format").toMatch(/ · Teka$/);

        if (runAxe) {
          const results = await new AxeBuilder({ page })
            .withRules(["color-contrast", "target-size"])
            .analyze();
          const summary = results.violations.map((violation) => ({
            rule: violation.id,
            targets: violation.nodes.map((node) => node.target.join(" ")).slice(0, 8),
          }));
          expect.soft(summary, "axe color-contrast / target-size violations").toEqual([]);
        }
      } finally {
        await context.close();
      }
    });
  }
}

test("every audited route has a distinct title", async ({ browser }) => {
  // Visits every audited route once; the default 30s budget is per page.
  test.setTimeout(UX_ROUTES.length * 15_000);
  const titles = new Map<string, string>();
  for (const route of UX_ROUTES) {
    const { context, page } = await newRoutePage(browser, route, "desktop");
    try {
      await openRoute(page, route);
      titles.set(route.name, await page.title());
    } finally {
      await context.close();
    }
  }
  const byTitle = new Map<string, string[]>();
  for (const [name, title] of titles) {
    byTitle.set(title, [...(byTitle.get(title) ?? []), name]);
  }
  const duplicates = [...byTitle].filter(([, names]) => names.length > 1);
  expect(duplicates, "routes sharing a tab title").toEqual([]);
});
