import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { renderWithProviders } from "@/test/utils";

import { MessageCard } from "../components/message-card";
import type { BulkSendRow } from "../schemas/collections-schemas";

const baseRow: BulkSendRow = {
  notification_id: "notification-1",
  contact_id: "contact-1",
  contact_name: "Chị Lan",
  phone: "+84987654321",
  channel: "zalo_manual",
  purpose: "statements",
  status: "queued",
  message_text: "Kính gửi phụ huynh",
  url: "/s/token",
  collapsed: false,
};

describe("MessageCard", () => {
  it("shows the contact's phone when the API returns it", () => {
    renderWithProviders(<MessageCard row={baseRow} onMarkSent={vi.fn()} />);
    expect(screen.getByText("+84987654321")).toBeInTheDocument();
  });

  it("renders no phone line when the API masks it", () => {
    renderWithProviders(<MessageCard row={{ ...baseRow, phone: null }} onMarkSent={vi.fn()} />);
    expect(screen.getByText("Chị Lan")).toBeInTheDocument();
    expect(screen.queryByText(/\+84/)).not.toBeInTheDocument();
  });
});
