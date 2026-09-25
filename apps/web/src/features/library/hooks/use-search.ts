import { useEffect, useState } from "react";

/** Debounced search text: `query` is what the input shows, `q` what the list asks for. */
export function useSearch(initial = "") {
  const [query, setQuery] = useState(initial);
  const [q, setQ] = useState(initial.trim());
  useEffect(() => {
    const timer = setTimeout(() => setQ(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);
  return { query, setQuery, q };
}

export type SearchState = ReturnType<typeof useSearch>;
