import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { ThemeProvider } from "@/components/shared/theme-provider";
import { Toaster } from "@/components/ui/sonner";
import { SessionRestore } from "@/features/auth";
import { ApiError } from "@/lib/api/errors";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: (failureCount, error) => {
        // Client errors (4xx) will not succeed on retry.
        if (error instanceof ApiError && error.status !== null && error.status < 500) {
          return false;
        }
        return failureCount < 1;
      },
    },
  },
});

export function Providers({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <SessionRestore>{children}</SessionRestore>
        <Toaster />
      </ThemeProvider>
    </QueryClientProvider>
  );
}
