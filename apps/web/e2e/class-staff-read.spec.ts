import { expect, test, type Page } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
// The seeder staffs "Toán 8 - Tối Thứ Ba" (owned and taught by Cô Lan) with
// Cô Thu as hoc_vu and Thầy Minh as tro_giang, so both members can read the
// class end to end while every write trigger stays owner/giao_vien-only.
const STAFF_CLASS = "Toán 8 - Tối Thứ Ba";
const HOC_VU = { phone: "0901000003", password: "thu-password", name: "Cô Thu" };
const TRO_GIANG = { phone: "0901000002", password: "minh-password", name: "Thầy Minh" };

async function login(page: Page, user: { phone: string; password: string; name: string }) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(user.phone);
  await page.getByLabel("Mật khẩu").fill(user.password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(
    page.getByText(new RegExp(`Chào buổi (sáng|trưa|chiều|tối), ${user.name}!`)),
  ).toBeVisible();
}

/**
 * The read journey both staff roles share: the assigned class is listed with
 * its roster, class settings render read-only, and the classbook opens
 * without edit affordances. Purely read-only — it must not mutate the shared
 * seeded stack. Attendance is asserted per role below, because the two staff
 * roles diverge there: tro_giang may confirm attendance, hoc_vu may not.
 */
async function assertStaffReadJourney(page: Page) {
  // Roster: the assigned class appears as a picker tab and its active
  // enrollments are readable.
  await page.goto("/students");
  await page.getByRole("tab", { name: STAFF_CLASS }).click();
  await expect(page).toHaveURL(/class_id=(?!none)/);
  await expect(page.getByRole("row").filter({ hasText: "Bé An" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Bé Bình" })).toBeVisible();
  const classId = new URL(page.url()).searchParams.get("class_id");
  expect(classId).toBeTruthy();

  // Class settings: readable, but saving is reserved for giao_vien/owner.
  await page.goto(`/classes/${classId}/settings`);
  await expect(page.getByLabel("Tên lớp")).toHaveValue(STAFF_CLASS);
  await expect(
    page.getByText("Chỉ giáo viên phụ trách hoặc chủ trung tâm mới sửa được cài đặt lớp."),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Lưu thay đổi" })).toBeDisabled();

  // Classbook: the class is selectable and its teaching data loads, with the
  // curriculum edit link hidden for non-writers.
  await page.goto(`/classbook?class_id=${classId}`);
  await expect(page.getByRole("tab", { name: STAFF_CLASS })).toBeVisible();
  await page.getByRole("tab", { name: "Chương trình & giáo án" }).click();
  await expect(page.getByText("CHƯƠNG TRÌNH", { exact: true })).toBeVisible();
  await expect(page.getByText(/Sửa chương trình/)).toHaveCount(0);
}

/**
 * Opens the attendance sheet of a past session of the staffed class. A
 * cancelled session renders no attendance sheet, so pick a row whose status
 * mentions điểm danh (past session, confirmed or not).
 */
async function openStaffAttendanceSheet(page: Page) {
  await page.goto("/sessions");
  await page.getByRole("tab", { name: STAFF_CLASS }).click();
  // The trio picker (and the "Cần điểm danh" shortcut) reach past sessions
  // without any date filtering — a past card's status mentions "điểm danh"
  // (confirmed or not), while upcoming reads "Sắp tới" and cancelled "Đã huỷ".
  const sessionRow = page
    .getByRole("link")
    .filter({ hasText: /điểm danh/ })
    .first();
  await expect(sessionRow).toBeVisible();
  await sessionRow.click();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);
  await expect(page.getByRole("radiogroup").first()).toBeVisible();
}

test("hoc_vu reads the assigned class everywhere but cannot write", async ({ page }) => {
  await login(page, HOC_VU);
  await assertStaffReadJourney(page);

  // Attendance stays frozen for hoc_vu: the sheet is readable, but the
  // confirm bar is the role-gate label and cancelling is out of reach.
  await openStaffAttendanceSheet(page);
  const confirmButton = page.getByRole("button", {
    name: "CHỈ GIÁO VIÊN, TRỢ GIẢNG LỚP HOẶC CHỦ TRUNG TÂM MỚI XÁC NHẬN ĐƯỢC",
  });
  await expect(confirmButton).toBeVisible();
  await expect(confirmButton).toBeDisabled();
  await expect(page.getByRole("button", { name: "Huỷ buổi học" })).toHaveCount(0);
});

test("tro_giang reads the assigned class and holds a live attendance confirm", async ({ page }) => {
  await login(page, TRO_GIANG);
  await assertStaffReadJourney(page);

  // Attendance is tro_giang's one write surface: the confirm bar offers a
  // real save (never the role-gate label). Cancelling a session is a
  // lifecycle write (sessions.write) and stays giao_vien/owner-only, so the
  // cancel affordance must not render. The actual write round-trip lives in
  // class-staff-write.spec.ts — this journey stays read-only.
  await openStaffAttendanceSheet(page);
  await expect(
    page.getByRole("button", {
      name: "CHỈ GIÁO VIÊN, TRỢ GIẢNG LỚP HOẶC CHỦ TRUNG TÂM MỚI XÁC NHẬN ĐƯỢC",
    }),
  ).toHaveCount(0);
  // Anchored so the disabled role-gate label (which also contains the words
  // "XÁC NHẬN") can never satisfy this locator.
  const confirmButton = page.getByRole("button", {
    name: /^(XÁC NHẬN|ĐÃ XÁC NHẬN ✓|LƯU VÀ TẠO ĐIỀU CHỈNH)( · .+)?$/,
  });
  await expect(confirmButton).toBeVisible();
  await expect(confirmButton).toBeEnabled();
  await expect(page.getByRole("button", { name: "Huỷ buổi học" })).toHaveCount(0);
});
