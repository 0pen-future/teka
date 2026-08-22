import { useMemo, useState } from "react";

/** The minimal class shape the picker needs — both screens' lists satisfy it. */
interface NamedClass {
  id: string;
  name: string;
}

/**
 * Prototype "CHỌN LỚP" behavior shared by the Điểm danh and Lớp & học sinh
 * screens: the "Tìm lớp…" input only appears once the class list outgrows 5
 * pills, filters tabs by case-insensitive substring on the class name, and
 * reports an inline note when nothing matches. While the input is hidden the
 * query is ignored entirely, so a stale query can never filter invisibly if
 * the list later shrinks below the threshold.
 */
export function useClassSearch<T extends NamedClass>(classes: T[]) {
  const [query, setQuery] = useState("");
  const showSearch = classes.length > 5;
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!showSearch || !q) {
      return classes;
    }
    return classes.filter((klass) => klass.name.toLowerCase().includes(q));
  }, [classes, query, showSearch]);
  const emptyNote =
    showSearch && query.trim() !== "" && filtered.length === 0
      ? `Không có lớp nào khớp "${query}"`
      : null;
  return { query, setQuery, filtered, showSearch, emptyNote };
}
