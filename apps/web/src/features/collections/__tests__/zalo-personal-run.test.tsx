import { http, HttpResponse } from "msw";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { NotificationsPage } from "../pages/notifications-page";
import {
  collectionsHandlers,
  contactSingleChildOwing,
  failRunRowFor,
  fixturePeriod,
  holdRunProgress,
  interruptRun,
  markZaloNotFriend,
  resetCollectionsStore,
  seedOldRunFailure,
  seedRunMidFlight,
  setPreviewMaxRunSize,
} from "./collections-handlers";

/** Polling runs on a real 2s interval, so run-progress waits need headroom. */
const pollTimeout = { timeout: 6000 };

const linkedZaloHandler = http.get(`${API_URL}/me/zalo`, () =>
  HttpResponse.json(
    ok({
      linked: true,
      display_name: "Cô Lan",
      status: "linked",
      linked_at: "2026-08-01T08:00:00Z",
    }),
  ),
);

const expiredZaloHandler = http.get(`${API_URL}/me/zalo`, () =>
  HttpResponse.json(
    ok({
      linked: true,
      display_name: "Cô Lan",
      status: "expired",
      linked_at: "2026-08-01T08:00:00Z",
    }),
  ),
);

function renderNotificationsPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<NotificationsPage />, {
    route: `/notifications/${fixturePeriod.id}`,
    path: "/notifications/:periodId",
  });
}

/**
 * The confirm dialog's counts render across nested tags (`<strong>2</strong>
 * phụ huynh…`), so line assertions match on the whole element's textContent.
 */
function findDialogLine(text: string) {
  return screen.findByText((_, element) => element?.textContent === text);
}

/** Confirms the personal send once the preview resolves (Gửi is disabled until then). */
async function confirmSend(user: ReturnType<typeof userEvent.setup>) {
  const send = await screen.findByRole("button", { name: "Gửi" });
  await waitFor(() => expect(send).toBeEnabled());
  await user.click(send);
}

async function sendPersonal(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("Chưa có thông báo nào cho kỳ này.");
  await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
  await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));
  await confirmSend(user);
}

beforeEach(() => {
  resetCollectionsStore();
  server.use(...collectionsHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("NotificationsPage zalo_personal channel", () => {
  it("disables the auto-send option and points to the profile page while Zalo is unlinked", async () => {
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    expect(screen.getByRole("radio", { name: "Zalo thủ công" })).toBeChecked();
    const personalRadio = screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" });
    expect(personalRadio).toBeDisabled();
    expect(personalRadio).toHaveAttribute("aria-describedby", "send-channel-note");
    const note = screen.getByRole("link", { name: "Kết nối Zalo để gửi tự động" });
    expect(note).toHaveAttribute("href", "/profile");
    expect(note.closest("p")).toHaveAttribute("id", "send-channel-note");
  });

  it("explains itself when the Zalo status cannot be loaded", async () => {
    server.use(
      http.get(`${API_URL}/me/zalo`, () =>
        HttpResponse.json(fail("INTERNAL", "boom"), { status: 500 }),
      ),
    );
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    expect(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" })).toBeDisabled();
    expect(
      await screen.findByText("Không kiểm tra được trạng thái Zalo — chỉ gửi thủ công được."),
    ).toBeInTheDocument();
  });

  it("offers a re-scan link instead of the auto option when the session has expired", async () => {
    server.use(expiredZaloHandler);
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    expect(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" })).toBeDisabled();
    expect(screen.getByText("Phiên Zalo đã hết hạn.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Quét lại mã" })).toHaveAttribute("href", "/profile");
  });

  it("confirms with auto/manual counts, then tracks the run to completion while manual rows keep the copy flow", async () => {
    server.use(linkedZaloHandler);
    const user = userEvent.setup();
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));

    // Nothing may be sent before the teacher confirms the split. The counts
    // come from the server preview: 2 mapped friends, 2 unmapped.
    expect(await findDialogLine("2 phụ huynh gửi tự động (đã là bạn Zalo).")).toBeInTheDocument();
    expect(
      await findDialogLine("2 phụ huynh chưa ghép Zalo — dùng copy thủ công."),
    ).toBeInTheDocument();
    expect(screen.queryAllByRole("button", { name: "Sao chép" })).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Gửi" }));

    expect(await screen.findByText("Đang gửi 1/2…", undefined, pollTimeout)).toBeInTheDocument();
    expect(
      screen.getByText("2 phụ huynh chưa liên kết — dùng copy-paste bên dưới."),
    ).toBeInTheDocument();

    // Only the two unmapped contacts get copy-paste cards; the auto-sent
    // rows never enter the manual flow.
    expect(screen.getAllByRole("button", { name: "Sao chép" })).toHaveLength(2);
    expect(screen.queryByText(contactSingleChildOwing.full_name)).not.toBeInTheDocument();

    expect(await screen.findByText("Đã gửi 2 · Lỗi 0", undefined, pollTimeout)).toBeInTheDocument();
  }, 15000);

  it("counts only this period's still-owing contacts for a reminder send", async () => {
    server.use(linkedZaloHandler);
    const user = userEvent.setup();
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await user.click(screen.getByRole("tab", { name: "Nhắc nợ" }));
    await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
    await user.click(await screen.findByRole("button", { name: "Tạo nhắc nợ" }));

    // Hùng is fully paid, so the reminder pool is Lan + Mai (mapped) + Bình.
    expect(await findDialogLine("2 phụ huynh gửi tự động (đã là bạn Zalo).")).toBeInTheDocument();
    expect(
      await findDialogLine("1 phụ huynh chưa ghép Zalo — dùng copy thủ công."),
    ).toBeInTheDocument();
  });

  it("surfaces a failed row's reason from the ledger after the run finishes", async () => {
    server.use(linkedZaloHandler);
    failRunRowFor(contactSingleChildOwing.id, "Phiên Zalo đã hết hạn");
    const user = userEvent.setup();
    renderNotificationsPage();

    await sendPersonal(user);

    expect(await screen.findByText("Đã gửi 1 · Lỗi 1", undefined, pollTimeout)).toBeInTheDocument();
    expect(
      await screen.findByText(
        `${contactSingleChildOwing.full_name}: Phiên Zalo đã hết hạn`,
        undefined,
        pollTimeout,
      ),
    ).toBeInTheDocument();
  }, 15000);

  it("keeps an older run's failure out of the current run's banner", async () => {
    server.use(linkedZaloHandler);
    seedOldRunFailure("Lỗi lượt cũ");
    failRunRowFor(contactSingleChildOwing.id, "Phiên Zalo đã hết hạn");
    const user = userEvent.setup();
    renderNotificationsPage();

    // The old failed row keeps the ledger non-empty, so generation starts
    // from the header button instead of the empty state.
    await screen.findByRole("button", { name: "Tạo thông báo học phí" });
    await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));
    await confirmSend(user);

    expect(await screen.findByText("Đã gửi 1 · Lỗi 1", undefined, pollTimeout)).toBeInTheDocument();
    expect(
      await screen.findByText(
        `${contactSingleChildOwing.full_name}: Phiên Zalo đã hết hạn`,
        undefined,
        pollTimeout,
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Lỗi lượt cũ/)).not.toBeInTheDocument();
  }, 15000);

  it("refuses a second personal send while a run is still active", async () => {
    server.use(linkedZaloHandler);
    seedRunMidFlight();
    holdRunProgress();
    const user = userEvent.setup();
    renderNotificationsPage();

    expect(await screen.findByText("Đang gửi 1/2…", undefined, pollTimeout)).toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));
    await confirmSend(user);

    expect(await screen.findByText("Đang có lượt gửi chạy, đợi xong đã")).toBeInTheDocument();
  }, 15000);

  it("restores the banner from the snapshot when the page reopens mid-run", async () => {
    server.use(linkedZaloHandler);
    seedRunMidFlight();
    renderNotificationsPage();

    // No generate click — the snapshot alone must drive the banner.
    expect(await screen.findByText("Đã gửi 2 · Lỗi 0", undefined, pollTimeout)).toBeInTheDocument();
  }, 15000);

  it("shows the interrupted banner and resumes the run", async () => {
    server.use(linkedZaloHandler);
    seedRunMidFlight();
    interruptRun();
    const user = userEvent.setup();
    renderNotificationsPage();

    expect(
      await screen.findByText("Lượt gửi bị gián đoạn — còn 1 chưa gửi.", undefined, pollTimeout),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Gửi tiếp" }));

    expect(await screen.findByText("Đã gửi 2 · Lỗi 0", undefined, pollTimeout)).toBeInTheDocument();
  }, 15000);

  it("warns before a manual regenerate that would re-message auto-sent parents", async () => {
    server.use(linkedZaloHandler);
    seedRunMidFlight();
    const user = userEvent.setup();
    renderNotificationsPage();

    expect(await screen.findByText("Đã gửi 2 · Lỗi 0", undefined, pollTimeout)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));
    expect(
      await screen.findByText(
        "Kỳ này đã có lượt gửi tự động — tạo lại sẽ tạo tin copy-paste cho cả phụ huynh đã nhận qua Zalo. Tiếp tục?",
      ),
    ).toBeInTheDocument();
    // Nothing is generated until the teacher accepts the duplicate risk.
    expect(screen.queryAllByRole("button", { name: "Sao chép" })).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "Tạo lại" }));
    expect(await screen.findAllByRole("button", { name: "Sao chép" })).toHaveLength(4);
  }, 15000);

  it("hides a finished run's banner when dismissed", async () => {
    server.use(linkedZaloHandler);
    seedRunMidFlight();
    const user = userEvent.setup();
    renderNotificationsPage();

    expect(await screen.findByText("Đã gửi 2 · Lỗi 0", undefined, pollTimeout)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Ẩn" }));
    expect(screen.queryByText("Đã gửi 2 · Lỗi 0")).not.toBeInTheDocument();
  }, 15000);

  it("warns about mapped-but-not-friend parents and links the befriend flow", async () => {
    server.use(linkedZaloHandler);
    markZaloNotFriend(contactSingleChildOwing.id);
    const user = userEvent.setup();
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));

    // Lan moved out of auto_send into the not-friend bucket: 1 auto, 1 warned.
    expect(await findDialogLine("1 phụ huynh gửi tự động (đã là bạn Zalo).")).toBeInTheDocument();
    expect(
      screen.getByText(/phụ huynh đã ghép Zalo nhưng chưa là bạn bè của bạn/),
    ).toBeInTheDocument();
    // The befriend CTA points at the contacts page, which owns the flow.
    expect(screen.getByRole("link", { name: "Kết bạn trước" })).toHaveAttribute(
      "href",
      "/contacts",
    );
    // A not-friend split warns but never blocks — the send stays available.
    const send = screen.getByRole("button", { name: "Gửi" });
    await waitFor(() => expect(send).toBeEnabled());
  });

  it("blocks the send while the run would exceed max_run_size", async () => {
    server.use(linkedZaloHandler);
    // Both mapped contacts (2) queue into the run; a cap of 1 must block.
    setPreviewMaxRunSize(1);
    const user = userEvent.setup();
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));

    expect(
      await screen.findByText(
        "Vượt giới hạn 1 tin tự động mỗi lượt — hãy gửi thành các đợt nhỏ hơn.",
      ),
    ).toBeInTheDocument();
    // The confirm stays disabled — the server would reject this run anyway —
    // while cancel remains available.
    expect(screen.getByRole("button", { name: "Gửi" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Huỷ" })).toBeEnabled();
  });

  it("still allows sending when the preview itself fails", async () => {
    server.use(
      linkedZaloHandler,
      http.get(`${API_URL}/billing-periods/:id/notifications/preview`, () =>
        HttpResponse.json(fail("INTERNAL", "boom"), { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await user.click(screen.getByRole("radio", { name: "Gửi qua Zalo (tự động)" }));
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));

    // The dialog explains the degraded state but the server stays the
    // authority — the teacher can still send.
    expect(
      await screen.findByText(
        "Không kiểm tra được danh sách bạn bè Zalo — vẫn gửi được, nhưng tin tới người chưa là bạn có thể không đến nơi.",
      ),
    ).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Gửi" })).toBeEnabled());
  });

  it("polls the run endpoint exactly once when the period has no run", async () => {
    let calls = 0;
    server.use(
      http.get(`${API_URL}/billing-periods/:id/notifications/run`, () => {
        calls += 1;
        return HttpResponse.json(ok({ active: false, run_id: null, total: 0, sent: 0, failed: 0 }));
      }),
    );
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await waitFor(() => expect(calls).toBe(1));
    // Longer than one 2s poll interval: a live interval would have refetched.
    await new Promise((resolve) => setTimeout(resolve, 2600));
    expect(calls).toBe(1);
  }, 10000);
});
