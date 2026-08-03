import { LayoutDashboardIcon, UsersIcon } from "lucide-react";
import { NavLink, Outlet } from "react-router";

import { ModeToggle } from "@/components/shared/mode-toggle";
import { useAuthStore } from "@/features/auth/stores/auth-store";
import { cn } from "@/lib/utils";

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboardIcon },
  { to: "/users", label: "Users", icon: UsersIcon },
];

function SidebarNav({ className }: { className?: string }) {
  return (
    <nav aria-label="Main" className={cn("flex gap-1 md:flex-col", className)}>
      {navItems.map(({ to, label, icon: Icon }) => (
        <NavLink
          key={to}
          to={to}
          end={to === "/"}
          className={({ isActive }) =>
            cn(
              "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors",
              isActive
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
            )
          }
        >
          <Icon aria-hidden className="size-4" />
          {label}
        </NavLink>
      ))}
    </nav>
  );
}

/** Authenticated app shell: top bar, sidebar (inline nav under md), content. */
export function DashboardLayout() {
  const user = useAuthStore((state) => state.user);

  return (
    <div className="flex min-h-svh flex-col">
      <header className="flex items-center justify-between border-b px-4 py-3 md:px-6">
        <span className="text-lg font-semibold tracking-tight">Teka</span>
        <div className="flex items-center gap-3">
          {user ? (
            <span className="hidden text-sm text-muted-foreground sm:inline">{user.email}</span>
          ) : null}
          <ModeToggle />
        </div>
      </header>
      {/* Sidebar collapses to a horizontal nav bar under md. */}
      <div className="flex flex-1 flex-col md:flex-row">
        <aside className="border-b p-2 md:w-56 md:border-b-0 md:border-r md:p-4">
          <SidebarNav />
        </aside>
        <main id="main-content" className="flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
