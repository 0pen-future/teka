import { LogOutIcon, type LucideProps } from "lucide-react";
import type { ComponentType } from "react";
import { NavLink, Outlet, Link } from "react-router";

import {
  HvCheckIcon,
  HvFileIcon,
  HvHomeIcon,
  HvSendIcon,
  HvUsersIcon,
  HvWalletIcon,
} from "@/components/hv";
import { useAuthStore, useLogout } from "@/features/auth";
import { useCurrentPeriod } from "@/features/billing";
import { usePendingSessions } from "@/features/dashboard/hooks/use-dashboard";
import { cn, nameInitial } from "@/lib/utils";

interface NavEntry {
  label: string;
  /** Null while a period-scoped route's id has not resolved yet — renders disabled. */
  to: string | null;
  Icon: ComponentType<LucideProps>;
  pending?: boolean;
}

/**
 * The six nav destinations from the prototype `home` screen. The three
 * period-scoped routes (Chốt sổ, Gửi thông báo, Thu tiền) build their link
 * once `useCurrentPeriod` resolves rather than routing through a redirect
 * page, since phase 1 owns no `/billing/current`-style route.
 */
function useNavEntries(): NavEntry[] {
  const { data: period } = useCurrentPeriod();
  const { data: pendingSessionsResponse } = usePendingSessions();
  const periodId = period?.id ?? null;
  const hasPending = (pendingSessionsResponse?.total ?? 0) > 0;

  return [
    { label: "Tổng quan", to: "/", Icon: HvHomeIcon },
    { label: "Điểm danh", to: "/sessions", Icon: HvCheckIcon, pending: hasPending },
    { label: "Lớp & học sinh", to: "/students", Icon: HvUsersIcon },
    { label: "Chốt sổ", to: periodId ? `/billing/${periodId}` : null, Icon: HvFileIcon },
    {
      label: "Gửi thông báo",
      to: periodId ? `/notifications/${periodId}` : null,
      Icon: HvSendIcon,
    },
    { label: "Thu tiền", to: periodId ? `/collections/${periodId}` : null, Icon: HvWalletIcon },
  ];
}

function PendingDot() {
  return <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-coral-400" />;
}

function SidebarNavItem({ to, label, Icon, pending }: NavEntry) {
  if (!to) {
    return (
      <span
        aria-disabled="true"
        className="flex cursor-not-allowed items-center gap-3 rounded-[14px] px-3 py-[10px] text-[14px] text-ink-300"
      >
        <Icon className="size-5" />
        <span className="font-display font-bold">{label}</span>
      </span>
    );
  }
  return (
    <NavLink
      to={to}
      end={to === "/"}
      className={({ isActive }) =>
        cn(
          "relative flex items-center gap-3 rounded-[14px] px-3 py-[10px] text-[14px] transition-colors",
          isActive ? "bg-mint-50 text-mint-600" : "text-ink-500 hover:bg-cream-100",
        )
      }
    >
      <span className="relative">
        <Icon className="size-5" />
        {pending ? <PendingDot /> : null}
      </span>
      <span className="font-display font-bold">{label}</span>
    </NavLink>
  );
}

function RailNavItem({ to, label, Icon, pending }: NavEntry) {
  if (!to) {
    return (
      <span
        aria-disabled="true"
        title={label}
        className="flex size-12 cursor-not-allowed items-center justify-center rounded-[14px] text-ink-300"
      >
        <Icon className="size-5" />
      </span>
    );
  }
  return (
    <NavLink
      to={to}
      end={to === "/"}
      title={label}
      aria-label={label}
      className={({ isActive }) =>
        cn(
          "relative flex size-12 items-center justify-center rounded-[14px] transition-colors",
          isActive ? "bg-mint-50 text-mint-600" : "text-ink-500 hover:bg-cream-100",
        )
      }
    >
      <Icon className="size-5" />
      {pending ? <PendingDot /> : null}
    </NavLink>
  );
}

function BottomTabItem({ to, label, Icon, pending }: NavEntry) {
  if (!to) {
    return (
      <span
        aria-disabled="true"
        className="flex min-h-[56px] flex-1 cursor-not-allowed flex-col items-center justify-center gap-1 px-1 text-center text-[11px] text-ink-300"
      >
        <Icon className="size-5" />
        <span className="w-full truncate">{label}</span>
      </span>
    );
  }
  return (
    <NavLink
      to={to}
      end={to === "/"}
      className={({ isActive }) =>
        cn(
          "flex min-h-[56px] flex-1 flex-col items-center justify-center gap-1 px-1 text-center text-[11px] font-semibold",
          isActive ? "text-mint-600" : "text-ink-500",
        )
      }
    >
      <span className="relative">
        <Icon className="size-5" />
        {pending ? <PendingDot /> : null}
      </span>
      <span className="w-full truncate">{label}</span>
    </NavLink>
  );
}

function CurrentPeriodCard() {
  const { data: period } = useCurrentPeriod();
  return (
    <div className="m-4 rounded-[var(--radius-lg)] bg-mint-50 p-4">
      <p className="text-[11px] font-semibold tracking-wide text-ink-400 uppercase">Kỳ hiện tại</p>
      {period ? (
        <>
          <p className="mt-1 font-display text-[15px] font-bold text-mint-600">
            Tháng {period.month}/{period.year}
          </p>
          <p className="text-[12px] text-ink-500">
            {period.status === "open" ? "Đang mở" : "Đã chốt"}
          </p>
        </>
      ) : (
        <p className="mt-1 text-[12px] text-ink-400">Đang tải…</p>
      )}
    </div>
  );
}

function CurrentPeriodDisc() {
  const { data: period } = useCurrentPeriod();
  if (!period) {
    return null;
  }
  return (
    <Link
      to={`/billing/${period.id}`}
      title="Kỳ hiện tại"
      aria-label={`Kỳ hiện tại: tháng ${period.month}`}
      className="mb-6 flex size-11 items-center justify-center rounded-full bg-mint-50 font-display text-[13px] font-bold text-mint-600"
    >
      T{period.month}
    </Link>
  );
}

function LogoutButton() {
  const logoutMutation = useLogout();
  return (
    <button
      type="button"
      onClick={() => logoutMutation.mutate()}
      disabled={logoutMutation.isPending}
      className="inline-flex items-center gap-2 rounded-[var(--radius-md)] px-3 py-2 text-[13px] font-semibold text-ink-500 transition-colors hover:bg-cream-100 hover:text-coral-600 disabled:opacity-50"
    >
      <LogOutIcon aria-hidden className="size-4" />
      Đăng xuất
    </button>
  );
}

/**
 * Sidebar footer per the prototype: below the period card, split by a border —
 * the profile nav entry (avatar disc + name) on top of the Đăng xuất row.
 */
function SidebarFooter() {
  const user = useAuthStore((state) => state.user);
  const logoutMutation = useLogout();
  return (
    <div className="mx-4 mb-4 border-t border-line-200 pt-3">
      <NavLink
        to="/profile"
        className={({ isActive }) =>
          cn(
            "flex items-center gap-2.5 rounded-[14px] px-2.5 py-2 transition-colors hover:bg-cream-200",
            isActive && "bg-mint-50",
          )
        }
      >
        <span
          aria-hidden
          className="flex size-9 shrink-0 items-center justify-center rounded-full bg-mint-100 font-display text-[16px] font-extrabold text-mint-600"
        >
          {nameInitial(user?.full_name ?? "")}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13.5px] font-extrabold text-ink-900">
            {user?.full_name}
          </span>
          <span className="block text-[12px] text-ink-400">Hồ sơ giáo viên</span>
        </span>
      </NavLink>
      <button
        type="button"
        onClick={() => logoutMutation.mutate()}
        disabled={logoutMutation.isPending}
        className="flex w-full cursor-pointer items-center gap-2.5 rounded-[14px] px-2.5 py-2 text-left text-[13.5px] font-extrabold text-ink-400 transition-colors hover:bg-cream-200 hover:text-coral-500 disabled:opacity-50"
      >
        <span className="inline-flex w-9 justify-center">
          <LogOutIcon aria-hidden className="size-[18px]" />
        </span>
        Đăng xuất
      </button>
    </div>
  );
}

/** Rail avatar disc → /profile, mirroring the sidebar footer at md–lg. */
function ProfileDisc() {
  const user = useAuthStore((state) => state.user);
  return (
    <NavLink
      to="/profile"
      title="Hồ sơ giáo viên"
      aria-label="Hồ sơ giáo viên"
      className={({ isActive }) =>
        cn(
          "mb-2 flex size-11 items-center justify-center rounded-full bg-mint-100 font-display text-[15px] font-bold text-mint-600",
          isActive && "ring-2 ring-mint-400",
        )
      }
    >
      {nameInitial(user?.full_name ?? "")}
    </NavLink>
  );
}

/** Authenticated app shell: full sidebar at lg+, icon rail at md–lg, bottom tab bar under md. */
export function DashboardLayout() {
  const entries = useNavEntries();

  return (
    <div className="flex min-h-svh bg-cream-100">
      <aside className="hidden lg:flex lg:w-[236px] lg:shrink-0 lg:flex-col lg:border-r lg:border-line-200 lg:bg-white">
        <div className="p-6">
          <p className="font-display text-[22px] font-extrabold text-ink-900">Teka</p>
          <p className="text-[12px] text-ink-400">Quản lý lớp học</p>
        </div>
        <nav aria-label="Main" className="flex flex-1 flex-col gap-1 px-4">
          {entries.map((entry) => (
            <SidebarNavItem key={entry.label} {...entry} />
          ))}
        </nav>
        <CurrentPeriodCard />
        <SidebarFooter />
      </aside>

      <aside className="hidden md:flex md:w-[72px] md:shrink-0 md:flex-col md:items-center md:border-r md:border-line-200 md:bg-white lg:hidden">
        <nav aria-label="Main" className="flex flex-1 flex-col items-center gap-2 py-6">
          {entries.map((entry) => (
            <RailNavItem key={entry.label} {...entry} />
          ))}
        </nav>
        <ProfileDisc />
        <CurrentPeriodDisc />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <main
          id="main-content"
          className={cn(
            "flex-1 p-4 pb-24",
            "md:mx-auto md:w-full md:max-w-[var(--w-content)] md:p-6 md:pb-6",
            "lg:max-w-[var(--w-page)] lg:px-8 lg:py-7",
          )}
        >
          {/* At lg+ the sidebar footer owns profile + logout; below that the
              header row keeps both reachable for the rail and bottom tabs. */}
          <div className="mb-2 flex items-center justify-end gap-1 lg:hidden">
            <Link
              to="/profile"
              className="inline-flex items-center gap-2 rounded-[var(--radius-md)] px-3 py-2 text-[13px] font-semibold text-ink-500 transition-colors hover:bg-cream-100 hover:text-mint-600"
            >
              Hồ sơ giáo viên
            </Link>
            <LogoutButton />
          </div>
          <Outlet />
        </main>
      </div>

      <nav
        aria-label="Main"
        className="fixed inset-x-0 bottom-0 z-40 flex border-t border-line-200 bg-white md:hidden"
      >
        {entries.map((entry) => (
          <BottomTabItem key={entry.label} {...entry} />
        ))}
      </nav>
    </div>
  );
}
