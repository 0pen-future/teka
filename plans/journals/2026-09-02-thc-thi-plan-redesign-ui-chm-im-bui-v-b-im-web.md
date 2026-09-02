---
title: Thực thi plan redesign UI chấm điểm buổi và bộ điểm (web)
date: 2026-09-02
summary: "Hoàn tất 6 phase UI chấm điểm/bộ điểm trong apps/web; review phát hiện flush() báo thành công dù còn ô invalid, đã sửa guard và bỏ guard modal."
---

# Thực thi plan redesign UI chấm điểm buổi và bộ điểm (web)

## What happened

- Thực thi toàn bộ plan `plans/260902-1209-session-scoring-ui-redesign/` bằng `/ak:cook --auto` trên nhánh `teka/260902-1241`: hv kit primitive (HvStateBlock, HvNotice, HvSegmented, HvScoreInput, HvModal size), chấm điểm theo học sinh có autosave 800ms + guard ô chưa lưu, bảng đầy đủ dùng chung nháp, bottom sheet mobile, trình soạn bộ điểm (từng cột / dán danh sách, tối đa 10 cột, chặn trùng), gán bộ điểm bằng radio card, bảng lớp/thẻ responsive.
- Gate xanh sau đợt sửa: eslint 0 lỗi (5 warning `react-hooks/incompatible-library` có sẵn), `tsc -b` sạch, Vitest 79 file / 561 test.
- Reviewer bắt lỗi High: `flush()` trong `use-score-draft.ts` bỏ ô invalid khỏi payload rồi trả `true`, nên "Lưu và đóng" đóng panel và vứt chữ người dùng gõ. Sửa: `flush()` trả `false` khi còn ô invalid; guard nhận `invalidCount`, vô hiệu "Lưu và đóng" và hiện thông báo danger; đếm dirty/invalid được truyền entry → panel → page.
- Medium đã sửa: page reset đếm dirty khi tự đóng panel (panel unmount trước khi effect báo về); đổi lớp đi qua guard như đổi buổi; bỏ guard đóng modal bảng đầy đủ vì nháp dùng chung với panel, đóng modal không mất gì.
- Phát hiện khi test: payload autosave phải dựng lúc debounce bắn (lazy), không snapshot lúc schedule, để ô đã revert trong 800ms không bị gửi lại. React Compiler cấm ghi ref trong render nên memo hàng bảng dùng comparator `areRowPropsEqual` thay mảng ổn định qua ref.
- Sonner toast tồn tại xuyên test trong cùng file: assert theo nội dung toast cụ thể, không dùng `getByRole("status")` chung.

## Decision

- Guard ô chưa lưu chỉ đặt ở panel (đóng panel, đổi buổi, đổi lớp); modal bảng đầy đủ không guard. Plan và phase-04 đã ghi deviation.
- Chuỗi loading chỉ nằm trong `title` của `HvStateBlock`; tiêu chí grep `Đang tải` ở phase-02 được viết lại cho đúng thực tế.
- Không commit trong phiên tự động: quy tắc harness chỉ commit khi người dùng yêu cầu; commit message khi tạo phải không có tham chiếu AI.

## Next steps

- Kiểm tra tay có ảnh ở 1280/1080/390px (sticky, line-clamp, không cuộn ngang với 10 cột) trước khi mở PR; các ô tương ứng trong phase-02/03/04/06 vẫn để mở.
- Commit qua `git-manager` khi người dùng đồng ý; xem lại kết quả re-review đợt sửa (reviewer-fix-cycle) nếu còn concern.
- Follow-up API ngoài scope: `has_scores` cho lớp, `class_count` cho bộ điểm, batch `score-components`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
