import { z } from "zod";

// VITE_* variables are compiled into the public bundle — never put secrets
// here. Validation fails fast at module load so a misconfigured build dies at
// boot with a readable message instead of failing on the first request.
const envSchema = z.object({
  VITE_API_URL: z.url({ message: "must be an absolute URL, e.g. http://localhost:8080/api/v1" }),
});

const parsed = envSchema.safeParse(import.meta.env);

if (!parsed.success) {
  const details = parsed.error.issues
    .map((issue) => `  ${issue.path.join(".")}: ${issue.message}`)
    .join("\n");
  throw new Error(`Invalid environment configuration:\n${details}`);
}

export const env = parsed.data;

/** API origin (scheme + host) for endpoints outside the versioned base path. */
export const apiOrigin = new URL(env.VITE_API_URL).origin;
