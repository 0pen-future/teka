import { ArrowLeftIcon } from "lucide-react";
import { Link, useParams } from "react-router";

import { EmptyState } from "@/components/shared/empty-state";
import { PageHeader } from "@/components/shared/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { toApiError } from "@/lib/api/errors";

import { useUser } from "../hooks/use-users";

function formatDateTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function UserDetailPage() {
  const { id = "" } = useParams();
  const userQuery = useUser(id);

  return (
    <div className="space-y-6">
      <PageHeader title="User detail" description="Account information.">
        <Button variant="outline" asChild>
          <Link to="/users">
            <ArrowLeftIcon aria-hidden className="size-4" />
            Back to users
          </Link>
        </Button>
      </PageHeader>

      {userQuery.isPending ? (
        <Card className="max-w-xl">
          <CardHeader>
            <Skeleton className="h-5 w-48" />
            <Skeleton className="h-4 w-64" />
          </CardHeader>
          <CardContent className="space-y-3">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-2/3" />
          </CardContent>
        </Card>
      ) : userQuery.isError ? (
        <EmptyState title="Could not load user" description={toApiError(userQuery.error).message}>
          <Button variant="outline" onClick={() => void userQuery.refetch()}>
            Retry
          </Button>
        </EmptyState>
      ) : (
        <Card className="max-w-xl">
          <CardHeader>
            <CardTitle>{userQuery.data.name}</CardTitle>
            <CardDescription>{userQuery.data.email}</CardDescription>
          </CardHeader>
          <CardContent>
            <dl className="space-y-2 text-sm">
              <div className="flex justify-between gap-4">
                <dt className="text-muted-foreground">Role</dt>
                <dd className="capitalize">{userQuery.data.role}</dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="text-muted-foreground">Created</dt>
                <dd>{formatDateTime(userQuery.data.created_at)}</dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="text-muted-foreground">Updated</dt>
                <dd>{formatDateTime(userQuery.data.updated_at)}</dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="text-muted-foreground">ID</dt>
                <dd className="font-mono text-xs">{userQuery.data.id}</dd>
              </div>
            </dl>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
