import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { HvButton } from "@/components/hv";
import type { HvButtonSize, HvButtonVariant } from "@/components/hv";

const VARIANTS: HvButtonVariant[] = ["primary", "secondary", "reward", "danger", "ghost"];
const SIZES: HvButtonSize[] = ["sm", "md", "lg"];

const VARIANT_BG_CLASS: Record<HvButtonVariant, string> = {
  primary: "bg-mint-400",
  secondary: "bg-sky-300",
  reward: "bg-sun-400",
  danger: "bg-coral-400",
  ghost: "bg-white",
};

const VARIANT_PRESS_SHADOW: Record<HvButtonVariant, string> = {
  primary: "shadow-press-mint",
  secondary: "shadow-press-sky",
  reward: "shadow-press-sun",
  danger: "shadow-press-coral",
  ghost: "shadow-[0_var(--press-depth)_0_var(--line-300),inset_0_0_0_2px_var(--line-200)]",
};

const SIZE_MIN_HEIGHT: Record<HvButtonSize, string> = {
  sm: "min-h-[44px]",
  md: "min-h-[56px]",
  lg: "min-h-[64px]",
};

// Every variant collapses its press shadow on `:active`; ghost's press
// shadow is the outer chunky drop, so its active state instead collapses to
// just the inset border (see hv-button.tsx).
const VARIANT_ACTIVE_SHADOW: Record<HvButtonVariant, string> = {
  primary: "active:shadow-none",
  secondary: "active:shadow-none",
  reward: "active:shadow-none",
  danger: "active:shadow-none",
  ghost: "active:shadow-[inset_0_0_0_2px_var(--line-200)]",
};

describe("HvButton", () => {
  const cases = VARIANTS.flatMap((variant) => SIZES.map((size) => [variant, size] as const));

  it.each(cases)("renders %s / %s with the design system classes", (variant, size) => {
    render(
      <HvButton variant={variant} size={size}>
        {`${variant}-${size}`}
      </HvButton>,
    );
    const button = screen.getByRole("button", { name: `${variant}-${size}` });

    expect(button).toHaveClass(VARIANT_BG_CLASS[variant]);
    expect(button).toHaveClass(SIZE_MIN_HEIGHT[size]);
    expect(button).toHaveClass("font-display");
    expect(button.className).toContain(VARIANT_PRESS_SHADOW[variant]);
    expect(button.className).toContain(VARIANT_ACTIVE_SHADOW[variant]);
    expect(button.className).toContain("active:translate-y-[var(--press-depth)]");
  });

  it("expands to fill its container and renders leading/trailing icons", () => {
    render(
      <HvButton
        block
        icon={<span data-testid="icon-left" />}
        iconRight={<span data-testid="icon-right" />}
      >
        Lưu
      </HvButton>,
    );

    expect(screen.getByRole("button", { name: "Lưu" })).toHaveClass("w-full");
    expect(screen.getByTestId("icon-left")).toBeInTheDocument();
    expect(screen.getByTestId("icon-right")).toBeInTheDocument();
  });

  it("disables interaction and applies the disabled styling", () => {
    render(<HvButton disabled>Không khả dụng</HvButton>);
    const button = screen.getByRole("button", { name: "Không khả dụng" });

    expect(button).toBeDisabled();
    expect(button.className).toContain("disabled:bg-line-200");
    expect(button.className).toContain("disabled:cursor-not-allowed");
  });
});
