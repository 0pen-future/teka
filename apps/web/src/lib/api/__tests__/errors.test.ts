import { AxiosError, AxiosHeaders, type AxiosResponse } from "axios";
import { describe, expect, it } from "vitest";

import { ApiError, NETWORK_ERROR, toApiError } from "../errors";

/** An axios failure carrying `body` as the parsed response payload. */
function axiosFailure(status: number, body: unknown): AxiosError {
  const config = { headers: new AxiosHeaders() };
  const response = {
    data: body,
    status,
    statusText: "",
    headers: {},
    config,
  } as AxiosResponse;
  return new AxiosError("Request failed", String(status), config, {}, response);
}

describe("toApiError", () => {
  it("surfaces the envelope's details untouched", () => {
    const details = { errors: [{ sheet: "Lop", line: 3, code: "TOO_LONG", message: "quá dài" }] };
    const err = toApiError(
      axiosFailure(422, {
        success: false,
        error: { code: "VALIDATION_ERROR", message: "sai", details },
      }),
    );

    expect(err.code).toBe("VALIDATION_ERROR");
    expect(err.status).toBe(422);
    // Deliberately unvalidated here: the owning feature parses it with its
    // own schema, so the shared layer must not reshape or drop anything.
    expect(err.details).toEqual(details);
  });

  it("leaves details undefined on an envelope without them", () => {
    const err = toApiError(
      axiosFailure(403, {
        success: false,
        error: { code: "FORBIDDEN", message: "không có quyền" },
      }),
    );

    expect(err.code).toBe("FORBIDDEN");
    expect(err.details).toBeUndefined();
    expect(err.fields).toBeUndefined();
  });

  it("keeps fields and details side by side", () => {
    const err = toApiError(
      axiosFailure(422, {
        success: false,
        error: {
          code: "VALIDATION_ERROR",
          message: "sai",
          fields: { name: "bắt buộc" },
          details: { errors: [] },
        },
      }),
    );

    expect(err.fields).toEqual({ name: "bắt buộc" });
    expect(err.details).toEqual({ errors: [] });
  });

  it("falls back when the body is not the envelope", () => {
    const err = toApiError(axiosFailure(502, "<html>bad gateway</html>"));

    expect(err.code).toBe("UNKNOWN_ERROR");
    expect(err.status).toBe(502);
    expect(err.details).toBeUndefined();
  });

  it("reports a request that never got a response", () => {
    const config = { headers: new AxiosHeaders() };
    const err = toApiError(new AxiosError("timeout", "ECONNABORTED", config));

    expect(err.code).toBe(NETWORK_ERROR);
    expect(err.status).toBeNull();
  });

  it("passes an ApiError through unchanged", () => {
    const original = new ApiError("CONFLICT", "đang chạy", 409, undefined, { busy: true });

    expect(toApiError(original)).toBe(original);
  });
});
