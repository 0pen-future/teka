/// <reference types="node" />
import { readFileSync } from "node:fs";
import path from "node:path";

import { createElement } from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { HvButton } from "@/components/hv";
import type { HvButtonVariant } from "@/components/hv";

/**
 * Locks WCAG AA contrast (4.5:1 for normal text) for the token pairs the UI
 * relies on. Reads the real `colors.css` and the classes `HvButton` actually
 * renders, so changing a token or a variant's text color fails here without
 * a running stack (axe in e2e only sees what a page happens to render).
 */

const AA_NORMAL_TEXT = 4.5;

// Read from disk: vitest strips CSS content from `?raw` imports.
const colorsCss = readFileSync(path.resolve(import.meta.dirname, "../colors.css"), "utf8");

/** `--name` → raw value (a hex or a `var(--other)` alias). */
const declarations = new Map(
  [...colorsCss.matchAll(/--([a-z0-9-]+):\s*([^;]+);/g)].map(([, name, value]) => [
    name ?? "",
    (value ?? "").trim(),
  ]),
);

/** Resolves a token through `var()` aliases to its hex value. */
function tokenHex(name: string, seen: string[] = []): string {
  const value = declarations.get(name);
  if (!value) throw new Error(`unknown color token --${name}`);
  const alias = /^var\(--([a-z0-9-]+)\)$/.exec(value);
  if (alias?.[1]) {
    if (seen.includes(name)) throw new Error(`alias cycle at --${name}`);
    return tokenHex(alias[1], [...seen, name]);
  }
  if (!/^#[0-9a-f]{6}$/i.test(value)) throw new Error(`--${name} is not a 6-digit hex: ${value}`);
  return value;
}

function relativeLuminance(hex: string): number {
  const linear = (offset: number) => {
    const c = parseInt(hex.slice(offset, offset + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * linear(1) + 0.7152 * linear(3) + 0.0722 * linear(5);
}

function contrastRatio(fgHex: string, bgHex: string): number {
  const fg = relativeLuminance(fgHex);
  const bg = relativeLuminance(bgHex);
  return (Math.max(fg, bg) + 0.05) / (Math.min(fg, bg) + 0.05);
}

/**
 * Tailwind utility → token. The DS bridges `--color-<x>` to `var(--<x>)` for
 * every ramp; `text-on-brand` maps to the semantic `--text-on-brand` alias.
 */
const UTILITY_TOKEN_OVERRIDES: Record<string, string> = { "on-brand": "text-on-brand" };

function utilityToken(utility: string): string {
  return UTILITY_TOKEN_OVERRIDES[utility] ?? utility;
}

/** The unprefixed (resting state) `bg-*` and `text-*` color utilities of a class list. */
function restingColors(className: string): { bg: string; fg: string } {
  const classes = className.split(/\s+/).filter((c) => !c.includes(":"));
  const bg = classes.find((c) => /^bg-[a-z]+(-\d+)?$/.test(c));
  const fg = classes.find((c) => /^text-(?:[a-z]+-\d+|white|on-brand)$/.test(c));
  if (!bg || !fg) throw new Error(`no resting bg/text color in "${className}"`);
  return { bg: utilityToken(bg.slice(3)), fg: utilityToken(fg.slice(5)) };
}

const BUTTON_VARIANTS: HvButtonVariant[] = ["primary", "secondary", "reward", "danger", "ghost"];

describe("HvButton variant contrast", () => {
  it.each(BUTTON_VARIANTS)("%s text meets AA on its background", (variant) => {
    render(createElement(HvButton, { variant }, "Nhãn"));
    const { bg, fg } = restingColors(screen.getByRole("button").className);
    const ratio = contrastRatio(tokenHex(fg), tokenHex(bg));
    expect(ratio, `${fg} on ${bg} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(AA_NORMAL_TEXT);
  });
});

describe("text token contrast on surfaces", () => {
  const PAIRS: [fg: string, bg: string][] = [
    ["text-on-brand", "mint-400"],
    ["text-on-brand", "sky-300"],
    ["text-on-brand", "coral-400"],
    ["text-on-brand", "sun-400"],
    ["text-on-primary", "mint-400"],
    ["ink-500", "cream-100"],
    ["ink-500", "white"],
    ["ink-500", "mint-50"],
    ["mint-600", "cream-100"],
    ["mint-600", "white"],
    ["mint-600", "mint-50"],
    ["coral-600", "coral-100"],
  ];

  it.each(PAIRS)("%s on %s meets AA", (fg, bg) => {
    const ratio = contrastRatio(tokenHex(fg), tokenHex(bg));
    expect(ratio, `${fg} on ${bg} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(AA_NORMAL_TEXT);
  });
});
