import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";

import { aliceUser, API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testAdmin, testUser } from "@/test/utils";

import { EditUserDialog } from "../components/edit-user-dialog";

function capturePatch() {
  const captured: { body: Record<string, unknown> | null; id: string | null } = {
    body: null,
    id: null,
  };
  server.use(
    http.patch(`${API_URL}/users/:id`, async ({ request, params }) => {
      captured.body = (await request.json()) as Record<string, unknown>;
      captured.id = String(params.id);
      return HttpResponse.json(ok({ ...aliceUser, ...captured.body }));
    }),
  );
  return captured;
}

describe("EditUserDialog", () => {
  it("sends only the changed name, never the role, for non-admin edits", async () => {
    const captured = capturePatch();
    signInAs(testUser);
    const onOpenChange = vi.fn();
    renderWithProviders(
      <EditUserDialog user={aliceUser} onOpenChange={onOpenChange} canEditRole={false} />,
    );
    const user = userEvent.setup();

    expect(screen.queryByLabelText("Role")).not.toBeInTheDocument();

    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Alice Renamed");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(captured.body).toEqual({ name: "Alice Renamed" });
    });
    expect(captured.id).toBe(aliceUser.id);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("includes the role in the patch when an admin changes it", async () => {
    const captured = capturePatch();
    signInAs(testAdmin);
    const onOpenChange = vi.fn();
    renderWithProviders(
      <EditUserDialog user={aliceUser} onOpenChange={onOpenChange} canEditRole />,
    );
    const user = userEvent.setup();

    await user.click(screen.getByLabelText("Role"));
    await user.click(await screen.findByRole("option", { name: "Admin" }));
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(captured.body).toEqual({ role: "admin" });
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("closes without a request when nothing changed", async () => {
    const captured = capturePatch();
    signInAs(testAdmin);
    const onOpenChange = vi.fn();
    renderWithProviders(
      <EditUserDialog user={aliceUser} onOpenChange={onOpenChange} canEditRole />,
    );
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
    expect(captured.body).toBeNull();
  });
});
