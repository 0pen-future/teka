# PM — finalize sync-back: authz write-scope root cause

Plan: `plans/260906-0627-authz-write-scope-root-cause/plan.md` — status `completed` (2026-09-07). Cook `--auto --tdd --advise`, 5 phase, một cây làm việc, chưa commit.

## Checkbox sweep

| File | status | [x] | [ ] |
|---|---|---|---|
| phase-01-view-all-write-scope-hotfix.md | completed | 7 | 0 |
| phase-02-row-anchor-type.md | completed | 7 | 0 |
| phase-03-reports-send-implied-read-keys.md | completed | 5 | 0 |
| phase-04-repository-scope-linter.md | completed | 5 | 0 |
| phase-05-route-spec-manifest.md | completed | 4 | 0 |
| plan.md Success Criteria | completed | 10 | 0 |

Plan criteria được tick sau khi kiểm trực tiếp 2026-09-07: grep `IsOwner: true` ngoài centers = rỗng; `authctx.Scope{` có field ngoài centers/middleware/testutil = rỗng (chỉ còn `Scope{}` zero-value trong handler); `ReportsOversight()` chỉ còn ở gate gửi + gate ghi zalo-mapping; `make api-docs` tái sinh đúng bằng cây (0 drift); inventory.md của plan RBAC có ghi chú superseded (dòng 118).

## Verification (lần cuối trên cây)

| Gate | Kết quả |
|---|---|
| `make test-api` (integration, 37 package) | xanh, coverage 77.8% (sàn 60%) |
| `make lint-api` (golangci-lint 2.7.2 + scopelint) | 0 issue |
| `make test-api-unit` sau sửa review | xanh |
| `make scopelint` cây thật | 0 diagnostic, 0 directive, 21 repository package (sàn 18) |
| `go test ./tools/...` | xanh |
| `make api-docs` | không drift |
| CI | **chưa chạy** (0 push) |

## Reports theo phase

code-review ×5, tester ×4, kongming ×4 (design phase 2, checkpoint phase 2, checkpoint 3+5, checkpoint cuối), inventory prod 260906, slice reports ×4 — tất cả trong `plans/reports/` với hậu tố 260906/260907.

## Docs impact

Đã cập nhật trong phase: `docs/api-guidelines.md` (Tenancy), `docs/adding-permissions.md`, `docs/event-bus.md`, `apps/api/CLAUDE.md`, ghi chú superseded ở inventory plan RBAC. Không cần `docs-manager` thêm: không còn surface nào đổi hành vi mà chưa có doc.

## Chưa giải quyết (cần người)

1. Owner chốt D9 (ai ghi thanh toán; chỉ owner reverse/reallocate; member không xem lại phiếu) và OQ6 (`students.Create` owner-only?) — trước merge. Tin nhắn dán sẵn: `kongming-final-checkpoint-260907.md` §4.
2. Commit: cần user yêu cầu. Công thức 5 commit trên branch `authz-write-scope`: cùng report §3. Không push master.
3. Follow-up ngoài phạm vi: web `member-permissions-dialog.tsx` chưa hiển thị key suy ra; comment `D8`/`D4`/"phase 2" có sẵn trên master (payments, statements, sessions, testutil) vi phạm quy tắc Stable Code Artifacts; test chống chuỗi cho `impliedKeys`; câu về `*Anchored` trong docs Tenancy; `tree_test` chưa truyền `-tags=integration`.
4. Sau deploy: owner chạy `POST /payments/:id/allocations/auto` cho payment dư của member (dữ liệu H-1 tồn đọng) — checklist §5 cùng report.
