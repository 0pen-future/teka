import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ProgressBar } from "@/components/hv";

describe("ProgressBar", () => {
  it("fills with the coral color when color is 'missing'", () => {
    render(<ProgressBar value={40} color="missing" />);

    const track = screen.getByRole("progressbar");
    const fill = track.firstElementChild;

    expect(fill).toHaveClass("bg-coral-400");
  });

  it("names the progressbar element from aria-label, not the wrapper", () => {
    render(<ProgressBar value={50} aria-label="Có mặt 50%" className="w-16" />);

    const track = screen.getByRole("progressbar", { name: "Có mặt 50%" });
    expect(track).toHaveAttribute("aria-valuenow", "50");
    expect(track.parentElement).toHaveClass("w-16");
  });
});
