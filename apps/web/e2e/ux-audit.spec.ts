import { AxeBuilder } from "@axe-core/playwright";
import { expect, test as base, type BrowserContext, type Page } from "@playwright/test";

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
 * Signs in once per worker and opens every owner route in that same browser
 * context. Separate contexts restored from one saved session would each
 * rotate the same refresh token, and the API revokes the session on reuse.
 */

const SCREENSHOT_DIR = "test-results/ux-audit";

const test = base.extend<object, { ownerContext: BrowserContext }>({
  ownerContext: [
    async ({ browser }, use) => {
      const context = await browser.newContext();
      await loginAsOwner(await context.newPage());
      await use(context);
      await context.close();
    },
    { scope: "worker" },
  ],
});

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

/** A page at `viewport`: in the shared signed-in context, or a fresh one for public routes. */
async function withRoutePage(
  ownerContext: BrowserContext,
  browserContext: () => Promise<BrowserContext>,
  route: UxRoute,
  viewport: ViewportName,
  run: (page: Page) => Promise<void>,
) {
  const context = route.auth === "owner" ? ownerContext : await browserContext();
  const page = await context.newPage();
  await page.setViewportSize(VIEWPORTS[viewport]);
  try {
    await run(page);
  } finally {
    await page.close();
    if (context !== ownerContext) await context.close();
  }
}

for (const route of UX_ROUTES) {
  for (const viewport of viewportsFor(route)) {
    const runAxe = AXE_VIEWPORTS.includes(viewport);
    const checks = [
      "overflow",
      "h1",
      "title",
      ...(runAxe ? ["color-contrast", "target-size"] : []),
    ];

    test(`${route.name} @ ${viewport}: ${checks.join(", ")}`, async ({ browser, ownerContext }) => {
      await withRoutePage(
        ownerContext,
        () => browser.newContext(),
        route,
        viewport,
        async (page) => {
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
        },
      );
    });
  }
}

test("every audited route has a distinct title", async ({ browser, ownerContext }) => {
  // Visits every audited route once; the default 30s budget is per page.
  test.setTimeout(UX_ROUTES.length * 15_000);
  const titles = new Map<string, string>();
  for (const route of UX_ROUTES) {
    await withRoutePage(
      ownerContext,
      () => browser.newContext(),
      route,
      "desktop",
      async (page) => {
        await openRoute(page, route);
        titles.set(route.name, await page.title());
      },
    );
  }
  const byTitle = new Map<string, string[]>();
  for (const [name, title] of titles) {
    byTitle.set(title, [...(byTitle.get(title) ?? []), name]);
  }
  const duplicates = [...byTitle].filter(([, names]) => names.length > 1);
  expect(duplicates, "routes sharing a tab title").toEqual([]);
});

test.describe("wide tables on a phone", () => {
  test("a wide table scrolls inside its region instead of widening the page", async ({
    ownerContext,
  }) => {
    const page = await ownerContext.newPage();
    await page.setViewportSize(VIEWPORTS.phone);
    try {
      await page.goto("/classes");
      const region = page.getByRole("region", { name: "Bảng danh sách lớp" });
      await expect(region).toBeVisible();
      const scroll = await region.evaluate((el) => el.scrollWidth - el.clientWidth);
      expect(scroll, "table still scrolls inside its region").toBeGreaterThan(0);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );
      expect(overflow, "horizontal page overflow (px)").toBe(0);
    } finally {
      await page.close();
    }
  });

  test("an empty filter result stays on screen", async ({ ownerContext }) => {
    const page = await ownerContext.newPage();
    await page.setViewportSize(VIEWPORTS.phone);
    try {
      await page.goto("/courses");
      await page.getByRole("searchbox", { name: "Tìm khóa học" }).fill("zzz-khong-khop");
      await expect(page.getByText("Không có khóa nào khớp.")).toBeInViewport();
    } finally {
      await page.close();
    }
  });
});
