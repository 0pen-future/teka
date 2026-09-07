---
phase: 1
title: "Fold tiếng Việt, lọc học sinh, ?q= trên URL"
status: completed
priority: P1
effort: "3h"
dependencies: []
---

# Phase 1: Fold tiếng Việt, lọc học sinh, `?q=` trên URL

## Overview

Đặt nền logic thuần (không UI): helper bỏ dấu tiếng Việt dùng chung, hàm lọc + tìm khoảng khớp trong tên học sinh để tô đậm, và state `q` trên URL của trang. Xong phase này trang chưa đổi giao diện, chỉ có thêm state và test.

## Requirements

- Functional:
  - `foldVietnamese("Nguyễn Đức")` → `"nguyen duc"` (NFD, bỏ `̀–ͯ`, `đ/Đ → d`, lowercase). Hành vi **y hệt** `foldVietnamese` cục bộ đang ở `zalo-friend-picker.tsx` (regex `[̀-ͯ]` ≡ `[̀-ͯ]`).
  - `findFoldedMatch(text, query)` trả `{ start, end }` trên **chuỗi gốc** hay `null`; đúng cả khi `text` lưu dạng NFD (độ dài fold ≠ độ dài gốc) — không dùng `q.length` để cắt như mockup.
  - `filterStudentRows(rows, query)` giữ thứ tự gốc; `query.trim() === ""` → trả nguyên `rows`.
  - `?q=` đọc/ghi qua `useSearchParams` với `replace: true`; xoá param khi rỗng; giữ `class_id`.
  - Đổi lớp xoá `q` (D2): `selectClass` set `class_id` **và** delete `q` trong cùng một lần `setSearchParams` để chỉ có một entry history.
- Non-functional: thuần hàm, không phụ thuộc React trừ phần URL; test đơn vị cho mọi nhánh.

## Architecture

```
lib/utils/vietnamese.ts        foldVietnamese, findFoldedMatch      (dùng chung toàn app)
        ▲                                 ▲
        │ import                          │ import
roster/components/zalo-friend-picker.tsx  teaching/lib/student-search.ts  filterStudentRows
                                                   ▲
                                                   │ useMemo(rows, query)
                                          teaching/pages/records-page.tsx  query = searchParams.get("q")
```

`findFoldedMatch` gấp từng code point của `text`, dựng mảng `originalIndexByFoldedIndex`; tìm `indexOf(foldedQuery)` trên chuỗi gấp rồi ánh xạ ngược ra `[start, end)` gốc. Combining mark đứng lẻ (input NFD) gấp thành chuỗi rỗng nên không có chỉ số riêng.

## Related Code Files

- Create: `apps/web/src/lib/utils/vietnamese.ts`
- Create: `apps/web/src/lib/utils/__tests__/vietnamese.test.ts`
- Create: `apps/web/src/features/teaching/lib/student-search.ts`
- Create: `apps/web/src/features/teaching/__tests__/student-search.test.ts`
- Modify: `apps/web/src/lib/utils/index.ts` (export 2 hàm mới)
- Modify: `apps/web/src/features/roster/components/zalo-friend-picker.tsx` (xoá `foldVietnamese` cục bộ, import từ `@/lib/utils`)
- Modify: `apps/web/src/features/teaching/pages/records-page.tsx` (thêm `query`/`setQuery`, `filteredRows` — chưa render ô tìm)

## Implementation Steps

1. Viết `vietnamese.ts` với `foldVietnamese` và `findFoldedMatch` (JSDoc nêu lý do map chỉ số). Export qua `lib/utils/index.ts`.
2. Sửa `zalo-friend-picker.tsx` dùng helper chung; chạy test hiện có của roster để chứng minh không đổi hành vi (`npx vitest run src/features/roster`).
3. Viết `teaching/lib/student-search.ts`: `filterStudentRows<T extends { name: string }>(rows: T[], query: string): T[]` dùng `findFoldedMatch(row.name, query) !== null`.
4. Trong `records-page.tsx`: `const query = searchParams.get("q") ?? ""`; hàm `setQuery(next)` → `setSearchParams(prev => { next.trim() ? set("q", next) : delete("q") }, { replace: true })`; `const filteredRows = useMemo(() => filterStudentRows(rows, query), [rows, query])`. Bảng vẫn nhận `rows` ở phase này (đổi sang `filteredRows` ở Phase 4) để UI không đổi sớm.
   Sửa `selectClass` hiện có: trong callback `setSearchParams(prev => …)` thêm `next.delete("q")` sau khi set `class_id` (giữ `replace: true`).
5. Test đơn vị: fold (dấu, đ, hoa/thường), match (NFC, NFD, không khớp, query rỗng → null), filter (giữ thứ tự, trim).

## Todo

- [x] `vietnamese.ts` + test
- [x] `zalo-friend-picker.tsx` dùng helper chung, test roster xanh
- [x] `student-search.ts` + test
- [x] `records-page.tsx` đọc/ghi `?q=` (chưa render UI)
- [x] `selectClass` xoá `q` cùng lúc đổi `class_id` (một entry history)

## Success Criteria

- [x] `foldVietnamese("Trần Đức Ánh") === "tran duc anh"`; `findFoldedMatch("Nguyễn Văn An", "nguyen")` → `{start:0,end:6}`; với input NFD `"Nguyẽn"` → end đúng vị trí gốc.
- [x] `npx vitest run src/lib src/features/roster src/features/teaching` xanh; `records-pages.test.tsx` không sửa.
- [x] Không thay đổi thị giác nào trên `/records` sau phase này.

## Risk Assessment

- **Trùng lặp helper** nếu chỉ copy từ zalo-friend-picker → bắt buộc bước 2 (DRY). Tín hiệu: còn 2 định nghĩa `foldVietnamese` trong repo → chưa xong.
- **`useSearchParams` re-render mỗi phím**: ≤100 dòng/lớp nên chấp nhận; nếu đo được giật (>16ms/phím) thì thêm `useDeferredValue(query)` cho phần lọc, không đổi URL contract.

## Kết quả

Commit `2e39587`. Không lệch.
