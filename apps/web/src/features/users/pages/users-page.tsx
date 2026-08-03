import { MoreHorizontalIcon, PlusIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { toast } from "sonner";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { EmptyState } from "@/components/shared/empty-state";
import { PageHeader } from "@/components/shared/page-header";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuthStore } from "@/features/auth";
import { toApiError } from "@/lib/api/errors";

import { CreateUserDialog } from "../components/create-user-dialog";
import { EditUserDialog } from "../components/edit-user-dialog";
import { useDeleteUser, useUsersList } from "../hooks/use-users";
import { userSortKeys } from "../schemas/user-schemas";
import type { User, UserSort } from "../types/user-types";

const SEARCH_DEBOUNCE_MS = 300;

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

export function UsersPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const page = Math.max(1, Number(searchParams.get("page") ?? "1") || 1);
  const q = searchParams.get("q") ?? "";
  // URL values are untyped strings; fall back to defaults on anything outside
  // the API's accepted values.
  const roleParam = searchParams.get("role");
  const role = roleParam === "admin" || roleParam === "user" ? roleParam : "";
  const sortParam = searchParams.get("sort") ?? "-created_at";
  const sort = (userSortKeys as readonly string[]).includes(sortParam)
    ? (sortParam as UserSort)
    : "-created_at";

  const isAdmin = useAuthStore((state) => state.user?.role === "admin");

  const [searchInput, setSearchInput] = useState(q);
  // Tracks the last q value this page wrote (or adopted), so an external URL
  // change — e.g. the nav link resetting /users — can be told apart from our
  // own debounced writes and mirrored back into the input.
  const lastAppliedQ = useRef(q);
  const [createOpen, setCreateOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [deletingUser, setDeletingUser] = useState<User | null>(null);

  const usersQuery = useUsersList({
    page,
    sort,
    q: q || undefined,
    role: role || undefined,
  });
  const deleteMutation = useDeleteUser();

  // replace: true keeps keystroke-level filter changes from flooding history;
  // Back leaves the page rather than replaying every search/page tweak.
  function updateParams(updates: Record<string, string | null>) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [key, value] of Object.entries(updates)) {
          if (value === null || value === "") {
            next.delete(key);
          } else {
            next.set(key, value);
          }
        }
        return next;
      },
      { replace: true },
    );
  }

  // Mirror external URL changes back into the input.
  useEffect(() => {
    if (q !== lastAppliedQ.current) {
      lastAppliedQ.current = q;
      setSearchInput(q);
    }
  }, [q]);

  // Push the debounced search term into the URL; changing the filter restarts
  // pagination from page 1.
  useEffect(() => {
    if (searchInput === q) {
      return;
    }
    const handle = setTimeout(() => {
      lastAppliedQ.current = searchInput;
      updateParams({ q: searchInput, page: null });
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(handle);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput, q]);

  function confirmDelete() {
    if (!deletingUser) {
      return;
    }
    deleteMutation.mutate(deletingUser.id, {
      onSuccess: () => {
        toast.success(`User ${deletingUser.email} deleted`);
        setDeletingUser(null);
      },
      // Deleting your own account is a server-side CONFLICT.
      onError: (error) => toast.error(toApiError(error).message),
    });
  }

  const columns: DataTableColumn<User>[] = [
    {
      key: "name",
      header: "Name",
      sortKey: "name",
      cell: (user) => (
        <Link to={`/users/${user.id}`} className="font-medium underline-offset-4 hover:underline">
          {user.name}
        </Link>
      ),
    },
    { key: "email", header: "Email", sortKey: "email", cell: (user) => user.email },
    {
      key: "role",
      header: "Role",
      cell: (user) => (
        <span className="inline-flex rounded-full border px-2 py-0.5 text-xs font-medium capitalize">
          {user.role}
        </span>
      ),
    },
    {
      key: "created_at",
      header: "Created",
      sortKey: "created_at",
      cell: (user) => formatDate(user.created_at),
    },
    ...(isAdmin
      ? [
          {
            key: "actions",
            header: "",
            className: "w-12",
            cell: (user: User) => (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" aria-label={`Actions for ${user.email}`}>
                    <MoreHorizontalIcon aria-hidden className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={() => setEditingUser(user)}>Edit</DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onSelect={() => setDeletingUser(user)}>
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ),
          },
        ]
      : []),
  ];

  return (
    <div className="space-y-6">
      <PageHeader title="Users" description="Manage workspace accounts.">
        {isAdmin ? (
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon aria-hidden className="size-4" />
            New user
          </Button>
        ) : null}
      </PageHeader>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Input
          type="search"
          placeholder="Search by name or email…"
          aria-label="Search users"
          className="sm:max-w-xs"
          value={searchInput}
          onChange={(event) => setSearchInput(event.target.value)}
        />
        <Select
          value={role || "all"}
          onValueChange={(value) =>
            updateParams({ role: value === "all" ? null : value, page: null })
          }
        >
          <SelectTrigger className="sm:w-40" aria-label="Filter by role">
            <SelectValue placeholder="All roles" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All roles</SelectItem>
            <SelectItem value="admin">Admin</SelectItem>
            <SelectItem value="user">User</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {usersQuery.isError ? (
        <EmptyState title="Could not load users" description={toApiError(usersQuery.error).message}>
          <Button variant="outline" onClick={() => void usersQuery.refetch()}>
            Retry
          </Button>
        </EmptyState>
      ) : (
        <DataTable
          columns={columns}
          rows={usersQuery.data?.items ?? []}
          rowKey={(user) => user.id}
          loading={usersQuery.isPending}
          sort={sort}
          onSortChange={(nextSort) => updateParams({ sort: nextSort, page: null })}
          meta={usersQuery.data?.meta}
          onPageChange={(nextPage) => updateParams({ page: String(nextPage) })}
          empty={
            <EmptyState
              title="No users found"
              description={
                q || role ? "Try adjusting your search or filters." : "No accounts exist yet."
              }
            />
          }
        />
      )}

      <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} />
      <EditUserDialog
        user={editingUser}
        onOpenChange={(open) => {
          if (!open) {
            setEditingUser(null);
          }
        }}
        canEditRole={isAdmin}
      />
      <ConfirmDialog
        open={deletingUser !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeletingUser(null);
          }
        }}
        title="Delete user"
        description={`This permanently removes ${deletingUser?.email ?? "this user"} and revokes their access.`}
        confirmLabel="Delete"
        destructive
        pending={deleteMutation.isPending}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
