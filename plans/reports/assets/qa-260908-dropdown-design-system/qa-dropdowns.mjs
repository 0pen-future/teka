// Visual + keyboard QA for every HvSelect position (plan 260908-0832, phase 5).
// Usage: node qa-dropdowns.mjs <outDir>   (stack: teka-e2e on :55173, fresh seed)
import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";

const WEB = "/home/cesc/Documents/personal-workspace/teka/apps/web";
const require = createRequire(path.join(WEB, "package.json"));
const { chromium, expect } = require("@playwright/test");

const BASE = "http://localhost:55173";
const OUT = process.argv[2];
fs.mkdirSync(OUT, { recursive: true });

const TRIGGER_PROPS = ["min-height","border-top-width","border-top-color","border-top-left-radius","font-size","font-weight","color","background-color","padding-left"];
const LIST_PROPS = ["background-color","border-top-width","border-top-color","border-top-left-radius","padding-top","max-height","box-shadow"];
const OPTION_PROPS = ["font-size","font-weight","color","border-top-left-radius","min-height","background-color"];

const results = { positions: {}, keyboard: {}, recordsEmpty: null, errors: [] };

async function login(page) {
  await page.goto(BASE + "/login");
  await page.getByLabel("Số điện thoại").fill("0901000001");
  await page.getByLabel("Mật khẩu").fill("lan-password");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await page.waitForURL((u) => !u.pathname.startsWith("/login"));
}

async function styles(loc, props) {
  return loc.evaluate((el, props) => {
    const cs = getComputedStyle(el);
    return Object.fromEntries(props.map((p) => [p, cs.getPropertyValue(p)]));
  }, props);
}

/** The element that actually paints the floating panel (popover content or sheet body). */
function panelOf(page) {
  return page.locator('[data-radix-popper-content-wrapper] > *').first();
}

async function classIdOfToan8(page) {
  await page.goto(BASE + "/records");
  await page.getByRole("combobox", { name: /^Lớp/ }).click();
  await page.getByRole("option", { name: /Toán 8/ }).click();
  await page.waitForURL(/class_id=/);
  return new URL(page.url()).searchParams.get("class_id");
}

async function classIdOfLy7(page) {
  await page.goto(BASE + "/students?tab=classes");
  await page.getByRole("row", { name: /Lý 7/ }).getByRole("link", { name: /Cài đặt/ }).click().catch(async () => {
    await page.getByRole("row", { name: /Lý 7/ }).getByRole("button", { name: /Cài đặt/ }).click();
  });
  await page.waitForURL(/\/classes\/[^/]+\/settings/);
  return page.url().split("/classes/")[1].split("/")[0];
}

async function periodId(page) {
  await page.goto(BASE + "/billing");
  await page.waitForURL(/\/billing\/.+$/);
  return page.url().split("/billing/")[1].split(/[/?]/)[0];
}

const POSITIONS = [
  { id: 1, name: "Hồ sơ học sinh — Lớp", go: async (p) => { await p.goto(BASE + "/records"); return p.getByRole("combobox", { name: /^Lớp/ }); } },
  { id: 2, name: "Sổ lớp — Chọn lớp", go: async (p) => { await p.goto(BASE + "/classbook"); return p.getByRole("combobox", { name: /^Chọn lớp/ }); } },
  { id: 3, name: "Nhật ký — Giáo viên", go: async (p) => { await p.goto(BASE + "/audit"); return p.getByRole("combobox", { name: "Giáo viên" }); } },
  { id: 4, name: "Nhật ký — Nhóm hành động", go: async (p) => { await p.goto(BASE + "/audit"); return p.getByRole("combobox", { name: "Nhóm hành động" }); } },
  { id: 5, name: "Ghi nhận thanh toán — Hình thức", user: ["0901000002", "minh-password"], go: async (p, ctx) => {
      await p.goto(`${BASE}/collections/${process.env.QA_PERIOD_ID}`);
      await expect(p.getByRole("heading", { name: "Thu tiền" })).toBeVisible();
      await p.getByRole("button", { name: "Thu tiền" }).first().click();
      return p.getByRole("dialog").getByRole("combobox", { name: "Hình thức" });
    } },
  { id: 6, name: "Ghi danh vào lớp — Lớp", go: async (p) => {
      await p.goto(BASE + "/students?tab=unenrolled");
      await p.getByRole("button", { name: "Ghi danh vào lớp" }).first().click();
      return p.getByRole("dialog").getByRole("combobox", { name: "Lớp" });
    } },
  { id: 7, name: "Phân quyền — Vai trò", go: async (p) => {
      await p.goto(BASE + "/center");
      await p.getByRole("button", { name: "Phân quyền cho Cô Thu" }).click();
      return p.getByRole("dialog").getByRole("combobox", { name: "Vai trò" });
    } },
  { id: 8, name: "Phân quyền — từng quyền", go: async (p) => {
      await p.goto(BASE + "/center");
      await p.getByRole("button", { name: "Phân quyền cho Cô Thu" }).click();
      return p.getByRole("dialog").getByRole("combobox", { name: /^Quyền / }).first();
    } },
  { id: 9, name: "Cài đặt lớp — Chọn thành viên", go: async (p, ctx) => {
      await p.goto(`${BASE}/classes/${ctx.classId2}/settings`);
      await p.getByRole("button", { name: /Thêm (học vụ|trợ giảng)/ }).first().click();
      return p.getByRole("combobox", { name: /^Chọn (học vụ|trợ giảng)/ });
    } },
  { id: 10, name: "Cài đặt lớp — Bàn giao cho", go: async (p, ctx) => {
      await p.goto(`${BASE}/classes/${ctx.classId}/settings`);
      return p.locator("#teacher-handoff").getByLabel("Bàn giao cho");
    } },
];

async function shoot(page, file) {
  await page.screenshot({ path: path.join(OUT, file), type: "jpeg", quality: 80 });
}

async function capturePosition(page, ctx, pos, width) {
  const key = `${pos.id}`;
  results.positions[key] ??= { name: pos.name };
  const rec = results.positions[key];
  try {
    const combobox = await pos.go(page, ctx);
    await expect(combobox).toBeVisible();
    if (width === 1024) {
      rec.trigger = await styles(combobox, TRIGGER_PROPS);
      rec.triggerAttrs = await combobox.evaluate((el) => ({
        role: el.getAttribute("role"), expanded: el.getAttribute("aria-expanded"),
        haspopup: el.getAttribute("aria-haspopup"), controls: el.getAttribute("aria-controls"),
        placeholder: el.hasAttribute("data-placeholder"), value: el.getAttribute("data-value"),
        disabled: el.getAttribute("aria-disabled"), tag: el.tagName, text: el.textContent?.trim(),
      }));
      await shoot(page, `${pos.id}-closed-1024.jpg`);
    }
    await combobox.click();
    const listbox = page.getByRole("listbox");
    await expect(listbox).toBeVisible();
    const inSheet = await page.getByRole("dialog").filter({ has: listbox }).count();
    const shot = width === 1024 ? `${pos.id}-popover-1024.jpg` : `${pos.id}-sheet-375.jpg`;
    await page.waitForTimeout(250); // let the sheet/popover finish animating
    await shoot(page, shot);
    const info = {
      inSheet: inSheet > 0,
      options: await listbox.getByRole("option").count(),
      hasSearch: (await listbox.locator("..").getByRole("searchbox").count()) + (await page.getByPlaceholder(/^Tìm /).count()) > 0,
      selected: await listbox.locator('[role="option"][aria-selected="true"]').count(),
      selectedHasCheck: await listbox.locator('[role="option"][aria-selected="true"] svg').count(),
      disabledOptions: await listbox.locator('[role="option"][aria-disabled="true"]').count(),
      sheetTitle: inSheet ? await page.getByRole("dialog").filter({ has: listbox }).getByRole("heading").first().textContent().catch(() => null) : null,
      listbox: await styles(listbox, LIST_PROPS),
      firstOption: await styles(listbox.getByRole("option").first(), OPTION_PROPS),
    };
    if (width === 1024) {
      const panel = panelOf(page);
      info.panel = (await panel.count()) ? await styles(panel, LIST_PROPS) : null;
      info.panelAttrs = (await panel.count()) ? await panel.evaluate((el) => ({ maxHeightVar: el.style.maxHeight || getComputedStyle(el).maxHeight })) : null;
    }
    rec[width === 1024 ? "popover" : "sheet"] = info;
    await page.keyboard.press("Escape");
    await expect(listbox).toBeHidden();
  } catch (e) {
    results.errors.push({ pos: pos.id, width, error: String(e).split("\n").slice(0, 3).join(" | ") });
    await shoot(page, `${pos.id}-ERROR-${width}.jpg`).catch(() => {});
  }
}

async function tabTo(page, combobox) {
  for (let i = 0; i < 40; i++) {
    if (await combobox.evaluate((el) => el === document.activeElement)) return true;
    await page.keyboard.press("Tab");
  }
  return false;
}

async function keyboardCheck(page, ctx, label, go) {
  const rec = {};
  results.keyboard[label] = rec;
  try {
    const combobox = await go(page, ctx);
    await expect(combobox).toBeVisible();
    rec.tabReachesTrigger = await tabTo(page, combobox);
    await page.keyboard.press("Enter");
    const listbox = page.getByRole("listbox");
    rec.enterOpens = await listbox.isVisible();
    const activeBefore = await page.evaluate(() => document.activeElement?.textContent?.trim());
    await page.keyboard.press("ArrowDown");
    const afterDown = await page.evaluate(() => ({ role: document.activeElement?.getAttribute("role"), text: document.activeElement?.textContent?.trim() }));
    rec.arrowDownMovesFocus = afterDown.role === "option" && afterDown.text !== activeBefore;
    rec.focusedAfterDown = afterDown.text;
    await page.keyboard.press("ArrowUp");
    const afterUp = await page.evaluate(() => ({ role: document.activeElement?.getAttribute("role"), text: document.activeElement?.textContent?.trim() }));
    rec.arrowUpMovesFocus = afterUp.role === "option";
    await page.keyboard.press("ArrowDown");
    const target = await page.evaluate(() => document.activeElement?.textContent?.trim());
    await page.keyboard.press("Enter");
    rec.enterSelectsAndCloses = (await listbox.count()) === 0 || !(await listbox.isVisible());
    await page.waitForTimeout(600);
    const triggerText = await combobox.textContent();
    rec.triggerShowsPicked = { picked: target, trigger: triggerText?.trim() };
    rec.focusBackOnTriggerAfterSelect = await combobox.evaluate((el) => el === document.activeElement);
    await page.keyboard.press("Enter");
    rec.reopens = await page.getByRole("listbox").isVisible();
    await page.keyboard.press("Escape");
    rec.escapeCloses = !(await page.getByRole("listbox").isVisible().catch(() => false));
    rec.focusBackOnTriggerAfterEscape = await combobox.evaluate((el) => el === document.activeElement);
  } catch (e) {
    rec.error = String(e).split("\n").slice(0, 3).join(" | ");
  }
}

const browser = await chromium.launch();
const ctx = {};
for (const width of [1024, 375]) {
  const context = await browser.newContext({ viewport: { width, height: width === 375 ? 740 : 768 }, locale: "vi-VN" });
  context.setDefaultTimeout(15000);
  const page = await context.newPage();
  await login(page);
  if (width === 1024) {
    ctx.classId = await classIdOfToan8(page);
    ctx.periodId = await periodId(page);
    ctx.classId2 = await classIdOfLy7(page);
    // Records with no class in the URL: does the trigger sit in placeholder state?
    await page.goto(BASE + "/records");
    const cb = page.getByRole("combobox", { name: /^Lớp|Chọn lớp/ });
    await expect(cb).toBeVisible();
    results.recordsEmpty = {
      placeholder: await cb.evaluate((el) => el.hasAttribute("data-placeholder")),
      text: (await cb.textContent())?.trim(),
      styles: await styles(cb, ["font-weight", "color"]),
    };
    await shoot(page, "1-empty-1024.jpg");
  }
  for (const pos of POSITIONS) {
    if (pos.user) {
      const c2 = await browser.newContext({ viewport: { width, height: width === 375 ? 740 : 768 }, locale: "vi-VN" });
      c2.setDefaultTimeout(15000);
      const p2 = await c2.newPage();
      await p2.goto(BASE + "/login");
      await p2.getByLabel("Số điện thoại").fill(pos.user[0]);
      await p2.getByLabel("Mật khẩu").fill(pos.user[1]);
      await p2.getByRole("button", { name: "Đăng nhập" }).click();
      await p2.waitForURL((u) => !u.pathname.startsWith("/login"));
      await capturePosition(p2, ctx, pos, width);
      await c2.close();
    } else {
      await capturePosition(page, ctx, pos, width);
    }
  }
  if (width === 1024) {
    await keyboardCheck(page, ctx, "records", POSITIONS[0].go);
    await keyboardCheck(page, ctx, "audit-teacher", POSITIONS[2].go);
    await keyboardCheck(page, ctx, "override-in-dialog", POSITIONS[7].go);
  }
  await context.close();
}
await browser.close();
fs.writeFileSync(path.join(OUT, "results.json"), JSON.stringify({ ctx, ...results }, null, 2));
console.log(JSON.stringify({ errors: results.errors, keyboard: results.keyboard, recordsEmpty: results.recordsEmpty }, null, 2));
