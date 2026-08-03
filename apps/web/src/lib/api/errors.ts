import { isAxiosError } from "axios";

/** Code used when the request never reached the API (offline, DNS, timeout). */
export const NETWORK_ERROR = "NETWORK_ERROR";

/**
 * The one error shape components ever see. Normalized from the backend's
 * {success, error: {code, message, fields}} envelope; raw axios errors never
 * escape the API layer.
 */
export class ApiError extends Error {
  readonly code: string;
  /** HTTP status, or null when the request got no response. */
  readonly status: number | null;
  /** Per-field validation messages (VALIDATION_ERROR responses). */
  readonly fields?: Record<string, string>;

  constructor(
    code: string,
    message: string,
    status: number | null,
    fields?: Record<string, string>,
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.fields = fields;
  }
}

interface ErrorBody {
  code?: string;
  message?: string;
  fields?: Record<string, string>;
}

/**
 * A failed response is not guaranteed to carry the API's JSON envelope — a
 * proxy 502 can return HTML, an empty body parses to null. Only trust it when
 * the shape is actually there.
 */
function extractErrorBody(data: unknown): ErrorBody | undefined {
  if (typeof data === "object" && data !== null && "error" in data) {
    const body: unknown = (data as { error?: unknown }).error;
    if (typeof body === "object" && body !== null) {
      return body;
    }
  }
  return undefined;
}

/** Normalize any thrown value into an ApiError. */
export function toApiError(err: unknown): ApiError {
  if (err instanceof ApiError) {
    return err;
  }
  if (isAxiosError(err)) {
    if (err.response) {
      const body = extractErrorBody(err.response.data);
      return new ApiError(
        body?.code ?? "UNKNOWN_ERROR",
        body?.message ?? "Something went wrong",
        err.response.status,
        body?.fields,
      );
    }
    return new ApiError(NETWORK_ERROR, "Cannot reach the server", null);
  }
  return new ApiError(
    "UNKNOWN_ERROR",
    err instanceof Error ? err.message : "Something went wrong",
    null,
  );
}
