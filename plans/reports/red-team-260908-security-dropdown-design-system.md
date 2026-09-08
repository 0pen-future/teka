# Red team — security & contract verification: dropdown design system

Plan reviewed: `plans/260908-0832-dropdown-design-system/` (plan.md + phase-01..05)
Role: contract verifier / accessibility-and-permission adversary. No files modified.

All findings below are backed by grep citations and, where marked *empirical*, by a
throwaway probe run against this repo's own vitest + Playwright/Chromium.

---

## Finding 1: Phase 4 lists `center-page.test.tsx` as "verify unchanged" while it drives the migrated dropdown with `selectOptions` three times

- **Severity:** Critical
- **Location:** Phase 4, "Related Code Files" and "Success Criteria"
- **Flaw:** The plan enumerates the tests that must change (`center-permissions`, `class-staff-section`, `class-settings-handoff`) and then puts `center-page.test.tsx` in the *Verify unchanged* bucket. That file calls `user.selectOptions` on the per-permission dropdown of `member-permissions-dialog.tsx` in three separate tests — the exact control Phase 4 converts to `HvSelect`.
- **Failure scenario:** Phase 4 is executed, the developer runs only `npx vitest run src/features/center src/features/roster` (step 6), which does include `center-page.test.tsx`, and three tests fail with `Value "grant" not found in options`. Because the plan told the implementer these tests do not change, the likely reaction is to assume the component is broken and start editing `HvSelect`, not the test. Worse: the three tests assert the exact PUT body sent to `/centers/me/members/:id/overrides` (`grants: ["reports.send"]`), i.e. they are the only unit-level proof that the permission-grant payload is correct. Losing or hand-waving them removes the authorization regression net.
- **Evidence:**
  - `apps/web/src/features/center/__tests__/center-page.test.tsx:236-238`, `:280-282`, `:311-313` — `await user.selectOptions(await within(dialog).findByRole("combobox", { name: "Quyền Gửi báo cáo học phí" }), "grant")`
  - `apps/web/src/features/center/components/member-permissions-dialog.tsx:209` — the `<select aria-label={`Quyền ${permission.label}`}>` Phase 4 replaces
  - Plan quote, phase-04 "Related Code Files": *"Verify unchanged: `apps/web/src/features/center/__tests__/center-page.test.tsx`, `class-config-page.test.tsx` (grep cho thấy có `option`/`combobox` — chạy để xác nhận không đụng tới 4 select này)"* — the grep conclusion is wrong.
  - *Empirical:* rendering `<button role="combobox" aria-label="Quyền X">` and calling `user.selectOptions(el, "grant")` throws `Value "grant" not found in options`.
- **Suggested fix:** Move `center-page.test.tsx` into Phase 4's *Modify* list with the three call sites named, and drop the "grep cho thấy" justification — the grep hit `selectOptions`, not just `combobox`.

---

## Finding 2: Phase 4 changes the handoff option's text but not the e2e label it is looked up by, breaking the shared-database restore path

- **Severity:** Critical
- **Location:** Phase 4, "Requirements" → Bàn giao, and step 5
- **Flaw:** Phase 4 replaces the inline owner suffix `" (chủ trung tâm)"` with `meta: member.is_owner ? "chủ trung tâm" : undefined`. In the ported markup, `label` and `meta` are two adjacent `<span>`s with no separator, so the option's accessible name becomes `"Cô Lanchủ trung tâm"`. The e2e helper looks the option up by the literal string `"${OWNER.name} (chủ trung tâm)"`. The plan only says "swap `selectOption` for click + `getByRole("option", { name })`" — it never says the name itself changed.
- **Failure scenario:** `ensureClassTeacher` is used both in the journey and in the `afterEach` restore. Its lookup now matches zero options, the click times out, the restore never completes, and the seeded e2e database is left with Lý 7 assigned to the wrong teacher. Every subsequent run of `class-staff-write` and anything reading that class starts from corrupted state, and the failure surfaces far from its cause.
- **Evidence:**
  - `apps/web/e2e/class-staff-write.spec.ts:195` — `await ensureClassTeacher(owner, classId, OWNER.name, `${OWNER.name} (chủ trung tâm)`)`
  - `apps/web/e2e/class-staff-write.spec.ts:155` — `await card.getByLabel("Bàn giao cho").selectOption({ label: targetOptionLabel })`
  - `apps/web/e2e/class-staff-write.spec.ts:162-165` — the same helper runs from `test.afterEach` "no matter where the handoff journey stopped"
  - `apps/web/src/features/roster/pages/class-settings-page.tsx:358-361` — current option text `{member.full_name}{member.is_owner ? " (chủ trung tâm)" : ""}`
  - `apps/web/src/features/teaching/components/records-class-select.tsx:153-154` — the ported option body: `<span className="flex-1">{name}</span><span …>{meta}</span>`, no separator
  - *Empirical (Chromium via Playwright):* for `<button role="option"><span>Cô Lan</span><span>chủ trung tâm</span></button>`, `getByRole("option", { name: "Cô Lan (chủ trung tâm)", exact: true })` matches 0; `name: "Cô Lanchủ trung tâm"` matches 1.
- **Suggested fix:** Either keep the parenthesised suffix inside `label`, or state the new option name explicitly in Phase 4 and update both `ensureClassTeacher` call sites. Add a rule to the plan: any option whose visible text is split into label + meta gets its new accessible name written out.

---

## Finding 3: `secretary-send.spec.ts` reads the current override with `inputValue()`; Phase 4 provides no replacement for the read half of its assert-then-set guard

- **Severity:** Critical
- **Location:** Phase 4, "Related Code Files" (`secretary-send.spec.ts` dòng 34–41) and step 5
- **Flaw:** The plan only addresses the *write* (`selectOption(target)` → click + option). The spec first reads the live value with `reportsSend.inputValue()` to stay idempotent on a reused database. `HvSelect` renders a `<button>`, and Playwright's `inputValue()` throws on anything that is not `<input>`, `<textarea>` or `<select>`. There is no `value` to read: under D3 this trigger is named by `aria-label` alone (`Quyền Gửi báo cáo học phí`), so the current mode is not even in the accessible name.
- **Failure scenario:** The spec throws at the guard before doing anything, so `setSendReportsGrant` never runs. Since the same helper is used to grant and to revoke, and `revokeGrant` runs in cleanup, a run that fails here leaves Cô Thu holding `reports.send` on the shared e2e database. The next run's "member without the grant cannot send" assertions then pass or fail for the wrong reason. This is a permission-state leak across runs, not a cosmetic locator break.
- **Evidence:**
  - `apps/web/e2e/secretary-send.spec.ts:33-36` — `const reportsSend = dialog.getByRole("combobox", { name: "Quyền Gửi báo cáo học phí" }); … if ((await reportsSend.inputValue()) === target) {`
  - `apps/web/e2e/secretary-send.spec.ts:24-28` — the comment stating the database is reused and the check must be assert-then-set
  - Plan quote, phase-04: *"`secretary-send` dùng `dialog.getByRole("combobox", …)` rồi `page.getByRole("option", { name: "Cấp riêng" })`"* — read path absent
- **Suggested fix:** Specify the read replacement in the plan, e.g. assert on the trigger's rendered text (`toHaveText`) or on `option[aria-selected="true"]` after opening, and make `HvSelect` expose the current selection deterministically (the trigger already renders the selected label). Decide this in Phase 1 so every consumer reads it the same way.

---

## Finding 4: Phase 2 asserts `classbook-page.test.tsx` needs no edit; D4's label/meta split breaks two assertions in it

- **Severity:** High
- **Location:** Phase 2, "Requirements" (non-functional) and "Success Criteria"; root plan D4
- **Flaw:** D4 says trigger *and* option both show `label` + `meta` "cách ` · `". That is true of the trigger in the ported source but false of the option: the option renders label and meta as adjacent spans with no separator. Classbook's test asserts the option's full text with the middle dot.
- **Failure scenario:** After Phase 2, `expect(within(picker).getByRole("option", { name: /Toán 6B/ })).toHaveTextContent("Toán 6B · Tối Thứ Ba")` fails because the option's text content is `"Toán 6BTối Thứ Ba"`. The plan states in two places that this file must not be touched, so the implementer will read the failure as a `HvSelect` defect and may "fix" it by injecting a separator into every option, silently changing the records dropdown that is supposed to be the frozen visual standard.
- **Evidence:**
  - `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx:444-446` — `.toHaveTextContent("Toán 6B · Tối Thứ Ba")`
  - `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx:180` — trigger `toHaveTextContent("Toán 6A · Tối Thứ Ba")` (this one survives; the trigger does carry `·`)
  - `apps/web/src/features/teaching/components/records-class-select.tsx:149-155` vs `:217-227` — option has no `·`, trigger does
  - Plan quote, phase-02: *"`classbook-page.test.tsx` xanh **không sửa** (đã dùng combobox/listbox/option)"*
  - *Empirical:* `toHaveTextContent("Toán 6B · Tối Thứ Ba")` fails against `<button role="option"><span>Toán 6B</span><span>Tối Thứ Ba</span></button>`.
- **Suggested fix:** Decide the option's separator in Phase 1 (this is a DS decision, not a per-consumer one) and record which classbook assertions change as a consequence. Do not leave it to be discovered at test-run time.

---

## Finding 5: Phase 2's locator-change inventory undercounts the affected lines by 5

- **Severity:** High
- **Location:** Phase 2, "Related Code Files" and "Requirements"
- **Flaw:** The plan names `records-toolbar.test.tsx` dòng 43 and `records-pages.test.tsx` dòng 329 and states outright that `records-pages.test.tsx` is edited on "chỉ … 1 dòng". There are 2 sites in the first file and 5 in the second, plus a second site in `records-search.spec.ts`.
- **Failure scenario:** The stated blast radius ("8 file test + 4 e2e đổi locator") is used to size Phase 2 at 4h and to decide Phases 2/3/4 may run in parallel. A locator inventory that is wrong on the one file the plan measured most precisely is not a reliable basis for the parallelism decision, and the missed lines fail only when the whole suite runs — in Phase 5, after three commits have landed.
- **Evidence:**
  - `apps/web/src/features/teaching/__tests__/records-toolbar.test.tsx:43` and `:96` — both `getByRole("button", …)` on the picker
  - `apps/web/src/features/teaching/__tests__/records-pages.test.tsx:302`, `:321`, `:329`, `:340`, `:365`
  - `apps/web/e2e/records-search.spec.ts:28` and `:32`
  - Plan quote, phase-02 Requirements: *"test `records-pages.test.tsx` chỉ sửa role ở 1 dòng"*
- **Suggested fix:** Regenerate the inventory from `grep -rn 'getByRole("button", { name: /\^Lớp' src e2e` and record counts per file, not single line numbers.

---

## Finding 6: The handoff test's exclusion assertion becomes vacuous, and the plan's fix note does not say so

- **Severity:** High
- **Location:** Phase 4, "Related Code Files" (`class-settings-handoff.test.tsx` dòng 72–73)
- **Flaw:** Line 72 is a *negative* assertion — the current teacher must not appear among handoff targets. With a native `<select>` the options exist in the DOM whether or not the control is open, so the assertion has teeth. With `HvSelect` closed, no `option` exists at all, so `queryByRole("option", { name: /Cô Lan/ })` returns null unconditionally. The plan describes the change as "mở trước rồi `screen`", which reads as a mechanical locator swap and gives no signal that the negative half silently stops testing anything if the open step is forgotten or placed after the assertion.
- **Failure scenario:** The implementer fixes line 73 (which fails loudly) and leaves line 72 as-is (which passes). The only automated guard that the currently-assigned teacher is excluded from their own handoff target list is now a no-op. A later change to the `targets` filter re-introduces self-handoff and no test complains.
- **Evidence:**
  - `apps/web/src/features/roster/__tests__/class-settings-handoff.test.tsx:71-74` — `const select = screen.getByLabelText("Bàn giao cho"); expect(within(select).queryByRole("option", { name: /Cô Lan/ })).not.toBeInTheDocument(); expect(within(select).getByRole("option", { name: /Thầy Nam/ })).toBeInTheDocument();`
  - Plan quote, phase-04: *"dòng 72–73 `within(select).queryByRole("option")` → mở trước rồi `screen`"*
- **Suggested fix:** Spell out that after opening, the negative assertion must be scoped to the live listbox (`within(await screen.findByRole("listbox"))`) and paired with a positive count assertion so it cannot pass on an empty tree.

---

## Finding 7: Porting the records list into `components/hv/` pulls a `features/roster` import into the design-system layer, and no lint rule stops it

- **Severity:** High
- **Location:** Phase 1, "Implementation Steps" 1–2 and "Architecture" item 2
- **Flaw:** Phase 1 says to port `useClassSearch` into a private `useOptionSearch`, and is silent about `ClassSearchEmptyNote`, which the source uses to render the no-match note and which lives in `features/roster`. A literal port produces `import { ClassSearchEmptyNote } from "@/features/roster"` inside `components/hv/hv-select.tsx`, inverting the documented dependency direction: features depend on `components/`, never the reverse. Every feature already imports `@/components/hv`, so this also creates a features↔components import cycle through `features/roster/index.ts`.
- **Failure scenario:** Nothing fails at build time and nothing fails at lint time — `eslint.config.js` has exactly one `no-restricted-imports` block and it is scoped to `src/features/statement/**` and `public-layout.tsx`. So the violation lands silently, `components/hv/index.ts` transitively re-exports a feature's module graph, and the next person importing `HvSelect` from a lean context (or the statement route, which is explicitly firewalled from feature plumbing) drags roster code in.
- **Evidence:**
  - `apps/web/src/features/teaching/components/records-class-select.tsx:6` and `:161` — `import { ClassSearchEmptyNote, useClassSearch, type Class } from "@/features/roster"`, used inside the ported `ClassOptionList`
  - `apps/web/src/features/roster/index.ts:6` — `export { ClassSearchEmptyNote, ClassSearchInput } from "./components/class-search"`
  - `docs/frontend-guidelines.md:28-29` — *"Feature code depends on `components/`, `lib/`, and its own folder."*
  - `apps/web/eslint.config.js:38-62` — the only `no-restricted-imports` config, scoped to the statement route
  - Plan quote, phase-01 step 2: *"Port `useClassSearch` → `useOptionSearch` (private trong file)"* — `ClassSearchEmptyNote` never mentioned
- **Suggested fix:** Add an explicit Phase 1 step: inline the two-line empty-note paragraph into `hv-select.tsx`; assert `grep -n "@/features" apps/web/src/components/hv` returns nothing as a Phase 1 success criterion. Consider adding a `no-restricted-imports` block for `src/components/**` so the rule is enforced rather than remembered.

---

## Finding 8: D2 justifies churning 12 test and e2e files with a WAI-ARIA conformance claim the design does not satisfy

- **Severity:** Medium
- **Location:** Root plan, "Quyết định thiết kế đã chốt" D2; Phase 1 "Architecture" item 4
- **Flaw:** D2 says the trigger adopts "mẫu WAI-ARIA 1.2 *select-only combobox*" and calls the current `role=button` "sai mẫu ARIA". The APG select-only combobox keeps DOM focus on the combobox and tracks the active option with `aria-activedescendant`. The ported implementation does the opposite: it moves real DOM focus into the popup and uses roving `tabIndex`, and the popup itself is a Radix `Popover.Content`, which renders `role="dialog"` — so `aria-controls` points at a listbox nested inside a dialog. `aria-controls` is also only emitted while open. That is a defensible *combobox-with-dialog-popup* hybrid, but it is not the pattern D2 names, and "ARIA đúng hơn" is the sole stated reason for accepting the test churn.
- **Failure scenario:** The team pays for 12 locator migrations on the strength of a correctness argument. Post-migration, a screen-reader pass finds the trigger announced as "combobox, collapsed" while focus jumps into a dialog, `aria-activedescendant` absent, and the decision has to be re-litigated after the tests were already rewritten. The reversible-choice window closes before the claim is validated.
- **Evidence:**
  - `apps/web/src/features/teaching/components/records-class-select.tsx:63-68` — `options[wrapped]?.focus()`, real focus movement, no `aria-activedescendant` anywhere in the file
  - `apps/web/src/features/teaching/components/records-class-select.tsx:128-135` — `div[role=listbox] tabIndex={-1}` with `button[role=option] tabIndex={0|-1}`
  - `apps/web/src/features/teaching/components/records-class-select.tsx:253` — `"aria-controls": open ? listboxId : undefined`
  - `aria-query@5.3.2` reports `combobox.nameFrom = ["author"]` and `requiredProps = { "aria-controls": null, "aria-expanded": "false" }`
  - Plan quote D2: *"mẫu WAI-ARIA 1.2 select-only combobox … ARIA đúng hơn"*
- **Suggested fix:** Restate D2 honestly — the real driver is that 8 of 10 sites already locate by `combobox`, which is a consistency argument and is sufficient on its own. Either implement `aria-activedescendant` with focus retained on the trigger, or drop the APG conformance claim and note the hybrid explicitly so nobody audits against the wrong spec. Also emit `aria-controls` unconditionally: the plan's own Phase 1 lint note about jsx-a11y is a non-issue, but a conditional `aria-controls` is the part that actually diverges.

---

## Finding 9: The audit action-group dropdown always crosses the search threshold, and no `searchNoun` is specified for it

- **Severity:** Medium
- **Location:** Phase 3, "Requirements" → Audit and "Architecture"; root plan D5 and Success Criteria
- **Flaw:** `ACTION_GROUPS` holds 18 entries; with `"all"` (and sometimes `"custom"`) that is 19–20 options, permanently above the default `searchThreshold` of 5. Phase 3's example sets `searchNoun` only on the Giáo viên select. Nothing in Phase 3 sets it for Nhóm hành động, so it inherits the D5 default `"mục"`.
- **Failure scenario:** Every audit user sees `Tìm mục…` and `Không có mục nào khớp "x"` in a product whose copy is otherwise domain-specific Vietnamese. It ships because plan.md's success criterion only names *"audit Giáo viên khi nhiều thành viên, Chọn lớp khi nhiều lớp…"* as the dropdowns that gain a filter — the one dropdown guaranteed to gain it is not on the list, so the reviewer checking that criterion has no reason to look.
- **Evidence:**
  - `apps/web/src/features/audit/components/audit-filters.tsx:17-36` — 18 `ACTION_GROUPS` entries
  - `apps/web/src/features/audit/components/audit-filters.tsx:99-104` — plus `all` and conditional `custom`
  - `apps/web/src/features/roster/hooks/use-class-search.ts:20` — `const showSearch = classes.length > 5`, the threshold D5 mirrors
  - Plan quote D5: *"searchNoun … mặc định "mục""*; plan.md Success Criteria: *"Ô lọc chỉ hiện ở dropdown > 5 mục (audit Giáo viên khi nhiều thành viên, Chọn lớp khi nhiều lớp…)"*
- **Suggested fix:** Set `searchNoun="nhóm hành động"` explicitly in Phase 3, and list Nhóm hành động in the plan-level filter criterion so the always-on case is reviewed.

---

## Finding 10: The payment-method dropdown is the only migrated control with an `aria-invalid` contract, and no test exercises it

- **Severity:** Medium
- **Location:** Phase 3, "Requirements" → Ghi nhận thanh toán and "Success Criteria"
- **Flaw:** Phase 3 requires `id="payment-method"` for the `FieldLabel htmlFor` association and `aria-invalid` wired from `errors.method`, then lists "record-payment-dialog … test xanh" as the verification. That file contains a single test, about allocation sums; it never touches the method control, the label association, or the invalid state. So the success criterion is satisfied by a test that cannot detect a regression in what Phase 3 changed.
- **Failure scenario:** The trigger loses its `id`, or `aria-invalid` stops reaching the button through `HvSelect`'s prop plumbing (the plan passes it as a quoted `"aria-invalid"?: boolean` prop, which must be forwarded explicitly and is easy to drop). The label no longer names the control, and the invalid border never appears. Everything stays green, and the only thing standing between that and production is Phase 5's manual screenshot pass — which checks tokens, not attributes.
- **Evidence:**
  - `apps/web/src/features/collections/__tests__/record-payment-dialog.test.tsx:41-42` — the file's only `describe`/`it`, about reallocation sums
  - `apps/web/src/features/collections/components/record-payment-dialog.tsx:235-241` — `<SelectTrigger id="payment-method" className="w-full" aria-invalid={Boolean(errors.method)}>`
  - `apps/web/src/components/hv/hv-select.tsx` does not exist yet; Phase 1's interface declares `"aria-invalid"?: boolean` as a distinct prop that must be forwarded by hand
  - Plan quote, phase-03 Success Criteria: *"`audit-page`, `record-payment-dialog`, `enroll-student-dialog`, `collections-page`, `students-page` test xanh"*
- **Suggested fix:** Add one Phase 3 test asserting `getByRole("combobox", { name: "Hình thức" })` resolves via the `FieldLabel` and carries `aria-invalid="true"` after a failed submit. Alternatively move the invalid-state coverage into `hv-select.test.tsx` and say so, rather than implying the consumer test covers it.

---

## Contract checks that came back clean

Recorded so they are not re-audited: the 10-site inventory matches `grep -rn '<select' src` (4 hits) and `grep -rn 'components/ui/select' src` (4 hits) exactly; `components.json` has no per-file registry entry for `select`, and `eslint.config.js`'s `src/components/ui/**` override is folder-scoped, so deleting `select.tsx` needs no config edit; `jsx-a11y` recommended is satisfied by the proposed trigger markup (`role-has-required-aria-props` sees both `aria-controls` and `aria-expanded` in the JSX, `aria-invalid` is global, and `label-has-associated-control` accepts a bare `htmlFor`); `<label for>` on a `<button role=combobox>` *does* resolve to an accessible name in both `dom-accessibility-api` (vitest) and Playwright/Chromium, so D3's third naming mode holds in both harnesses; `class-staff-section.tsx` gates the whole section on `isOwner` at line 44, so no per-control permission guard can be lost in Phase 4; the `/` hotkey bail in `records-page.tsx:35` keys off `[role="listbox"],[role="dialog"]`, both of which the ported markup preserves.
