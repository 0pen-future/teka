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
});
