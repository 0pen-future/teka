import type { RouteObject } from "react-router";

export const tasksRoutes: RouteObject[] = [
  {
    path: "tasks",
    lazy: async () => ({ Component: (await import("./pages/task-board-page")).TaskBoardPage }),
  },
];
