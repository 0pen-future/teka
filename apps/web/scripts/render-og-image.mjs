// Renders public/og-image.svg to public/og-image.png (the 1200×630 link
// preview Zalo and Facebook show). Chromium from Playwright draws the SVG with
// the app's own Baloo 2 / Nunito files inlined, so the PNG never falls back to
// a system font and no extra dependency is needed.
//   node scripts/render-og-image.mjs   (from apps/web)
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { chromium } from "@playwright/test";
const require = createRequire(process.cwd() + "/package.json");
const font = (p) => readFileSync(require.resolve(p)).toString("base64");
const faces = [
  ["Baloo 2", 800, "@fontsource/baloo-2/files/baloo-2-latin-800-normal.woff2"],
  ["Baloo 2", 800, "@fontsource/baloo-2/files/baloo-2-vietnamese-800-normal.woff2"],
  ["Nunito", 700, "@fontsource/nunito/files/nunito-latin-700-normal.woff2"],
  ["Nunito", 700, "@fontsource/nunito/files/nunito-vietnamese-700-normal.woff2"],
  ["Nunito", 700, "@fontsource/nunito/files/nunito-latin-ext-700-normal.woff2"],
]
  .map(
    ([f, w, p]) =>
      `@font-face{font-family:'${f}';font-weight:${w};src:url(data:font/woff2;base64,${font(p)}) format('woff2');}`,
  )
  .join("\n");
const svg = readFileSync("public/og-image.svg", "utf8");
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1200, height: 630 } });
await page.setContent(`<style>${faces} html,body{margin:0}</style>${svg}`);
await page.evaluate(() => document.fonts.ready);
await page.evaluate(async () => {
  for (const f of ["800 10px 'Baloo 2'", "700 10px Nunito"])
    await document.fonts.load(f, "Quản lý");
});
await page.locator("svg").first().screenshot({ path: "public/og-image.png" });
await browser.close();
