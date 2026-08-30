import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import { toast } from "sonner";
import { afterAll, afterEach, beforeAll, beforeEach } from "vitest";

import { useAuthStore } from "@/features/auth";
import { markRefreshAlive } from "@/lib/api/auth-bridge";

import { server } from "./msw/server";

// Any request without a matching MSW handler is a test bug, not a network
// call — the suite must stay green offline.
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  cleanup();
  toast.dismiss();
});
afterAll(() => server.close());

beforeEach(() => {
  // Auth state is module-scoped (zustand store + refresh-dead flag); reset it
  // so tests stay order-independent.
  useAuthStore.setState({ user: null, accessToken: null });
  markRefreshAlive();
  window.localStorage.clear();
});

// --- jsdom shims for browser APIs used by ThemeProvider and Radix ---

Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => false,
  }),
});

// jsdom has no layout engine; Radix only needs these APIs to exist.
class ResizeObserverStub {
  observe = () => undefined;
  unobserve = () => undefined;
  disconnect = () => undefined;
}
window.ResizeObserver = ResizeObserverStub;

Element.prototype.scrollIntoView = () => undefined;
Element.prototype.hasPointerCapture = () => false;
Element.prototype.setPointerCapture = () => undefined;
Element.prototype.releasePointerCapture = () => undefined;
