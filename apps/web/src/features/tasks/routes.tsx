import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

export const tasksRoutes: RouteObject[] = [
  {
    path: "tasks",
    handle: { title: "Công việc" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/task-board-page")).TaskBoardPage }),
  },
];
