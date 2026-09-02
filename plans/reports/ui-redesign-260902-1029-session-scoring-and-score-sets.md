# Redesign: chấm điểm thành phần sau buổi học và định nghĩa bộ điểm

- Ngày: 2026-09-02
- Phạm vi: `apps/web/src/features/teaching/components/component-score-grid.tsx`, `session-detail-panel.tsx`, `apps/web/src/features/center/components/score-set-editor-modal.tsx`, `assign-score-set-dialog.tsx`, `apps/web/src/features/center/pages/class-config-page.tsx`
- Mockup trực quan: `plans/reports/ui-redesign-260902-1029-session-scoring-and-score-sets.html`
- Ràng buộc: giữ nguyên design system (token, hv kit); API hiện tại giữ nguyên trừ mục "API tuỳ chọn"

## 1. Bối cảnh dữ liệu (đã xác minh từ mã nguồn)

- Bộ điểm = tên + danh sách 1..10 cột (tên ≤ 50 ký tự, không trùng). Không có trọng số, không có điểm tối đa riêng. Điểm mỗi ô 0–10 bước 0.5 (`parseScoreInput`).
- Gán bộ điểm cho lớp là snapshot; lớp đã có điểm thì không đổi được (API trả 409, web chỉ biết sau khi bấm).
- Chấm điểm: `PUT /sessions/:id/scores` nhận nhiều ô một lần, `score: null` xoá ô. Chỉ chấm được học sinh `present` của buổi `held`.
- Điểm thành phần chưa vào báo cáo phụ huynh (copy trong grid hiện tại).

## 2. Chẩn đoán màn chấm điểm (`ComponentScoreGrid` trong `SessionDetailPanel`)

| # | Vấn đề | Bằng chứng | Ảnh hưởng khi ≥5 cột |
|---|--------|-----------|----------------------|
| S1 | Grid đặt trong panel hẹp | panel `min-w-[320px] flex-1` cạnh bảng buổi `flex-[1.6] basis-[460px]`; ở 1080px panel còn ~400px | Mỗi cột ~80px + cột tên ~120px: 5 cột = 520px, 8 cột = 760px. Luôn phải cuộn ngang |
| S2 | Cuộn hai chiều | `max-h-[280px] overflow-x-auto overflow-y-auto` | Tiêu đề cột trôi mất khi cuộn dọc, cột tên dính nhưng không biết đang nhập cột nào |
| S3 | Ô nhập nhỏ hơn chuẩn chạm | `w-16 py-[5px]` ≈ 64×34px, `type=number` có spinner | Khó bấm trên tablet, dễ nhập nhầm cột kế bên |
| S4 | Thứ tự Tab theo hàng | DOM theo student × component | Giáo viên chấm từ tập bài (theo học sinh) thì ổn, nhưng chấm theo rubric (theo cột cho cả lớp) phải Tab qua 7 ô mỗi lần |
| S5 | Không có tiến độ và không biết ô nào chưa lưu | chỉ có chữ "Chưa lưu" toàn cục | Với 20 học sinh × 8 cột = 160 ô, không biết đã chấm tới đâu |
| S6 | Học sinh vắng chiếm nguyên hàng | mỗi ô render pill "Vắng" | 8 pill "Vắng" cho một học sinh, nhiễu |
| S7 | Tên cột dài làm cột phình | không `truncate`/`max-w` ở `th` | Tên 30–50 ký tự đẩy bảng rộng gấp đôi |
| S8 | Không có bản mobile | dưới `sm`, panel xuống dưới bảng buổi, grid 320px hiện 2–3 cột | Không dùng được trên điện thoại |
| S9 | Nút lưu là chuỗi className riêng | `saveButtonActive/Idle` | Lệch kit (đã ghi ở review tổng, C2) |

## 3. Chẩn đoán màn định nghĩa bộ điểm

| # | Vấn đề | Bằng chứng | Ảnh hưởng khi ≥5 cột |
|---|--------|-----------|----------------------|
| E1 | Modal hẹp, hàng cột chật | HvModal `max-w-md` (448px); mỗi hàng: số + input + ↑ + ↓ + Xoá | Input còn ~250px, tên dài bị cắt |
| E2 | Nút ↑ ↓ và Xoá là chữ 24px | `px-1 py-1` glyph, không phải HvButton | Dưới chuẩn chạm; sắp 8 cột bằng ↑↓ cần hàng chục lần bấm |
| E3 | Lỗi gom chung dưới danh sách | `FieldError errors=[...components.map(...)]` | "Tên cột điểm bị trùng" không chỉ ra hàng nào trong 8 hàng |
| E4 | Không nhập nhanh | phải bấm "+ Thêm cột điểm" rồi gõ từng ô | Tạo bộ IELTS 8 cột = 7 lần bấm thêm + 8 lần gõ |
| E5 | Không xem trước | không có preview | Không biết tên cột hiển thị ra sao trong bảng chấm |
| E6 | Không có đếm | không hiện "6/10" | Chạm giới hạn 10 mới biết (nút thêm mờ đi) |
| E7 | Danh sách bộ điểm nối cột bằng dấu phẩy | `components.join(", ")` 12.5px | 8 tên nối thành một dòng dài khó quét |
| E8 | Gán bộ điểm bằng `<select>` | native select, chỉ hiện "cột hiện tại" | Không thấy cột của bộ sắp gán; khoá 409 chỉ biết sau khi bấm |
| E9 | Bảng gán lớp không có bản mobile, `th` thiếu scope | `min-w-[420px]` | Đã ghi ở review tổng (C9) |

## 4. Thiết kế lại màn chấm điểm

Nguyên tắc: một cách nhập cho không gian hẹp, một cách nhập cho màn rộng, cùng một nguồn draft và cùng một hành động lưu. Không thêm màu, không thêm bán kính; dùng HvButton, HvBadge, HvModal, HvCard, StatPill.

### 4.1 Chế độ mặc định trong panel: "Theo học sinh"

- Danh sách học sinh dạng hàng 56px: tên, chấm tiến độ dạng `n/N` (số cột đã có điểm), điểm trung bình buổi (StatPill), mũi tên mở.
- Bấm một hàng thì mở thẻ chấm ngay dưới hàng đó (accordion, chỉ một thẻ mở): các cột xếp lưới 2 cột ở ≥360px, mỗi ô là nhãn + input 44px `inputmode="decimal"` (không dùng `type=number` để bỏ spinner). Enter hoặc Tab đi tới cột kế; hết cột thì tự mở học sinh kế tiếp và focus ô đầu.
- Học sinh vắng gộp thành một hàng mờ với một badge "Vắng" duy nhất, đặt cuối danh sách.
- Thanh trạng thái dính đáy panel: "12/18 học sinh đã chấm · 3 ô chưa lưu" + HvButton "Lưu điểm" (sm). Ô sửa chưa lưu tô nền sun-100. Đóng panel khi còn ô chưa lưu thì HvConfirmDialog hỏi.
- Đây cũng là bản mobile: dưới `sm` panel mở dạng bottom sheet (HvModal) với đúng danh sách này.

### 4.2 Chế độ "Bảng đầy đủ" (màn ≥ 1024px)

- Nút "Mở bảng" ở đầu tab Điểm buổi. Bảng mở trong HvModal cỡ lớn (thêm `size="xl"` cho HvModal: `max-w-[var(--w-page)]`, chiều cao 90dvh) để có toàn bộ chiều rộng, thay vì đẩy panel cạnh bảng buổi.
- Lưới: cột tên dính trái 180px; mỗi cột điểm cố định 76px; tiêu đề cột dính trên, tên cột `line-clamp-2` và có `title`; cột cuối "TB" tính trung bình các ô đã có điểm.
- Ô: 44px cao, `inputmode="decimal"`, viền line-200, focus mint-400, chưa lưu nền sun-100, vừa lưu nền mint-50 nhạt dần (dur-slow).
- Bàn phím: Enter đi xuống cùng cột (chấm theo rubric), Tab đi sang phải (chấm theo học sinh), Shift+Enter đi lên. Bấm tiêu đề cột thì focus ô đầu của cột đó.
- Học sinh vắng: một ô gộp `colspan` chữ "Vắng", nền coral-100, đặt cuối bảng.
- Chân bảng dính: cùng thanh trạng thái với 4.1.

### 4.3 Tự lưu hay lưu tay

Giữ nút "Lưu điểm" nhưng thêm tự lưu khi rời ô (blur) sau 800ms bằng hook `use-debounced-save` đã có, vì API nhận `score: null` để xoá và ghi đè từng ô nên không mất dữ liệu. Nút lưu chỉ còn để lưu ngay. Toast một lần mỗi đợt, không toast từng ô.

### 4.4 Thay đổi mã dự kiến

- `component-score-grid.tsx` tách thành `score-entry-by-student.tsx` (4.1) và `score-entry-table-modal.tsx` (4.2), dùng chung `use-score-draft.ts` (draft, dirty count, parse, save, autosave).
- `session-detail-panel.tsx`: tab Điểm buổi chỉ còn header (mô tả + nút Mở bảng) và render 4.1.
- `components/hv/hv-modal.tsx`: thêm prop `size: "md" | "lg" | "xl"` (md = hiện tại).
- `components/hv/hv-score-input.tsx`: input 44px `inputmode=decimal`, prop `state: idle | dirty | saved | invalid`, dùng cho cả hai chế độ và cho ô điểm chung (general score) hiện có.
- Bỏ `save-button-styles.ts`, thay bằng HvButton.

## 5. Thiết kế lại màn định nghĩa bộ điểm

### 5.1 Trình soạn bộ điểm (`ScoreSetEditorModal`)

- HvModal `size="lg"` (720px). Trên phone vẫn là bottom sheet, nội dung cuộn, footer dính.
- Hai cách nhập, chuyển bằng HvSegmented "Từng cột | Dán danh sách":
  - Từng cột: mỗi hàng 48px gồm tay cầm kéo thả (≡, kéo bằng chuột/chạm; giữ ↑ ↓ trong menu ba chấm cho bàn phím), số thứ tự, input, nút xoá 44px ghost. Lỗi hiện ngay dưới hàng lỗi (viền coral-400 + FieldError của hàng). Enter ở hàng cuối tự thêm hàng mới.
  - Dán danh sách: một textarea "mỗi dòng một cột", tách theo xuống dòng, dấu phẩy hoặc chấm phẩy, tự cắt khoảng trắng và bỏ dòng rỗng; đếm trực tiếp và báo trùng theo dòng. Chuyển về "Từng cột" sẽ đổ ra hàng.
- Đếm "6/10 cột" cạnh nhãn; đủ 10 thì nút thêm ẩn và hiện HvNotice info.
- Xem trước: dải tiêu đề mô phỏng bảng chấm (cột 76px, tên clamp 2 dòng) để thấy tên nào bị cắt.
- Gợi ý mẫu (chip HvBadge bấm được): "4 kỹ năng IELTS", "Toán tiểu học", "Kiểm tra 15p / 1 tiết". Chỉ là mẫu điền sẵn phía web, không cần API.

### 5.2 Danh sách bộ điểm trên `class-config-page.tsx`

- Mỗi bộ điểm là một HvCard flat: tên + badge "n cột"; các cột hiển thị dạng chip HvBadge neutral sm, wrap; dòng phụ "Đang dùng ở N lớp" (cần API tuỳ chọn, xem mục 7).
- Xoá: vẫn hai bước tại chỗ, nhưng chặn khi đang được dùng (nếu có N lớp) với lý do rõ.

### 5.3 Gán bộ điểm (`AssignScoreSetDialog`)

- Thay `<select>` bằng danh sách thẻ chọn (radio card): tên bộ + chip cột, để thấy cột trước khi gán. Bộ đang gán (nếu snapshot khớp tên) đánh dấu "Đang dùng".
- Nếu lớp đã có điểm: hiện HvNotice warning ngay khi mở, khoá nút Gán và Xoá gán, thay vì đợi 409.
- Bảng "Gán bộ điểm cho lớp": cột Bộ điểm hiện chip tên bộ + số cột thay cho chỉ một nút; dưới `sm` chuyển sang thẻ.

## 6. Tiêu chí chấp nhận

- Bộ điểm 8 cột, lớp 20 học sinh: chấm đủ trên panel 400px không cần cuộn ngang; trên bảng đầy đủ 1280px không cần cuộn ngang.
- Mọi ô nhập và nút ≥ 44px; không còn `type=number`.
- Enter/Tab đi đúng như 4.2; tiêu đề cột luôn nhìn thấy khi cuộn dọc.
- Ô chưa lưu phân biệt được bằng mắt và đếm được trên thanh trạng thái; đóng panel khi còn ô chưa lưu phải hỏi.
- Tạo bộ 8 cột bằng cách dán 8 dòng trong một thao tác; lỗi trùng chỉ đúng hàng.
- Gán bộ điểm thấy cột trước khi bấm Gán; lớp đã có điểm biết bị khoá ngay khi mở.

## 7. API tuỳ chọn (không bắt buộc cho đợt 1)

- `GET /score-sets` thêm `class_count` (đếm `class_score_components.source_set_id`), cho 5.2.
- `GET /classes/:id/score-components` thêm `has_scores: bool`, cho 5.3 khoá sớm. Hiện web chỉ biết qua 409.

## 8. Thứ tự làm

1. `HvScoreInput`, `HvModal size`, `use-score-draft` (nền tảng, không đổi hành vi).
2. Chế độ theo học sinh trong panel (thay grid hiện tại, giải quyết S1–S8).
3. Bảng đầy đủ trong modal xl.
4. Trình soạn bộ điểm: hàng 48px + lỗi theo hàng + dán danh sách + đếm + xem trước.
5. Danh sách bộ điểm dạng chip và assign dialog dạng radio card.
6. API tuỳ chọn mục 7.

## Câu hỏi còn mở

- Giữ nút "Lưu điểm" song song với tự lưu khi rời ô (đề xuất), hay chỉ một trong hai?
- Mẫu bộ điểm gợi ý (IELTS, Toán tiểu học...) có phù hợp với tập khách hàng hiện tại không, hay bỏ?
- Có cần trọng số/điểm tối đa theo cột không? Nếu có thì là thay đổi schema, ngoài phạm vi này.
