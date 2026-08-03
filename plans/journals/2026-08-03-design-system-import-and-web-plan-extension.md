---
title: Design system imported from Claude Design — web plans extended for 100% DS adherence and responsive tiers
date: 2026-08-03
summary: "Imported the Học Vui Mỗi Ngày DS (Dịu Mát direction) + Sổ Lớp prototype via claude_design MCP; created foundation plan 260803-2325 (tokens/fonts/component kit) and rewrote plans 07/08 with prototype-mapped design specs and a 3-tier responsive strategy"
---

# Design system imported from Claude Design — web plans extended for 100% DS adherence and responsive tiers

## What happened

The user imported a Claude Design project ("Sổ Lớp" prototype + "Học Vui Mỗi Ngày" design system, direction "Dịu Mát") and asked for plans 07 (teacher app) and 08 (parent statement page) to follow it 100%. Instead of duplicating DS integration in both plans, a new P1 foundation plan `260803-2325-web-design-system-foundation` was created that both plans are now `blockedBy`:

- **Phase 1 — tokens/fonts/theme**: copy the four DS token CSS files verbatim into `apps/web/src/styles/tokens/`, self-host Baloo 2 + Nunito via Fontsource (replacing geist, zero Google Fonts requests), bridge tokens into Tailwind v4 `@theme inline` referencing `var(--…)` only, remap shadcn semantic variables (background→cream-100, primary→mint-400, destructive→coral-400, ring→mint-200, radius→20px).
- **Phase 2 — component kit** `apps/web/src/components/hv/`: HvButton (5 variants with the chunky `0 5px 0` press shadow + translateY(5px) active), HvCard, StatusPill (paid/partial/unpaid trio byte-matching the prototype's PILLS/STATUS_L maps), HvBadge, StatPill, ProgressBar, HvModal (wraps shadcn Dialog), HvToast, HvIcon; grep gate bans raw hex.

Plans 07/08 each gained a "Design Source — 100% Adherence" section. Plan 07's IA now follows the prototype's 6-section nav (Tổng quan, Điểm danh, Lớp & học sinh, Chốt sổ, Gửi thông báo, Thu tiền); roster consolidates into one `/students` screen with modal create flows; every phase carries a prototype-mapped Design Spec. Plan 08's screen authority is the prototype's parent preview modal.

After the initial handoff gate, the user asked for tablet/desktop responsive support. All three plans were updated with a 3-tier strategy on Tailwind defaults: phone `<md` bottom tab bar; tablet `md–lg` 72px icon rail + content centered at `--w-content`; desktop `lg+` full 236px sidebar + `--w-page` 1080px. HvModal ships responsive built-in (bottom sheet under `sm`, centered ≤480px panel above); hover affordances live behind `@media (hover: hover)`; attendance two-pane is `lg+`-only; plan 08 stays a single centered 390–480px column at all widths.

## Decisions

- Prototype IA wins over plan 07's original route design; original detail routes survive off-nav.
- Prototype demo behaviors are not product: one-tap send is replaced by the PRD's `zalo_manual` copy flow ("Sao chép tất cả chưa gửi").
- The tablet tier (72px icon rail) is an invention — the prototype defines only desktop and phone-panel layouts — anchored to DS width tokens.
- Attendance two-pane moved from `md+` to `lg+`: a 768px split leaves both panes too narrow.

## Open items

- Foundation plan must complete before plans 07/08 start (blockedBy enforced in frontmatter).
- User ended the planning session without red-team or validate interview; plans are validated structurally (`ak plan validate` clean ×3) but not adversarially reviewed.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
