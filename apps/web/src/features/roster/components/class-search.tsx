/**
 * Prototype "CHỌN LỚP" pieces — pair with `useClassSearch` for the reveal
 * threshold, filtering, and empty-note text.
 */
export function ClassSearchInput({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <input
      type="search"
      aria-label="Tìm lớp"
      placeholder="Tìm lớp…"
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="w-[150px] rounded-full border-2 border-line-200 bg-white px-4 py-2 text-[13.5px] font-bold text-ink-700 outline-none placeholder:text-ink-400 focus:border-mint-400"
    />
  );
}

export function ClassSearchEmptyNote({ note }: { note: string }) {
  return <p className="text-[13px] font-bold text-ink-400">{note}</p>;
}
