import { expect, test, type Page } from "@playwright/test";

import { loginAsMember, loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center and teaches "Văn 9 - Sáng Thứ Bảy"
// with no other staff on it, so inviting Thầy Minh as giáo viên and letting
// the owner confirm the invitation hands that class over to him. The
// restoring afterEach hands it back through the detail card, and the
// pre-clean cancels any invitation a dead earlier run left open, which keeps
// the journey idempotent on a reused database.
const INVITE_CLASS = "Văn 9 - Sáng Thứ Bảy";
const OWNER_NAME = "Cô Lan";
const MEMBER_NAME = "Thầy Minh";

/** Resolves the class id from the Giảng dạy class list (owner-readable). */
async function classIdFromClassList(page: Page, className: string): Promise<string> {
  await page.goto("/classes");
  await page.getByRole("link", { name: className, exact: true }).click();
  await expect(page).toHaveURL(/\/classes\/[0-9a-f-]+$/);
  const classId = page.url().split("/classes/")[1];
  expect(classId).toBeTruthy();
  return classId ?? "";
}

/** The invitation row for Thầy Minh on the invite class, narrowed by its status label. */
function invitationRow(page: Page, status: string) {
  return page
    .getByRole("row")
    .filter({ hasText: INVITE_CLASS })
    .filter({ hasText: MEMBER_NAME })
    .filter({ hasText: status });
}

/**
 * Cancels every open (pending or accepted) invitation for Thầy Minh on the
 * invite class so the send below never hits the duplicate-invitation 409.
 */
async function cancelOpenInvitations(page: Page) {
  await page.goto("/class-invitations");
  await expect(page.getByRole("heading", { name: "Lời mời nhận lớp" })).toBeVisible();
  for (const status of ["Đang chờ", "Đã đồng ý"]) {
    while ((await invitationRow(page, status).count()) > 0) {
      await invitationRow(page, status).first().getByRole("button", { name: "Hủy" }).click();
      await page.getByRole("button", { name: "Hủy lời mời" }).click();
      await expect(page.getByText(`Đã hủy lời mời của ${MEMBER_NAME}`)).toBeVisible();
      await expect(page.getByText(`Đã hủy lời mời của ${MEMBER_NAME}`)).toBeHidden();
    }
  }
}

/**
 * Reads the class-detail handoff card and, when the current teacher differs
 * from `targetName`, hands the class over. Assert-then-set keeps the restore
 * idempotent no matter where the journey stopped.
 */
async function ensureClassTeacher(
  page: Page,
  classId: string,
  targetName: string,
  targetOptionLabel: string,
) {
  await page.goto(`/classes/${classId}`);
  const card = page.locator("#teacher-handoff");
  const current = card.getByText("Giáo viên hiện tại:");
  await expect(current).toBeVisible();
  if ((await current.innerText()).includes(targetName)) {
    return;
  }
  await card.getByLabel("Bàn giao cho").click();
  await page.getByRole("option", { name: targetOptionLabel, exact: true }).click();
  await card.getByRole("button", { name: "Bàn giao lớp", exact: true }).click();
  await card.getByRole("button", { name: "Xác nhận bàn giao" }).click();
  await expect(page.getByText(new RegExp(`Đã bàn giao lớp cho ${targetName}`))).toBeVisible();
}

// Hand Văn 9 back to Cô Lan and close any invitation still open, so the seed
// state holds for the following specs and the next run.
test.afterEach(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await loginAsOwner(page);
    const classId = await classIdFromClassList(page, INVITE_CLASS);
    await ensureClassTeacher(page, classId, OWNER_NAME, `${OWNER_NAME} (chủ trung tâm)`);
    await cancelOpenInvitations(page);
  } finally {
    await context.close();
  }
});

test("owner invites a member, the member accepts, and the owner confirms the handover", async ({
  browser,
}) => {
  const ownerContext = await browser.newContext();
  const owner = await ownerContext.newPage();
  await loginAsOwner(owner);
  const classId = await classIdFromClassList(owner, INVITE_CLASS);
  await ensureClassTeacher(owner, classId, OWNER_NAME, `${OWNER_NAME} (chủ trung tâm)`);
  await cancelOpenInvitations(owner);

  // Send the invitation from the class detail's teaching-team card.
  await owner.goto(`/classes/${classId}`);
  const team = owner.getByRole("region", { name: "Đội ngũ giảng dạy" });
  await team.getByRole("button", { name: "+ Mời GV" }).click();
  const inviteDialog = owner.getByRole("dialog", { name: "Mời thành viên vào lớp" });
  await inviteDialog.getByRole("combobox", { name: "Thành viên" }).click();
  // The option list portals to `body`, outside the dialog's DOM subtree.
  await owner.getByRole("option", { name: new RegExp(MEMBER_NAME) }).click();
  await inviteDialog.getByLabel("Lời nhắn").fill("Nhờ thầy nhận lớp Văn 9 giúp nhé.");
  await inviteDialog.getByRole("button", { name: "Gửi lời mời" }).click();
  await expect(owner.getByText(`Đã gửi lời mời cho ${MEMBER_NAME}`)).toBeVisible();
  await expect(
    team.getByRole("listitem", { name: /Thầy Minh — Giáo viên, Chờ nhận/ }),
  ).toBeVisible();

  // The invitee sees it under Lời mời nhận lớp and accepts; no assignment yet.
  const memberContext = await browser.newContext();
  const member = await memberContext.newPage();
  await loginAsMember(member);
  await member.goto("/class-invitations");
  const pending = invitationRow(member, "Đang chờ");
  await expect(pending).toHaveCount(1);
  await expect(pending).toContainText("Nhờ thầy nhận lớp Văn 9 giúp nhé.");
  await pending.getByRole("button", { name: "Chấp nhận" }).click();
  await expect(member.getByText(`Đã nhận lời mời lớp ${INVITE_CLASS}`)).toBeVisible();
  await expect(invitationRow(member, "Đã đồng ý")).toHaveCount(1);
  await memberContext.close();

  // The owner confirms: the dialog names the teacher being replaced, and the
  // class hands over with its upcoming sessions.
  await owner.goto("/class-invitations");
  const accepted = invitationRow(owner, "Đã đồng ý");
  await expect(accepted).toHaveCount(1);
  await accepted.getByRole("button", { name: "GV nhận lớp" }).click();
  const confirmDialog = owner.getByRole("dialog", {
    name: `${MEMBER_NAME} nhận lớp ${INVITE_CLASS}`,
  });
  await expect(confirmDialog).toContainText(`thay ${OWNER_NAME}`);
  await confirmDialog.getByRole("button", { name: "Xác nhận" }).click();
  await expect(
    owner.getByText(
      new RegExp(`${MEMBER_NAME} đã nhận lớp ${INVITE_CLASS} · chuyển \\d+ buổi sắp tới`),
    ),
  ).toBeVisible();
  await expect(invitationRow(owner, "Đã phân công")).toHaveCount(1);

  // The teaching team on the class detail now lists Thầy Minh as giáo viên.
  await owner.goto(`/classes/${classId}`);
  const teamAfter = owner.getByRole("region", { name: "Đội ngũ giảng dạy" });
  await expect(teamAfter.getByText(MEMBER_NAME, { exact: true })).toBeVisible();
  await expect(teamAfter.getByText("Giáo viên", { exact: true })).toBeVisible();
  await expect(teamAfter.getByText("Không có lời mời nào đang mở.")).toBeVisible();
  await ownerContext.close();
});
