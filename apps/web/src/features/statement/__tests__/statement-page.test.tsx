import { getDefaultNormalizer, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { formatMoney } from "@/lib/utils";
import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { StatementPage } from "../pages/statement-page";

// `formatMoney` renders a non-breaking space before "₫" (Intl's vi-VN
// currency format). RTL's default text normalizer collapses that into a
// regular space when reading the DOM but leaves the raw search string
// untouched, so an exact `findByText(formatMoney(...))` never matches unless
// whitespace-collapsing is turned off here too.
const moneyMatcher = { normalizer: getDefaultNormalizer({ collapseWhitespace: false }) };

describe("StatementPage", () => {
  it("renders the statement for a valid token", async () => {
    renderWithProviders(<StatementPage />, { route: "/s/valid-token", path: "/s/:token" });

    expect(await screen.findByText("Chị Hoa")).toBeInTheDocument();
    expect(screen.getByText("Nguyễn Văn An")).toBeInTheDocument();
    // The grand total renders the server's `total_due` verbatim, not a
    // client-side recomputation. The QR panel repeats the same figure, so
    // more than one match is expected — the requirement is that it appears.
    expect(screen.getAllByText(formatMoney(1_800_000), moneyMatcher).length).toBeGreaterThanOrEqual(
      1,
    );
  });

  it("renders the neutral error for an unknown token, leaking no student name or status code", async () => {
    renderWithProviders(<StatementPage />, { route: "/s/unknown-token", path: "/s/:token" });

    expect(await screen.findByText("Không mở được liên kết này.")).toBeInTheDocument();
    expect(screen.queryByText(/Nguyễn|Trần|Phạm|Lê/)).not.toBeInTheDocument();
    expect(screen.queryByText(/404/)).not.toBeInTheDocument();
  });

  it("renders byte-identical error text for a server failure as for an unknown token", async () => {
    server.use(
      http.get(`${API_URL}/public/statements/:token`, () =>
        HttpResponse.json(fail("INTERNAL", "boom"), { status: 500 }),
      ),
    );

    renderWithProviders(<StatementPage />, { route: "/s/valid-token", path: "/s/:token" });

    expect(await screen.findByText("Không mở được liên kết này.")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Liên kết có thể đã hết hạn. Vui lòng liên hệ thầy/cô để nhận liên kết mới.",
      ),
    ).toBeInTheDocument();
  });

  it("adds a noindex robots meta tag while mounted and removes it on unmount", async () => {
    const { unmount } = renderWithProviders(<StatementPage />, {
      route: "/s/valid-token",
      path: "/s/:token",
    });

    await screen.findByText("Chị Hoa");
    const meta = document.querySelector('meta[name="robots"]');
    expect(meta).not.toBeNull();
    expect(meta?.getAttribute("content")).toBe("noindex, nofollow");

    unmount();
    expect(document.querySelector('meta[name="robots"]')).toBeNull();
  });

  it("never requests /auth/refresh while rendering the statement route", async () => {
    let refreshRequested = false;
    server.use(
      http.post(`${API_URL}/auth/refresh`, () => {
        refreshRequested = true;
        return HttpResponse.json(fail("UNAUTHORIZED", "invalid refresh token"), { status: 401 });
      }),
    );

    renderWithProviders(<StatementPage />, { route: "/s/valid-token", path: "/s/:token" });

    await screen.findByText("Chị Hoa");
    // `SessionRestore` wraps the whole app above the router in
    // `app/providers.tsx`, but this render harness mounts the route element
    // directly without it — so a passing assertion here only proves the
    // statement feature itself never calls `/auth/refresh`. The actual
    // route-level skip (SessionRestore checking `window.location.pathname`)
    // is covered by `features/auth/__tests__/session-restore.test.tsx`.
    expect(refreshRequested).toBe(false);
  });
});
