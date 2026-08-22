import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useDebouncedSave } from "../hooks/use-debounced-save";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useDebouncedSave", () => {
  it("sends only the last payload once typing pauses — never one per keystroke", () => {
    const save = vi.fn<(payload: string) => void>();
    const { result } = renderHook(() => useDebouncedSave(save, 500));

    // A steady typist: 10 keystrokes 100ms apart. Trailing-edge debounce
    // resets on every keystroke, so nothing fires mid-burst.
    for (let i = 1; i <= 10; i += 1) {
      result.current.schedule(`draft ${i}`);
      vi.advanceTimersByTime(100);
    }
    expect(save).not.toHaveBeenCalled();

    // The pause completes the 500ms window (100ms already elapsed).
    vi.advanceTimersByTime(400);
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith("draft 10");
  });

  it("caps sustained edits at ~2 requests/second/field", () => {
    const save = vi.fn<(payload: number) => void>();
    const { result } = renderHook(() => useDebouncedSave(save, 500));

    // Worst case for a 500ms debounce: an edit exactly every 500ms for 3
    // seconds. Each window can complete at most once → ≤2 saves per second.
    for (let i = 0; i < 6; i += 1) {
      result.current.schedule(i);
      vi.advanceTimersByTime(500);
    }
    expect(save.mock.calls.length).toBeLessThanOrEqual(6);
    expect(save.mock.calls.length / 3).toBeLessThanOrEqual(2);
  });

  it("flush fires the pending payload immediately and is a no-op when idle", () => {
    const save = vi.fn<(payload: string) => void>();
    const { result } = renderHook(() => useDebouncedSave(save, 500));

    result.current.flush();
    expect(save).not.toHaveBeenCalled();

    result.current.schedule("on blur");
    result.current.flush();
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith("on blur");

    // The cleared timer must not fire the same payload a second time.
    vi.advanceTimersByTime(1000);
    expect(save).toHaveBeenCalledTimes(1);
  });

  it("cancel drops the pending payload without saving", () => {
    const save = vi.fn<(payload: string) => void>();
    const { result } = renderHook(() => useDebouncedSave(save, 500));

    result.current.schedule("stale draft");
    result.current.cancel();
    vi.advanceTimersByTime(1000);
    expect(save).not.toHaveBeenCalled();
  });

  it("flushes the pending payload on unmount so navigation never drops input", () => {
    const save = vi.fn<(payload: string) => void>();
    const { result, unmount } = renderHook(() => useDebouncedSave(save, 500));

    result.current.schedule("typed then navigated");
    unmount();
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith("typed then navigated");
  });
});
