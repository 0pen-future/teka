# PM report — Class staff roles + phone privacy: phase 4 complete

Plan: `plans/260830-0938-gh-260830-class-staff-roles-phone-privacy/`
Branch: `teka/260830-0506` | Date: 2026-08-30

## Status

| Phase | Status | Notes |
|---|---|---|
| 1 Schema class_staff + management | Done | shipped, migration applied |
| 2 Read scoping theo assignment | Done | |
| 3 Phone privacy + data ownership | Done | **trừ bước 4 dry-run queries trên prod — chờ user chạy** |
| 4 Writes theo capability map | Done | review cycle đóng, 11/12 finding fixed in-slice, 1 info-only |
| 5 Cleanup legacy scoping | Pending | soak-gated theo plan |

## Verification (2026-08-30)

- Web: `tsc -b` sạch; vitest 449 passed / 68 files; eslint 0 error (5 warning
  có sẵn).
- API: build + vet sạch; notifications + billing integration `-count=1` xanh
  (gồm 2 test mới: RunSnapshot class gate, class-periods edge cases); full
  `make test-api` xanh, total coverage 75.7% (floor 60%).
- e2e: **31 passed** trên stack cô lập `teka-e2e` seed mới (3.7m), gồm 3 spec
  class-staff với assertion mới (trợ giảng không thấy "Huỷ buổi học", copy
  nhãn từ chối mới, afterEach restore idempotent).
- Swagger regen (`make api-docs`) — diff lớn vì docs generated stale từ đầu
  branch.

## Review cycle (phase 4)

12 findings từ code-reviewer, chi tiết + resolution trong delivery note của
`phase-04-capability-map-writes.md`. Đáng chú ý: fix authz thật —
`RunSnapshot` chiều class thiếu `AuthorizeClassSend` (mọi member cùng center
poll được tiến độ gửi lớp bất kỳ), đã gate + pin test.

## Open items (cần user)

1. **Phase 3 bước 4**: dry-run queries trên prod DB chưa chạy (user-blocked).
2. **M5**: member POST /payments trả 404 hay 403 — open question + release note.
3. **Release note write-freeze sau handoff** (behavior removal, D8) khi công bố.
4. Prod đang chạy image build từ working tree chưa commit (GIT_SHA 23895cf
   không phản ánh đúng nội dung) — commit + rebuild khi tiện.
5. Phase 5 chờ soak.

## Uncommitted

~195 file trên `teka/260830-0506` — đề nghị commit (git-manager) sau khi user
duyệt.
