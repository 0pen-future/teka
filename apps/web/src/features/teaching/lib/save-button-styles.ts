/**
 * Shared save-button classnames for the teaching classbook's dirty/idle save
 * actions (session note, general score, component score). Kept out of
 * `session-detail-panel.tsx` so `component-score-grid.tsx` can reuse them
 * without a circular import between the panel and the grid it renders.
 */
export const saveButtonActive =
  "cursor-pointer rounded-[14px] bg-mint-400 px-5 py-[9px] text-[13px] font-extrabold text-white shadow-press-mint transition-transform active:translate-y-[3px] active:shadow-none";
export const saveButtonIdle =
  "cursor-default rounded-[14px] bg-cream-200 px-5 py-[9px] text-[13px] font-extrabold text-ink-400";
