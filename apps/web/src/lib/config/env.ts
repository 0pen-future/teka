import { z } from "zod";

// VITE_* variables are compiled into the public bundle — never put secrets
// here. Validation fails fast at module load so a misconfigured build dies at
// boot with a readable message instead of failing on the first request.
// Root-relative paths ("/api/v1") are valid: the compose stack serves the API
// same-origin through the Vite proxy.
const envSchema = z.object({
  VITE_API_URL: z.string().refine(
    // "//host/path" is protocol-relative (cross-origin), not root-relative.
    (value) =>
      (value.startsWith("/") && !value.startsWith("//")) || z.url().safeParse(value).success,
    {
      message: "must be an absolute URL or root-relative path, e.g. http://localhost:8080/api/v1",
    },
  ),
});

const parsed = envSchema.safeParse(import.meta.env);

if (!parsed.success) {
  const details = parsed.error.issues
    .map((issue) => `  ${issue.path.join(".")}: ${issue.message}`)
    .join("\n");
  throw new Error(`Invalid environment configuration:\n${details}`);
}

export const env = parsed.data;
