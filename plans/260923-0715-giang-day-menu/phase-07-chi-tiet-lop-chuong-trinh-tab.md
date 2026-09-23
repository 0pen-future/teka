---
phase: 7
title: "Chi tiết lớp: Chương trình học, Bài tập, Tài liệu, Chat, Lịch sử lớp"
status: completed
priority: P1
effort: "2.5d"
dependencies: [1, 4, 5]
---

# Phase 7: Chi tiết lớp — Chương trình học, Bài tập, Tài liệu, Chat, Lịch sử lớp

> Outline (deep mode). **Scout lại (bắt buộc)**: `teaching/service.go:94` (`PutCurriculum` gate `CapLessonPlanWrite` →
> `GetWritable` owner/giao_vien), `lesson_plans` theo index, `router.go#registerFeatures` (:155 classes, :186 imports,
> :203 handoff, :220 teaching — thứ tự dựng), `handoff/` làm mẫu feature điều phối, `audit` feature (bảng, cột, route
> `/audit-logs`, filter hiện có), `classscope.PhoneVisibleViaStudent` làm mẫu gate theo stint đang mở,
> `class-detail-page.tsx` sau Phase 1/2/5.

## Context Links
- [plan.md](./plan.md) · D4, D7, D8, D9 · Prototype tabs `cdt`: Chương trình học, Bài tập, Tài liệu, Chat, Lịch sử lớp / Lịch sử thay đổi.
- Red team: [reports/redteam-failure-260923.md](./reports/redteam-failure-260923.md), [reports/redteam-scope-260923.md](./reports/redteam-scope-260923.md).

## Goal
Lớp áp dụng một phiên bản chương trình mẫu (mặc định từ khóa học); các tab Bài tập / Tài liệu đọc xuyên từ phiên
bản; tab Buổi học ánh xạ buổi thực ↔ buổi mẫu; Chat nội bộ lớp; Lịch sử lớp (lineage lớp tách/ghép) và Lịch sử
thay đổi (audit theo entity).

## Key decisions
- **Hai feature điều phối mới, dựng sau `classes`/`teaching`/`library` trong `registerFeatures`** (khuôn `handoff` :203):
  `classprogram` sở hữu `/classes/:id/program*`, `classchat` sở hữu `/classes/:id/messages*`. Lý do: `classes` được dựng
  ở `router.go:155` **trước** `teaching` (:220) nên không thể bơm `teaching.Service` vào `classes` mà không đảo thứ tự
  dựng của nhiều feature khác. <!-- Red Team S1 F4 -->
- **`class_programs(class_id PK, center_id, template_version_id, applied_at, applied_by)`** — một lớp một chương trình.
  **Áp dụng / đổi phiên bản** ghi `class_programs` **và** `class_curricula.lessons` (tên buổi) qua
  `teaching.Service.PutCurriculum`; **Gỡ chương trình chỉ xoá `class_programs`**, giữ nguyên `class_curricula` và
  `lesson_plans` (Sổ đầu bài, giáo án không mất). <!-- Red Team S1 F4 -->
- **Gate áp dụng/đổi/gỡ: chỉ owner** — `PUT`/`DELETE /classes/:id/program` là `KindOwnerOnly`; owner luôn qua được
  `GetWritable` nên `PutCurriculum` không thể 403 sau khi route cho qua. `GET /classes/:id/program*` là `KindService`
  theo read port của lớp. Không dùng key `classes.edit`. <!-- Red Team S1 F4 --> <!-- Updated: Validation Session 1 - Q5: chỉ owner áp dụng/đổi/gỡ chương trình -->
- **Xác nhận thay vì preview**: body `PUT /classes/:id/program {template_version_id, confirm?}`; nếu `class_curricula.lessons`
  đang khác rỗng và khác với tiêu đề buổi mẫu → 409 `CURRICULUM_DIFFERS` kèm `{current_count, template_count}`; web mở
  `HvConfirmDialog` rồi gửi lại `confirm: true`. Không có endpoint `apply-preview`. <!-- Red Team S1 F15 -->
- Chỉ áp dụng phiên bản **published**; template có `class_programs` tham chiếu → không xoá được (409).
- Buổi thực thứ i ↔ buổi mẫu vị trí i (theo `session_date` tăng, chỉ buổi `planned|held`) — **web zip hai danh sách**
  (`GET /classes/:id/sessions?from&to` + `GET /classes/:id/program/lessons`), không đổi hợp đồng `sessions`. Cột NGUỒN theo
  D7: khớp schedule **hiệu lực tại ngày buổi**.
- **Chat = tin nhắn nội bộ** bảng `class_messages` (không Zalo OA — N1). Key `class_messages.post` (`def`, low, DefaultGrant + backfill).
  **Đọc/ghi chỉ với owner hoặc người có stint đang mở (`ended_at IS NULL`) trên lớp** — read port `classes` hiện giữ cả stint
  đã đóng nên `classchat` cần predicate riêng (khuôn `PhoneVisibleViaStudent`). <!-- Red Team S1 F13 --> `RESOURCE_LABELS` thêm `class_messages`.
- Lịch sử lớp: `classes.parent_class_id` FK composite `(parent_class_id, center_id) → classes (id, center_id)` + `classes.lineage_note`;
  UI hiện chuỗi cha → con; sửa qua `PUT /classes/:id` (pointer, gate `GetByID` như Phase 1).
- Lịch sử thay đổi: `GET /audit-logs?entity_type=class&entity_id=` — mở rộng **tương thích** filter của route hiện có
  (đường dẫn thật là `/audit-logs`, không phải `/audit`) + index `(center_id, entity_type, entity_id, occurred_at DESC)`
  trong migration 000031 (bảng `audit_logs`, cột `entity_type`/`entity_id` TEXT, `occurred_at` — `000010_audit_logs.up.sql:18-26`). <!-- Red Team S1 F15 --> <!-- Updated: Validation Session 1 - cột audit_logs xác minh -->
- Không bump `CatalogVersion` ở đây — Phase 8 đã xác nhận trong scope (Q2), bump ở Phase 8 theo D5. <!-- Updated: Validation Session 1 - Q2 -->

## Migration sketch — `000031_class_programs` <!-- Red Team S1 F5, F8 -->
```sql
class_programs (class_id UUID PRIMARY KEY, center_id UUID NOT NULL, template_version_id UUID NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), applied_by UUID NOT NULL,
  FOREIGN KEY (class_id, center_id)            REFERENCES classes (id, center_id) ON DELETE CASCADE,
  FOREIGN KEY (template_version_id, center_id) REFERENCES program_template_versions (id, center_id))
class_messages (id PK, center_id NOT NULL, class_id NOT NULL, author_id NOT NULL, body TEXT NOT NULL CHECK (length(body) <= 2000),
  created_at, deleted_at, FOREIGN KEY (class_id, center_id) REFERENCES classes (id, center_id) ON DELETE CASCADE)
  INDEX (class_id, created_at DESC)
ALTER TABLE classes ADD COLUMN parent_class_id UUID NULL, ADD COLUMN lineage_note TEXT,
  ADD CONSTRAINT fk_classes_parent FOREIGN KEY (parent_class_id, center_id) REFERENCES classes (id, center_id);
CREATE INDEX idx_audit_logs_entity ON audit_logs (center_id, entity_type, entity_id, occurred_at DESC);  -- cột xác minh: 000010_audit_logs.up.sql:18-26
-- backfill quyền class_messages.post (step label riêng)
-- down: xoá quyền theo step label; DROP INDEX idx_audit_logs_entity; ALTER TABLE classes DROP CONSTRAINT, DROP 2 cột; DROP 2 bảng
```
FK về `classes` dùng `ON DELETE CASCADE` theo khuôn 000009/000015 (chỉ chạy khi xoá cứng; soft-delete không kích hoạt). <!-- Updated: Validation Session 1 - D8 theo quy ước repo -->

## API
- `classprogram` (feature mới): `GET /classes/:id/program`, `PUT /classes/:id/program {template_version_id, confirm?}`,
  `DELETE /classes/:id/program` (chỉ xoá `class_programs`), `GET /classes/:id/program/lessons` (read-through: lessons +
  materials `shared_with_students` + exercises từ `library`). Kind: `PUT`/`DELETE` `KindOwnerOnly`, `GET` `KindService`;
  port: `classes` read port, `teaching.Service.PutCurriculum`, `library` read port. <!-- Updated: Validation Session 1 - Q5 -->
- `classchat` (feature mới): `GET/POST /classes/:id/messages` (cursor `before`, `limit ≤ 50`),
  `DELETE /classes/:id/messages/:mid` (tác giả hoặc owner). Kind: `POST` perm `class_messages.post` + gate stint mở;
  `GET` `KindService` gate stint mở/owner.
- `audit`: filter `entity_type`, `entity_id` trên `GET /audit-logs` (additive).
- `GET /classes/:id/sessions` **không đổi**.
- Mọi route mutating khai `req(action, "class_program"|"class_message", id)`; `route_policy_snapshot_test.go` (+7),
  `audit/action_test.go` (+4). <!-- Red Team S1 F7 -->
- Wiring: `router.go#registerFeatures` dựng `classprogram`, `classchat` **sau** `teaching` (:220) và `library`.

## Web (feature `roster`)
- Khối "Chương trình học" trong `class-info-tab.tsx`: Thiết lập (`HvSelect` template → version published) /
  Áp dụng từ khóa mẫu (dùng `course.default_template_version_id`) / Mở chương trình mẫu (link `/library/templates/:id`)
  / Đổi phiên bản / Gỡ — `HvConfirmDialog` khi API trả 409 `CURRICULUM_DIFFERS` (đổi/áp dụng) và luôn khi Gỡ
  (nêu rõ "Sổ đầu bài và giáo án giữ nguyên"). Gate hiển thị hành động: `isOwner`. <!-- Updated: Validation Session 1 - Q5 -->
- Tab Bài tập: nhóm theo buổi mẫu; tab Tài liệu: chỉ `shared_with_students` + toggle "Hiện tất cả".
- Tab Buổi học: zip sessions ↔ lessons theo thứ tự; cột NGUỒN + tên buổi mẫu; CTA "Sổ đầu bài".
- Tab Chat: `class-chat-panel.tsx` (list + composer, `useInfiniteQuery`), chú thích "Tin nhắn nội bộ, không đồng bộ Zalo";
  composer gate `has("class_messages.post")`.
- Lịch sử lớp (`class-lineage-card.tsx`) + "Lịch sử thay đổi" toggle đọc `/audit-logs` (chỉ khi `has("audit.read")`;
  web path hiện có `/audit-logs`).
- Bỏ trạng thái disabled của 3 tab Phase 1.

## Verification
- Integration: áp dụng chương trình → `class_curricula.lessons` = tiêu đề buổi mẫu; `lesson_plans` cũ giữ nguyên;
  áp dụng khi lessons đang khác → 409 rồi `confirm:true` → 200; **gỡ** → `class_programs` trống nhưng `class_curricula`
  còn nguyên; GV chính (giao_vien) và trợ giảng gọi PUT/DELETE program → 403 (owner-only); GV có stint đã đóng đọc messages → 404/403; owner đọc được.
- Migration 000031 up/down/up sạch.
- Manifest + snapshot + audit-action tests cho 7 route mới; Vitest tab Chat quartet + gửi tin; e2e cập nhật `class-list.spec.ts` (áp dụng CT).

## Risks
- Đụng hợp đồng `teaching` — chạy toàn bộ `make test-api` package `teaching` + web `teaching/__tests__`.
- Số buổi thực khác số buổi mẫu là bình thường; UI phải hiện lệch rõ, không tự sinh/xoá buổi.
- `audit_logs` đã có `entity_type`/`entity_id` (000010) — index mới chỉ bổ sung đường truy vấn theo entity; filter là additive.

## Completion notes (2026-09-24)
- Backend (`0f5c174`): migration 000031 (`class_programs`, `class_messages`, `parent_class_id` + `lineage_note` trên
  `classes` với FK composite cùng center `ON DELETE SET NULL`, index audit theo entity, backfill `class_messages.post`
  hai nhánh theo sổ). Hai feature điều phối `classprogram` (PUT/DELETE owner-only, GET service, `CURRICULUM_DIFFERS`
  409 + `confirm`) và `classchat` (cursor keyset, xoá theo tác giả hoặc owner) dựng sau `teaching` và `library`;
  `audit-logs` nhận filter `entity_type`/`entity_id`. `CatalogVersion` không đổi.
- Web (`952f0e5`): card "Chương trình học" (picker template → phiên bản published, "Áp dụng từ khóa mẫu", dialog khi
  sổ đầu bài đang khác, gỡ có xác nhận), card "Lịch sử lớp" + toggle "Lịch sử thay đổi" khi có `audit.read`, tab
  Chat (`useInfiniteQuery`, composer gate `class_messages.post`), tab Bài tập và Tài liệu đọc qua lớp, tab Buổi học
  zip với buổi mẫu và cảnh báo lệch. Ba tab bỏ trạng thái disabled.
- Sau review (`ae5a75b`, `a390f2d`): phiên bản lưu trữ vẫn đọc được qua lớp (`library.ReleasedVersion`), field
  `version_status` mới và badge "Đã lưu trữ"; trần 100 buổi khi áp dụng (`TEMPLATE_TOO_LONG`); xoá tin nhắn qua
  hộp xác nhận với nhãn a11y theo tin; link thư viện chỉ hiện khi có `library.read`.
- Lệch spec có chủ đích: `GET /program/lessons` trả đủ tài liệu, web lọc `shared_with_students` và có switch
  "Hiện tất cả" (người đọc là nhân sự lớp); e2e nằm ở `class-program.spec.ts` mới thay vì `class-list.spec.ts`
  (file này không tồn tại), tự tạo template và lớp qua API rồi dọn ở `afterEach`.
- Bài học: envelope API bỏ hẳn key `data` khi payload nil nên schema web phải `.nullish()`; audit ghi bất đồng bộ
  nên e2e phải reload đến khi thấy dòng thay vì chờ cố định.
- Kiểm chứng: integration `classprogram`, `classchat`, `library`, `migrations` xanh (`-p 1`); `make test-api-unit`,
  `scopelint`, `lint`, `test-web` xanh; e2e `class-program.spec.ts` xanh trên stack cô lập.
- Để lại cho Phase 9 (review L1, L3–L6, test gaps): xem `## Disposition` trong `reports/review-phase-07-260924.md`.
  L9 (chat tự làm mới) cần user quyết định.
