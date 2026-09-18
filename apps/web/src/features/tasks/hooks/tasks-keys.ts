import type { GetBoardParams } from "../api/tasks-api";

/**
 * `boards()` (no params) is the invalidation root every mutation targets —
 * TanStack Query's cache is the single source of truth for board state (no
 * optimistic snapshot/rollback), so every mutation error just invalidates
 * this prefix and lets a refetch recover.
 */
export const tasksKeys = {
  all: ["tasks"] as const,
  boards: () => [...tasksKeys.all, "board"] as const,
  board: (params: GetBoardParams) => [...tasksKeys.boards(), params] as const,
  directory: () => [...tasksKeys.all, "directory"] as const,
};
