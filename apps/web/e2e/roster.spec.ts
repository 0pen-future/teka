import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào Cô Lan/)).toBeVisible();
}

test("roster flow: contact, two students, a class, enroll both, end one", async ({ page }) => {
  // Timestamp suffix keeps this run's data unique even though the config
  // runs specs sequentially against one seeded stack (`playwright.config.ts`).
  const suffix = Date.now();
  const contactName = `E2E Phụ huynh ${suffix}`;
  const contactPhone = `09${String(suffix).slice(-8)}`;
  const studentOneName = `E2E Học sinh Một ${suffix}`;
  const studentTwoName = `E2E Học sinh Hai ${suffix}`;
  const studentThreeName = `E2E Học sinh Ba ${suffix}`;
  const className = `E2E Lớp ${suffix}`;

  await login(page);

  // 1. Create the contact (phone owner — PRD R1: phone lives on the contact).
  await page.goto("/contacts");
  await page.getByRole("button", { name: "Thêm người liên hệ" }).click();
  await page.getByLabel("Họ và tên").fill(contactName);
  await page.getByLabel("Số điện thoại").fill(contactPhone);
  await page.getByRole("button", { name: "Lưu" }).click();
  await expect(page.getByText(contactName)).toBeVisible();
  await page.getByText(contactName).click();
  await expect(page).toHaveURL(/\/contacts\/.+/);

  // 2. Add two students under that contact (closed field list: name + note only).
  await page.getByRole("button", { name: "Thêm học sinh" }).click();
  await page.getByLabel("Họ và tên").fill(studentOneName);
  await page.getByRole("dialog").getByRole("button", { name: "Thêm học sinh" }).click();
  await expect(page.getByText(studentOneName)).toBeVisible();

  await page.getByRole("button", { name: "Thêm học sinh" }).click();
  await page.getByLabel("Họ và tên").fill(studentTwoName);
  await page.getByRole("dialog").getByRole("button", { name: "Thêm học sinh" }).click();
  await expect(page.getByText(studentTwoName)).toBeVisible();

  // 3. Create a class with one weekly slot (`ClassDialog` create mode) —
  // class creation lives on the consolidated "Lớp & học sinh" screen; the
  // new class shows up as a pill tab in the class picker.
  await page.goto("/students");
  await page.getByRole("button", { name: "+ Tạo lớp mới" }).click();
  await page.getByLabel("Tên lớp").fill(className);
  await page.getByLabel("Giờ học").fill("18:00");
  await page.getByLabel("Thời lượng (phút)").fill("90");
  await page.getByLabel("Đơn giá / buổi (đ)").fill("150000");
  await page.getByRole("dialog").getByRole("button", { name: "Tạo lớp" }).click();
  await expect(page.getByRole("tab", { name: className })).toBeVisible();

  // 3b. Add-student wizard on the roster screen: create the profile (Bước
  // 1/2), postpone enrollment with "Để sau", find the student on the "Chưa
  // ghi danh" tab, then enroll from there (Bước 2 reused standalone).
  await page.getByRole("button", { name: "+ Thêm học sinh" }).click();
  await page.getByLabel("Họ và tên").fill(studentThreeName);
  await page.getByRole("combobox", { name: "Người liên hệ" }).fill(contactName);
  await page.getByRole("option", { name: new RegExp(contactName) }).click();
  await page.getByRole("button", { name: "Tiếp tục: Ghi danh →" }).click();
  await expect(page.getByText("Bước 2/2")).toBeVisible();
  await page.getByRole("button", { name: "Để sau" }).click();
  await expect(page.getByText(/Đã lưu hồ sơ — ghi danh sau/)).toBeVisible();
  await expect(page).toHaveURL(/class_id=none/);

  await page.getByPlaceholder("Tìm theo tên học sinh").fill(studentThreeName);
  const studentThreeRow = page.getByRole("row").filter({ hasText: studentThreeName });
  await expect(studentThreeRow.getByText("Chưa vào lớp nào")).toBeVisible();
  await studentThreeRow.getByRole("button", { name: "Ghi danh vào lớp" }).click();
  await page.getByRole("combobox", { name: "Lớp" }).click();
  await page.getByRole("option", { name: new RegExp(className) }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Ghi danh vào lớp" }).click();
  await expect(page.getByText(/tính tiền từ buổi có mặt đầu tiên/)).toBeVisible();

  // Enrolling moves the roster to that class's tab; the student is on it.
  await expect(page).toHaveURL(/class_id=(?!none)/);
  await expect(page.getByRole("row").filter({ hasText: studentThreeName })).toBeVisible();

  // 4. Enroll both remaining students from the "Chưa ghi danh" tab — the
  // per-class enrollment screen is gone; every enrollment goes through
  // `EnrollStudentDialog` for one student at a time. The success toast names
  // the student, which also disambiguates it from the previous toast.
  for (const studentName of [studentOneName, studentTwoName]) {
    await page.getByRole("tab", { name: "Chưa ghi danh" }).click();
    await page.getByPlaceholder("Tìm theo tên học sinh").fill(studentName);
    const studentRow = page.getByRole("row").filter({ hasText: studentName });
    await studentRow.getByRole("button", { name: "Ghi danh vào lớp" }).click();
    await page.getByRole("combobox", { name: "Lớp" }).click();
    await page.getByRole("option", { name: new RegExp(className) }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Ghi danh vào lớp" }).click();
    await expect(page.getByText(new RegExp(`Đã ghi danh ${studentName}`))).toBeVisible();
    // Enrolling switches the roster to the class's tab with the student on it.
    await expect(page.getByRole("row").filter({ hasText: studentName })).toBeVisible();
  }

  // 5. End one enrollment from the student detail screen — existing debt
  // (if any) must be preserved, not written off.
  await page.getByPlaceholder("Tìm theo tên học sinh").fill(studentOneName);
  await page.getByRole("link", { name: studentOneName }).click();
  await expect(page).toHaveURL(/\/students\/.+/);
  await page.getByRole("button", { name: "Kết thúc ghi danh" }).click();
  await expect(page.getByText("Nợ cũ (nếu có) vẫn được giữ.")).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Kết thúc" }).click();
  await expect(page.getByRole("button", { name: "Kết thúc ghi danh" })).toHaveCount(0);
  await expect(page.getByText(/— \d{4}-\d{2}-\d{2}/)).toBeVisible();

  // The other student's enrollment is untouched by ending the first one.
  await page.goto("/students");
  await page.getByRole("tab", { name: className }).click();
  await page.getByPlaceholder("Tìm theo tên học sinh").fill(studentTwoName);
  await page.getByRole("link", { name: studentTwoName }).click();
  await expect(page).toHaveURL(/\/students\/.+/);
  await expect(page.getByRole("button", { name: "Kết thúc ghi danh" })).toBeVisible();
});
