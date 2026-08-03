import type { FieldValues, Path, UseFormReturn } from "react-hook-form";
import { toast } from "sonner";

import { ApiError } from "@/lib/api/errors";

interface ApiFormErrorOptions<T extends FieldValues> {
  /**
   * Where a CONFLICT error should land. The API reports duplicates (e.g.
   * "email already in use") as CONFLICT with a plain message, not as a
   * field-level VALIDATION_ERROR, but users should still see it on the input
   * that caused it.
   */
  conflictField?: Path<T>;
}

/**
 * Returns a mutation onError handler that maps ApiError.fields onto the
 * matching inputs via setError, routes CONFLICT to the configured field, and
 * falls back to a form-level root error for everything else.
 */
export function useApiFormErrors<T extends FieldValues>(
  form: UseFormReturn<T>,
  options?: ApiFormErrorOptions<T>,
): (error: unknown) => void {
  return (error: unknown) => {
    if (!(error instanceof ApiError)) {
      toast.error("Something went wrong");
      return;
    }
    if (error.fields && Object.keys(error.fields).length > 0) {
      const known = new Set(Object.keys(form.getValues()));
      for (const [field, message] of Object.entries(error.fields)) {
        if (known.has(field)) {
          form.setError(field as Path<T>, { type: "server", message });
        } else {
          form.setError("root", { type: "server", message: `${field}: ${message}` });
        }
      }
      return;
    }
    if (error.code === "CONFLICT" && options?.conflictField) {
      form.setError(options.conflictField, { type: "server", message: error.message });
      return;
    }
    form.setError("root", { type: "server", message: error.message });
  };
}
