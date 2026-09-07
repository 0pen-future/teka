import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { mockViewport } from "@/test/viewport";

import { useMediaQuery } from "../use-media-query";

describe("useMediaQuery", () => {
  it("reports false under the default test shim", () => {
    const { result } = renderHook(() => useMediaQuery("(min-width: 640px)"));
    expect(result.current).toBe(false);
  });

  it("evaluates min-width and max-width against the mocked viewport", () => {
    mockViewport(1280);
    const { result: wide } = renderHook(() => useMediaQuery("(min-width: 640px)"));
    const { result: narrow } = renderHook(() => useMediaQuery("(max-width: 767px)"));
    expect(wide.current).toBe(true);
    expect(narrow.current).toBe(false);
  });

  it("re-renders when the viewport crosses the breakpoint", () => {
    mockViewport(1280);
    const { result } = renderHook(() => useMediaQuery("(min-width: 640px)"));
    expect(result.current).toBe(true);
    act(() => mockViewport(375));
    expect(result.current).toBe(false);
    act(() => mockViewport(640));
    expect(result.current).toBe(true);
  });
});
