import { z } from "zod";

/** Pagination block the API attaches to list responses. */
export const metaSchema = z.object({
  page: z.number().int(),
  per_page: z.number().int(),
  total: z.number().int(),
  total_pages: z.number().int(),
});

export type Meta = z.infer<typeof metaSchema>;

export interface Paginated<T> {
  items: T[];
  meta: Meta;
}

/**
 * Parse a success envelope's data payload. Interceptors already normalize
 * failures, so by the time a response body reaches feature code success is
 * implied; the zod parse catches contract drift loudly instead of letting a
 * shape change surface as undefined fields deep in a component.
 */
export function parseData<T>(schema: z.ZodType<T>, body: unknown): T {
  const envelope = z.object({ success: z.literal(true), data: schema }).parse(body);
  return envelope.data;
}

/**
 * Parse a success envelope carrying an unpaginated array payload. For the few
 * list endpoints that answer without a `meta` block — sending one through
 * `parseList` would reject every real response.
 */
export function parseArray<T>(schema: z.ZodType<T>, body: unknown): T[] {
  const envelope = z.object({ success: z.literal(true), data: z.array(schema) }).parse(body);
  return envelope.data;
}

/** Parse a success envelope carrying a list payload plus pagination meta. */
export function parseList<T>(schema: z.ZodType<T>, body: unknown): Paginated<T> {
  const envelope = z
    .object({ success: z.literal(true), data: z.array(schema), meta: metaSchema })
    .parse(body);
  return { items: envelope.data, meta: envelope.meta };
}
