import type { Locator, Page } from "@playwright/test";

/** Column pixels to move before the first step; clears the 6px mouse activation distance. */
const ACTIVATION_OFFSET = 8;

/** How far below a drop zone's top edge a "top" drop lands: inside the zone, past its padding. */
const TOP_INSET = 24;

export type DropPoint = "center" | "top";

/**
 * A point inside `locator`. Columns stretch to the tallest one on the board,
 * so their centre can sit below the viewport where no pointer event reaches
 * them; `"top"` picks a point just inside the top edge instead.
 */
async function pointIn(locator: Locator, at: DropPoint) {
  await locator.scrollIntoViewIfNeeded();
  const box = await locator.boundingBox();
  if (!box) throw new Error("drag: element is not visible");
  const y = at === "top" ? box.y + Math.min(TOP_INSET, box.height / 2) : box.y + box.height / 2;
  return { x: box.x + box.width / 2, y };
}

/**
 * Drags `source` onto `destination` with real mouse events. dnd-kit only
 * updates its collision state on successive `pointermove` events, so the
 * pointer is walked across in `steps` increments instead of jumping.
 */
export async function dragTo(
  page: Page,
  source: Locator,
  destination: Locator,
  { at = "center", steps = 12 }: { at?: DropPoint; steps?: number } = {},
) {
  const from = await pointIn(source, "center");
  const to = await pointIn(destination, at);
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + ACTIVATION_OFFSET, from.y);
  for (let i = 1; i <= steps; i++) {
    await page.mouse.move(
      from.x + ((to.x - from.x) * i) / steps,
      from.y + ((to.y - from.y) * i) / steps,
    );
  }
  await page.mouse.up();
}

/**
 * Touch counterpart of `dragTo`, driven through the Chrome DevTools Protocol
 * because Playwright has no multi-step touch gesture API. Chromium turns each
 * touch event into the pointer events dnd-kit's `TouchSensor` listens to.
 * `holdMs` is the time the finger rests before moving: the sensor's
 * long-press delay is 250ms, so 300ms activates a drag and 0 reads as a scroll.
 */
export async function touchDragTo(
  page: Page,
  source: Locator,
  destination: Locator,
  { holdMs, steps = 12 }: { holdMs: number; steps?: number },
) {
  const from = await pointIn(source, "center");
  const to = await pointIn(destination, "center");
  const cdp = await page.context().newCDPSession(page);
  const touch = (type: "touchStart" | "touchMove" | "touchEnd", x?: number, y?: number) =>
    cdp.send("Input.dispatchTouchEvent", {
      type,
      touchPoints: x === undefined || y === undefined ? [] : [{ x, y }],
    });
  try {
    await touch("touchStart", from.x, from.y);
    if (holdMs > 0) await page.waitForTimeout(holdMs);
    for (let i = 1; i <= steps; i++) {
      await touch(
        "touchMove",
        from.x + ((to.x - from.x) * i) / steps,
        from.y + ((to.y - from.y) * i) / steps,
      );
      // Real fingers report at a finite rate; back-to-back moves collapse.
      await page.waitForTimeout(16);
    }
    await touch("touchEnd");
  } finally {
    await cdp.detach();
  }
}
