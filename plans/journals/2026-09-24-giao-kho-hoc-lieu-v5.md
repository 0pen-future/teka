---
title: "Giao Kho học liệu v5: ba ngân hàng, mã bài tập tự sinh, 3 lỗi review nặng"
date: 2026-09-24
summary: "Plan 260924-0448-kho-hoc-lieu-v5 lên sản phẩm trên feat/giang-day-menu: migration 000033/000034, hub 3 ngân hàng, chi tiết phiên bản dạng tab; review bắt 3 lỗi High (sinh mã bài tập kẹt, kind other không sửa được, chip lớp rỗng) đều đã vá; PR chờ duyệt."
---

# Giao Kho học liệu v5: ba ngân hàng, mã bài tập tự sinh, 3 lỗi review nặng

**Date**: 2026-09-24 16:52
**Severity**: Medium
**Component**: apps/api (library: materials, exercises, exercise groups, templates), apps/web (Kho học liệu hub + chi tiết phiên bản), migrations 000033/000034, seed, docs
**Status**: Resolved

## What Happened

Plan `plans/260924-0448-kho-hoc-lieu-v5/plan.md` xong trên nhánh `feat/giang-day-menu`: migration
`000033`/`000034` thêm `active` cho material/exercise, mã bài tập tự sinh `BT-NNNN` khoá bằng
`pg_advisory_xact_lock`, nhóm bài tập theo từng phiên bản (copy sang draft mới khi nhân bản), `score_set`
đổi từ object sang mảng nhiều bộ điểm, thêm loại nhật ký `long_text`/`student`, giữ `VERSION_LOCKED` và
thêm 409 `MATERIAL_IN_USE`/`EXERCISE_IN_USE`. Web: nhóm sidebar riêng "Kho học liệu", hub `/library` dạng
thẻ, hai ngân hàng `/library/materials` và `/library/exercises`, trang chi tiết phiên bản chuyển từ
`?tab=` sang tab thật (kèm redirect cho link cũ). Seed bọc trong một transaction. Docs
`api-guidelines`/`frontend-guidelines` cập nhật theo. Code review sau khi implement bắt 3 lỗi High, tất
cả đã sửa trước khi verify lần cuối.

## The Brutal Truth

Đây là plan có review pass ăn tiền thật: cả 3 lỗi High đều là loại "chạy được trong demo, hỏng khi dữ
liệu vượt qua 9 bản ghi" hoặc "hỏng cho đúng nhóm người dùng cũ mà không ai test lại". Nếu ship thẳng
sau lượt implement đầu, sinh mã bài tập sẽ kẹt cứng ở `BT-0009` mãi mãi — kiểu bug im lặng, không panic,
không log lỗi, chỉ đơn giản là data sai và không ai biết cho tới khi có người hỏi "sao bài tập thứ 10
không được". Cũng mệt vì lần này lại dính đúng bẫy cũ: `make test-api` không tự thêm `-p 1`, chạy xong
timeout kiểu systemd-scope, tưởng code lỗi, hoá ra là tranh chấp song song — bài học đã ghi nhớ từ trước
mà vẫn phải tự nhắc lại giữa lúc đang verify.

## Technical Details

- Sinh mã `BT-NNNN`: bản đầu lấy `MAX` theo so sánh **lexicographic** trên chuỗi `BT-...`, nên
  `"BT-0010" < "BT-0009"` theo string compare → mã kế tiếp sau `BT-0009` bị tính lại là `BT-0010` lặp
  vô hạn ở dạng khác, hoặc tệ hơn là không tăng đúng khi số chữ số lệch. Fix: parse phần số bằng regex
  `^BT-[0-9]+$`, lấy MAX theo giá trị numeric thật, vẫn giữ `pg_advisory_xact_lock` để tránh đua giữa
  các request đồng thời.
- `library_materials.kind = 'other'` (dữ liệu cũ, theo D4 không cho **chọn mới**) bị chặn nhầm thành
  không cho **sửa** luôn — user có material cũ không đổi được tiêu đề/URL vì form coi `other` là kind bị
  cấm hoàn toàn thay vì chỉ cấm ở dropdown tạo mới.
- Chip lớp đang gắn trên tab Phiên bản luôn rỗng: query `VersionResponse.classes[]` không join đúng bảng
  `class_programs` theo phiên bản, nên UI hiển thị "chưa có lớp nào" kể cả khi có lớp thật đang dùng.
- Medium đã vá thêm: `lesson_count` cộng dồn qua nhiều phiên bản thay vì chỉ phiên bản hiện tại; tiêu đề
  trùng bị cắt sai byte (sửa sang cắt theo rune); giới hạn input trong dialog; cache client cũ gây `422`
  sau khi đổi nội dung phiên bản (thêm helper dùng chung `invalidateVersionContent`); N+1 khi liệt kê
  phiên bản; race điều kiện khi sinh mã.
- Bỏ qua có chủ đích: lớp đã xoá vẫn được đếm vào chip lớp (hành vi cũ, không đổi phạm vi); giới hạn 100
  item trong picker chọn từ ngân hàng.

## What We Tried

- `make test-api` chạy thẳng không cờ → timeout kiểu contention trong systemd-scope, tưởng nhầm là lỗi
  code mới; đã biết trước từ lần trước (ghi trong memory) nhưng vẫn phải dừng lại chẩn đoán lại trước khi
  nhớ ra — chạy `go test -p 1` trực tiếp mới sạch.
- `pgrep` guard để kiểm tra process trùng khi chạy chained trong cùng shell tự match luôn chính lệnh
  đang chạy (self-match) — phải tách lệnh kiểm tra ra khỏi chuỗi pipe chung.
- E2E spec chuẩn bị dữ liệu (prep) gãy vì danh sách chương trình mẫu đổi từ bảng (table rows) sang thẻ
  (`role=article`) theo UI mới của hub — selector cũ trỏ vào row không còn tồn tại; sửa lại selector theo
  role thẻ.
- Commit bị `commitlint` chặn vì `body-max-line-length` 100 ký tự; phải bọc lại dòng thân commit. Đồng
  thời phát hiện file staged bị lọt sang commit kế tiếp khi gộp nhiều đợt sửa liên tiếp — phải soát lại
  `git status`/`git diff --staged` từng commit thay vì tin theo trình tự đã gõ.

## Root Cause Analysis

- Bug sinh mã: implement đầu dùng string compare cho một giá trị về bản chất là số có prefix cố định —
  giả định ngầm "chuỗi có prefix giống nhau thì so sánh như số" chỉ đúng khi số chữ số bằng nhau, sai
  ngay khi vượt quá 9999 hoặc lệch độ dài.
- Bug `other` kind: một quy tắc business ("không cho chọn `other` khi tạo mới") bị áp dụng nhầm phạm vi
  thành "không cho chọn `other` ở bất cứ form nào", vì logic validate dùng chung một whitelist cho cả
  create và update thay vì tách theo action.
- Chip lớp rỗng: join sai bảng là lỗi implement thuần, không phải thiếu thiết kế — D-decision đã ghi rõ
  cần `classes[]` trên `VersionResponse`, chỉ là câu query đầu chưa đúng điều kiện join.

## Lessons Learned

- Không so sánh chuỗi có prefix cố định như số bằng string compare — luôn parse ra kiểu số trước khi
  `MAX`/so sánh, kể cả khi trông "chắc chắn không bao giờ vượt quá vài chữ số".
- Validate theo hành động (create vs update) phải tách whitelist/rule riêng, không dùng chung một danh
  sách "giá trị hợp lệ" cho cả hai — nếu không, quy tắc "khoá giá trị cũ khỏi lựa chọn mới" sẽ vô tình
  khoá luôn việc sửa dữ liệu cũ.
- `make test-api` cần `-p 1` — đã ghi nhớ nhưng vẫn quên giữa lúc verify; nên tự động hoá bằng cách sửa
  thẳng `Makefile` thay vì tiếp tục dựa vào trí nhớ người chạy lệnh.
- Đổi UI list sang card (`role=article`) phải rà lại toàn bộ spec e2e prep/setup trỏ vào cấu trúc DOM cũ,
  không chỉ spec test hành vi chính.

## Next Steps

- PR `feat/giang-day-menu` → `master` đã push, đang chờ người dùng duyệt trước khi merge (không tự
  merge/push force).
- Ghi nhận rõ: nhiều phiên bản published của cùng một chương trình mẫu vẫn có thể tồn tại song song — đây
  là hành vi kế thừa từ trước, plan này không đổi, không phải bug bỏ sót.
- Sau merge: theo dõi lần chạy migration `000033`/`000034` trên production giống các lần trước (backup
  trước khi `migrate-up`).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
