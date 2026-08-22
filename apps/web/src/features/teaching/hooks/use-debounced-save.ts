import { useCallback, useEffect, useRef } from "react";

export interface DebouncedSave<T> {
  /** Replace the pending payload and (re)start the delay. */
  schedule: (payload: T) => void;
  /** Fire the pending payload now (blur, explicit save); no-op when nothing is pending. */
  flush: () => void;
  /** Drop the pending payload without saving (e.g. the server state just won). */
  cancel: () => void;
}

/**
 * Trailing-edge debounce for per-keystroke saves (records-screen scores and
 * notes): typing keeps replacing the pending payload, only the last one is
 * sent once the user pauses for `delayMs`. Flushes on unmount so navigating
 * away never drops input. `save` is read through a ref, so an inline mutation
 * callback doesn't reset the timer every render.
 */
export function useDebouncedSave<T>(save: (payload: T) => void, delayMs = 500): DebouncedSave<T> {
  const saveRef = useRef(save);
  // Updated after render — timers and event handlers only fire later, so
  // they always see the latest callback without it resetting the timer.
  useEffect(() => {
    saveRef.current = save;
  });
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Payload boxed so `null` payloads stay distinguishable from "nothing pending".
  const pendingRef = useRef<{ payload: T } | null>(null);

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const flush = useCallback(() => {
    clearTimer();
    const pending = pendingRef.current;
    if (pending) {
      pendingRef.current = null;
      saveRef.current(pending.payload);
    }
  }, [clearTimer]);

  const schedule = useCallback(
    (payload: T) => {
      pendingRef.current = { payload };
      clearTimer();
      timerRef.current = setTimeout(flush, delayMs);
    },
    [clearTimer, flush, delayMs],
  );

  const cancel = useCallback(() => {
    clearTimer();
    pendingRef.current = null;
  }, [clearTimer]);

  useEffect(() => flush, [flush]);

  return { schedule, flush, cancel };
}
