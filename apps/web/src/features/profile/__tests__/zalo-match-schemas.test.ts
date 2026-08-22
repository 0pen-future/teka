import { describe, expect, it } from "vitest";

import { zaloFriendMatchSchema } from "../schemas/zalo-schemas";

describe("zaloFriendMatchSchema", () => {
  it("parses a fully-populated matched row", () => {
    const row = zaloFriendMatchSchema.parse({
      phone: "+84912345678",
      matched: true,
      user_id: "zl-user-101",
      display_name: "Mẹ Lan",
      zalo_name: "Lan Nguyen",
      avatar: "https://example.com/a.png",
      is_friend: true,
    });
    expect(row.matched).toBe(true);
    expect(row.user_id).toBe("zl-user-101");
    expect(row.is_friend).toBe(true);
  });

  it("parses an unmatched row carrying only the echoed phone", () => {
    // The server omits every optional field on a miss (`omitempty`).
    const row = zaloFriendMatchSchema.parse({
      phone: "0912345678",
      matched: false,
      is_friend: false,
    });
    expect(row.phone).toBe("0912345678");
    expect(row.user_id).toBeUndefined();
  });

  it("tolerates unknown fields the API may add later", () => {
    const row = zaloFriendMatchSchema.parse({
      phone: "+84912345678",
      matched: true,
      user_id: "zl-user-101",
      display_name: "Mẹ Lan",
      is_friend: false,
      gender: 1,
    });
    expect(row.display_name).toBe("Mẹ Lan");
  });

  it("rejects a row without the join key", () => {
    const parsed = zaloFriendMatchSchema.safeParse({ matched: true, is_friend: true });
    expect(parsed.success).toBe(false);
  });
});
