import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useSearchParams } from "react-router";
import { describe, expect, it } from "vitest";

import { asColumnId } from "@/lib/kanban";

import { useBoardUrlState } from "../hooks/use-board-url-state";

function wrapper(initialEntry: string) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <MemoryRouter initialEntries={[initialEntry]}>{children}</MemoryRouter>;
  };
}

/**
 * `useBoardUrlState` doesn't expose the raw `URLSearchParams` it writes; this
 * reads them back via a second `useSearchParams()` call sharing the same
 * `MemoryRouter` location, so a test can assert on the querystring itself
 * (e.g. "the default value was dropped") without depending on
 * `window.location`, which `MemoryRouter` never touches.
 */
function useBoardUrlStateWithParams() {
  const [searchParams] = useSearchParams();
  return { ...useBoardUrlState(), searchParams };
}

describe("useBoardUrlState", () => {
  it("defaults filter, assignee, and collapsed when the URL carries no params", () => {
    const { result } = renderHook(() => useBoardUrlState(), { wrapper: wrapper("/tasks") });
    expect(result.current.filter).toBe("all");
    expect(result.current.assignee).toBe("");
    expect(result.current.collapsed).toEqual(new Set());
  });

  it("reads filter, assignee, and collapsed from the URL", () => {
    const teacherId = "b3a3c6a4-3f0e-4c1a-9a7e-2f9f8d6a1b11";
    const { result } = renderHook(() => useBoardUrlState(), {
      wrapper: wrapper(`/tasks?filter=overdue&assignee=${teacherId}&collapsed=col-a,col-b`),
    });
    expect(result.current.filter).toBe("overdue");
    expect(result.current.assignee).toBe(teacherId);
    expect(result.current.collapsed).toEqual(new Set([asColumnId("col-a"), asColumnId("col-b")]));
  });

  it("falls back to defaults for an invalid filter or a non-uuid assignee", () => {
    const { result } = renderHook(() => useBoardUrlState(), {
      wrapper: wrapper("/tasks?filter=bogus&assignee=not-a-uuid"),
    });
    expect(result.current.filter).toBe("all");
    expect(result.current.assignee).toBe("");
  });

  it("set() writes a non-default filter and drops it again once reset to the default", () => {
    const { result } = renderHook(() => useBoardUrlStateWithParams(), {
      wrapper: wrapper("/tasks"),
    });

    act(() => result.current.set({ filter: "mine" }));
    expect(result.current.filter).toBe("mine");
    expect(result.current.searchParams.get("filter")).toBe("mine");

    act(() => result.current.set({ filter: "all" }));
    expect(result.current.filter).toBe("all");
    expect(result.current.searchParams.has("filter")).toBe(false);
  });

  it("set() merges a partial update without clobbering the other keys", () => {
    const teacherId = "b3a3c6a4-3f0e-4c1a-9a7e-2f9f8d6a1b11";
    const { result } = renderHook(() => useBoardUrlState(), { wrapper: wrapper("/tasks") });

    act(() => result.current.set({ filter: "unassigned" }));
    act(() => result.current.set({ assignee: teacherId }));

    expect(result.current.filter).toBe("unassigned");
    expect(result.current.assignee).toBe(teacherId);
  });

  it("set() serializes collapsed as a sorted, comma-joined list and drops the key once emptied", () => {
    const { result } = renderHook(() => useBoardUrlStateWithParams(), {
      wrapper: wrapper("/tasks"),
    });

    act(() =>
      result.current.set({ collapsed: new Set([asColumnId("col-b"), asColumnId("col-a")]) }),
    );
    expect(result.current.searchParams.get("collapsed")).toBe("col-a,col-b");
    expect(result.current.collapsed).toEqual(new Set([asColumnId("col-a"), asColumnId("col-b")]));

    act(() => result.current.set({ collapsed: new Set() }));
    expect(result.current.searchParams.has("collapsed")).toBe(false);
  });
});
