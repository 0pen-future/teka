import { useQuery } from "@tanstack/react-query";
import { CircleCheckIcon, CircleXIcon } from "lucide-react";

import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { apiClient } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import { apiOrigin } from "@/lib/config/env";

interface HealthResponse {
  status: string;
}

/**
 * Proves the frontend→API path end to end. healthz lives at the server root,
 * not under /api/v1, so the request overrides baseURL with the origin.
 */
export function HealthCard() {
  const { data, error, isPending } = useQuery({
    queryKey: ["healthz"],
    queryFn: async () => {
      const response = await apiClient.get<HealthResponse>("/healthz", {
        baseURL: apiOrigin,
      });
      return response.data;
    },
    refetchInterval: 30_000,
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>API health</CardTitle>
        <CardDescription>{apiOrigin}/healthz</CardDescription>
      </CardHeader>
      <CardContent>
        {isPending ? (
          <Skeleton className="h-5 w-40" />
        ) : error ? (
          <div className="flex items-center gap-2 text-sm text-destructive">
            <CircleXIcon aria-hidden className="size-4" />
            <span>
              {error instanceof ApiError ? `${error.code}: ${error.message}` : "Unexpected error"}
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-sm">
            <CircleCheckIcon aria-hidden className="size-4 text-green-600 dark:text-green-500" />
            <span>Status: {data.status}</span>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
