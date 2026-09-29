# Enhance UX / AX — `apps/web`

- Ngày: 2026-09-29 · Chế độ: default (chỉ review → phân tích → đề xuất → DONE; **không** triển khai)
- Commit: `ab092bc` (master)
- Evidence: `plans/reports/enhance-ux-ax-260929-1051-web/round-1/`

## Verdict

**Chưa đạt — cần làm các mục Must trước khi coi là "polished".**

Nền tảng tốt. `hv` kit có bản sắc rõ: mint, Baloo 2 + Nunito và nút "press". Dashboard có lời chào và cảnh báo điểm danh quá hạn kèm CTA. Có skip-link, `lang="vi"` và `prefers-reduced-motion`.

Có ba lỗi hệ thống kéo điểm xuống:

1. **Tràn ngang trên mobile/tablet** ở 9 route: `/classes` tràn 487 px ở 375 px. Nguyên nhân đã được chứng minh (xem M1).
2. **Nút primary chữ trắng trên `mint-400` chỉ đạt 2.03:1.** Mọi CTA chính đều trượt WCAG AA.
3. **Không có lớp chặn index nào.** Không có `robots.txt`, `X-Robots-Tag` hay meta robots, trong khi trang sao kê `/s/:token` chứa dữ liệu cá nhân và được gửi qua Zalo.

Còn vài lỗi nội dung ngay trước mặt người dùng: trang 404 viết tiếng Anh, footer phụ huynh có số Zalo giả `0900 000 000`, và `/billing` không có h1 lại có 41 nút "Điểm danh" giống hệt nhau.

## Phạm vi và môi trường

| Mục | Giá trị |
|---|---|
| Sản phẩm | App B2B private cho trung tâm dạy thêm. Phụ huynh chỉ mở link sao kê `/s/:token` (qua Zalo). Không có landing marketing: `/` chuyển hướng về login. |
| Stack | Vite 8, React 19, React Router (lazy routes), Tailwind v4, `hv` design system, shadcn `components/ui`, TanStack Query, Zustand. Prod: nginx SPA fallback sau Cloudflare và Traefik. |
| Môi trường 1 | Stack isolated `teka-commit-2237785` tại `http://localhost:55183` (dữ liệu seed). Đã tái hiện lỗi tràn và chạy discovery scan ở đây. |
| Môi trường 2 | Production `https://teka-web.cauchuyenlaptrinh.com`, **read-only**. Người dùng đã đồng ý; chỉ GET/xem, không bấm hành động ghi. |
| Viewport | 1440×900, 768×1024, 375×812 và reflow 320×720. |
| Dev server | Không khởi động server nào; dùng stack sẵn có và **không** dừng nó. |

**Các kiểm tra KHÔNG chạy được. Chúng không được tính là pass:**

- **Trang sao kê hợp lệ `/s/:token`.** Lấy token cần đọc DB và lệnh đọc đã bị từ chối. Tôi chỉ chụp được trạng thái token không hợp lệ.
- **Chụp khung hình motion (transition/animation).** Chỉ xem ảnh tĩnh và đọc code `prefers-reduced-motion`.
- **Validator structured data.** Không áp dụng vì không có JSON-LD (xem phần AX).
- **Không có skill SEO/GEO nào được cài** để đối chiếu thêm.

## Evidence baseline

| Nhóm | Đường dẫn | Ghi chú |
|---|---|---|
| Trang public | `round-1/{login,forgot,reset-invalid,invite-invalid,statement-invalid,notfound,root}/{375x812,768x1024,1440x900}.png` | Đủ ba viewport |
| App đã đăng nhập (prod) | `round-1/app-prod/<slug>/<viewport>-<slug>-loaded.png` và `page-facts.json` | 23 route × 4 viewport. **Được git-ignore** (`app-prod/.gitignore` = `*`) vì chứa dữ liệu thật. Report chỉ mô tả, không trích dữ liệu cá nhân. |
| Discovery scan | `round-1/discovery-scan-dev.txt` | Quét `localhost:55183` |

Tràn ngang đo trên prod (`scrollWidth - clientWidth` của `html`, đơn vị px):

| Route | 768 | 375 | 320 |
|---|---|---|---|
| `/classes` | 174 | 487 | 542 |
| `/classes/recruiting` | 174 | 487 | — |
| `/courses` | 202 | 515 | — |
| `/class-invitations` | 0 | 227 | — |
| `/library` (cả 3 tab) | 0 | 116 | — |
| `/paths` | 0 | 205 | — |
| `/classbook` | 0 | 22 | 77 |
| `/lesson-plans` | 0 | 0 | 36 |
| `/profile` | 0 | 0 | 36 |

*Ghi chú:* trong ảnh full-page, bottom nav cố định và sticky bar của billing xuất hiện giữa trang. Đó là do cách chụp, không phải bug.

## Kết quả discovery scan (`localhost:55183`)

- **ERROR (4):**
  - `robots.txt` trả 200 HTML (SPA fallback).
  - `sitemap.xml` trả HTML.
  - Thiếu meta description.
  - Thiếu `og:image`.
- **WARN (10):** thiếu `llms.txt`/`llms-full.txt`, canonical, `og:title`/`og:description`/`og:url`, twitter card và JSON-LD. Server HTML có dưới 200 ký tự. Markdown twin trả HTML.
- **INFO (2):** không có markdown alternate và không có content negotiation.

Đối chiếu với bối cảnh sản phẩm (app private, có dữ liệu cá nhân):

| Phát hiện | Quyết định | Lý do |
|---|---|---|
| `robots.txt` trả HTML | **Sửa (Must)** | Crawler không đọc được luật, và cũng không có luật chặn nào |
| Meta description, OG, `og:image`, twitter card | **Sửa (Should)**, chỉ cho `/login` và `/invite/*` | Link mời giáo viên và link login được dán vào Zalo; preview có thương hiệu giúp tăng tin cậy |
| `sitemap.xml` | **Chấp nhận thiếu.** Nên trả 404 thay vì HTML | Không có trang nào muốn được index |
| `llms.txt`, `llms-full.txt`, markdown twin, content negotiation, JSON-LD, canonical | **Chấp nhận thiếu** | Không có nội dung public để agent trích dẫn. Mở ra các surface này chỉ tăng rủi ro lộ dữ liệu. `llms.txt` cũng không phải đòn bẩy ranking. |
| Server HTML rỗng | **Chấp nhận** | SPA sau đăng nhập; không cần SSR |

## Chấm điểm rubric (0–3)

| Hạng mục | Điểm | Evidence |
|---|---|---|
| Brand recall | 2 | Bảng màu mint/cream nhất quán, font Baloo 2 và nút press rất riêng. Tuy vậy trang login, cửa ngõ đầu tiên, chỉ là một card trắng: không logo mark, không tagline (`round-1/login/1440x900.png`). |
| First impression | 1 | Login trơn và subtitle mờ (3.01:1). 404 tiếng Anh và lệch style (`notfound/*.png`). Dashboard sau đăng nhập thì tốt. |
| Content punch | 2 | Copy tiếng Việt rõ và thân thiện. Nhưng footer phụ huynh có số Zalo và email placeholder, cùng 3 "link" giả là `<span>` (`app-footer.tsx`). |
| Hierarchy | 1 | Không có `document.title` riêng: mọi tab đều là "Teka". `/billing` không có h1 và có tường 41 nút coral giống nhau. Sidebar có 22 mục với 3 mục trùng nghĩa ("Quản lý lớp học", "Danh sách lớp học", "Lớp & học sinh"). |
| Motion | 2 (chưa xác minh động) | Có khối `prefers-reduced-motion` (`globals.css:237`) và press animation nhẹ. Không có frame capture. |
| Responsive | 0 | 9 route tràn ngang; `/classes` tràn 487 px ở 375. `HvSegmented` của library tràn (nút tới x=491). Empty state của `/courses` nằm giữa bảng 1000 px nên trên mobile không nhìn thấy. |
| Trust | 1 | Thông tin liên hệ giả trên trang phụ huynh xem học phí. Không chặn index trang có dữ liệu cá nhân. Console có lỗi 401 `/auth/refresh` trên trang public. |
| Accessibility | 1 | Primary button 2.03:1; `ink-400` 2.81:1; chữ `mint-600` trên trắng 4.08:1. `/center/permissions` có 22 target dưới 24 px. Nút 404 dưới 44 px. Điểm cộng: skip-link, `lang="vi"`, kích thước `hv` từ 44 px trở lên. |
| AX / discovery | 1 | Xem mục trên. Mục tiêu đúng ở đây là **chặn index**, và hiện chưa có lớp chặn nào. |

## Đề xuất (xếp hạng)

### Must

**M1. Sửa tràn ngang mobile/tablet**

- **Nguyên nhân (đã chứng minh trên stack seed):**
  - `span.sr-only` (`position:absolute`) nằm trong `th` của các bảng có `min-w-[760–1000px]`.
  - Wrapper `overflow-x-auto` không phải positioned, nên containing block của span là `html`. Span vì thế nằm ở left≈889 px ngoài vùng cuộn, không bị scroll container cắt, và kéo `html.scrollWidth` lên 890 trong khi `body` chỉ 375.
  - Chỉ 1 trong 19 wrapper `overflow-x-auto` có `relative`.
- **Thay đổi:**
  - Thêm `relative` vào mọi wrapper bảng cuộn ngang. Tốt nhất là gom thành một primitive dùng chung, ví dụ `HvTableScroll` trong `components/hv`, để không tái phát.
  - `HvSegmented`: cho `overflow-x-auto` và `snap-x`, hoặc cho xuống dòng trên màn hình dưới `sm`.
  - Empty state của `/courses`: đưa ra ngoài bảng hoặc `sticky left-0` trong viewport.
  - Rà thêm `/classbook`, `/lesson-plans` và `/profile` ở 320 px.
- **Files:**
  - `features/roster/components/class-table.tsx:49`
  - `features/courses/pages/courses-page.tsx:150`
  - `learning-paths-page.tsx`, `class-invitations-page.tsx`
  - Các bảng khác có `sr-only` trong `th`: `permission-matrix`, `class-score-set-table`, `exercises-bank`, `materials-bank`, `lessons-table`, `score-sets-editor`, `lesson-exercises`, `classes-tab`, `roster-table`
  - `components/hv/hv-segmented.tsx`
- **Acceptance:**
  - `document.documentElement.scrollWidth - clientWidth === 0` trên cả 23 route ở 375×812 và 768×1024, và bằng 0 ở 320×720 cho các route đã liệt kê.
  - Bảng vẫn cuộn ngang bên trong wrapper.
  - "Không có khóa nào khớp." nằm trong viewport 375.

**M2. Tương phản nút primary**

- **Thay đổi.** Hiện tại chữ trắng trên `mint-400` chỉ đạt 2.03:1. Chọn một trong hai:
  - (a) Chữ `ink-900` `#1c3a31` trên `mint-400`: **6.96:1**, giữ nguyên màu thương hiệu. Khuyến nghị phương án này.
  - (b) Nền `mint-700` `#1f6b53` với chữ trắng.
  - Không dùng `mint-600` + chữ trắng: chỉ 4.08:1, vẫn trượt AA cho chữ thường.
- **Files:** `components/hv/hv-button.tsx`. Kiểm tra thêm các biến thể nền mint khác trong `hv`.
- **Acceptance:**
  - Mọi biến thể `HvButton` đạt ≥ 4.5:1 cho chữ, đo bằng axe hoặc script contrast.
  - Ảnh chụp login và dashboard được so trước/sau bằng vision, không lệch thương hiệu.

**M3. Chặn index toàn app, đặc biệt trang có dữ liệu cá nhân**

- **Thay đổi:**
  - Thêm `apps/web/public/robots.txt` với `User-agent: *` / `Disallow: /`.
  - Trong nginx, thêm `location = /robots.txt` trả `text/plain`, và cho `/sitemap.xml`, `/llms.txt` trả 404 thay vì HTML.
  - Thêm `add_header X-Robots-Tag "noindex, nofollow" always;` cho mọi location. Header hiện bị lặp ở mỗi block, nên gom vào một `include` snippet để không sót.
  - Thêm `<meta name="robots" content="noindex, nofollow">` trong `index.html`.
- **Lưu ý:** `robots.txt` Disallow đơn thuần không ngăn index URL đã lộ. Chính `X-Robots-Tag` mới chặn được, nên cần cả hai.
- **Files:** `apps/web/public/robots.txt` (mới), `apps/web/nginx.conf`, `apps/web/index.html`.
- **Acceptance:**
  - `curl -sI <prod>/robots.txt` trả `content-type: text/plain`.
  - `curl -sI <prod>/s/x` và `/login` đều có `x-robots-tag: noindex`.
  - Discovery scan hết lỗi `[robots]`, và `sitemap.xml` trả 404.

**M4. Footer trang phụ huynh: bỏ thông tin giả**

- **Thay đổi:**
  - Không để số Zalo `0900 000 000` và `hotro@teka.vn` là placeholder.
  - Có hai hướng:
    - Hiển thị liên hệ **của trung tâm** phát hành sao kê, lấy từ API statement (tên trung tâm, SĐT hoặc Zalo) nếu có.
    - Tạm ẩn khối liên hệ.
  - Bỏ ba `<span>` giả link ("Hướng dẫn sử dụng", "Bảng giá", "Câu hỏi thường gặp") cho tới khi có trang thật.
- **Files:** `components/shared/app-footer.tsx`. Có thể thêm cả statement feature nếu cần dữ liệu trung tâm.
- **Acceptance:**
  - Không còn chuỗi `0900000000`/`0900 000 000` trong `src`.
  - Footer không có phần tử trông như link mà không bấm được.
  - Chữ footer đạt ≥ 4.5:1.

**M5. Trang 404 tiếng Việt, dùng `hv`**

- **Thay đổi:**
  - Copy mới: "Không tìm thấy trang" và CTA "Về trang chủ". CTA trỏ về `/` hoặc về login nếu chưa đăng nhập.
  - Dùng `HvButton` (≥ 44 px) thay shadcn outline.
  - Bỏ `404` dạng `font-mono` mờ (2.81:1), dùng Baloo 2 với màu `ink-700` trở lên.
- **Files:** `components/shared/not-found.tsx`.
- **Acceptance:**
  - Không còn chuỗi tiếng Anh.
  - Target ≥ 44×44 ở 375.
  - Contrast ≥ 4.5:1 (hoặc ≥ 3:1 cho chữ lớn).

**M6. `/billing/:periodId`: cấu trúc và tường nút**

- **Thay đổi:**
  - Thêm h1 (ví dụ "Chốt số tháng 9/2026" làm h1).
  - Gom các dòng thiếu điểm danh theo lớp, dạng collapsible, và thay 41 nút lặp bằng một CTA tổng kiểu "Điểm danh N buổi còn thiếu", mỗi lớp một link.
- **Files:** trang billing period trong `features/billing/`.
- **Acceptance:**
  - Có đúng 1 h1.
  - Ở 375, không quá 1 CTA coral trong màn đầu tiên.
  - Mọi buổi thiếu vẫn truy cập được sau tối đa 2 lần bấm.

### Should

**S1. Tiêu đề tab theo route**

- **Thay đổi:** thêm hook `useDocumentTitle` hoặc dùng `handle.title` của route, rồi set theo dạng `"<Trang> · Teka"`.
- **Files:** `app/router.tsx` và layouts.
- **Acceptance:** 23 route có `document.title` khác nhau và không route nào chỉ là "Teka".

**S2. Link preview cho Zalo (`/login` và `/invite/*`)**

- **Thay đổi:**
  - Thêm meta description, `og:title`/`og:description`/`og:image` (PNG 1200×630, URL tuyệt đối) và `twitter:card` trong `index.html`.
  - Vì là SPA, dùng giá trị chung cho mọi route. Không đưa tên người, tên lớp hay số tiền vào OG.
- **Acceptance:**
  - Scan hết `[meta-description]` và `[open-graph]`.
  - Dán link vào Zalo thấy preview có logo.

**S3. Làm thương hiệu cho login**

- **Thay đổi:**
  - Thêm logo mark (đã có `favicon.svg`/icon PWA) và một dòng giá trị, ví dụ "Quản lý lớp, điểm danh và thu tiền trong một nơi".
  - Subtitle (hiện 3.01:1) và "Quên mật khẩu?" (hiện 4.08:1) phải đạt ≥ 4.5:1.
- **Acceptance:**
  - Ảnh chụp ba viewport có logo trong màn đầu tiên.
  - Contrast đạt.

**S4. Rút gọn IA sidebar**

- **Thay đổi:**
  - Gộp 3 mục lớp trùng nghĩa và nhóm 22 mục thành 4–5 nhóm có tiêu đề.
  - Cần quyết định sản phẩm (xem Câu hỏi 2).
- **Files:** `layouts/dashboard-layout.tsx` và cấu hình nav.
- **Acceptance:**
  - Không có hai mục cùng đích hoặc cùng nghĩa.
  - Không quá 7 mục mỗi nhóm.

**S5. Touch target `/center/permissions`**

- **Thay đổi:** checkbox và ô ma trận phải đạt ≥ 24×24 (tối thiểu WCAG 2.5.8), nên đạt 44 trên mobile.
- **Files:** `permission-matrix.tsx`.
- **Acceptance:** `smallTargets === 0` trên route này ở 375.

**S6. Token chữ phụ**

- **Thay đổi:** chữ phụ nhỏ không dùng `ink-400` (2.81:1). Chuyển sang `ink-500` `#5b756c` (4.65:1 trên cream). Rà các chỗ dùng `text-ink-400` cho chữ mang thông tin.
- **Acceptance:** axe không còn lỗi `color-contrast` trên login, dashboard và statement-invalid.

**S7. Tài liệu thiết kế cho agent và reviewer**

- **Thay đổi:**
  - Tạo `DESIGN.md` gốc tóm tắt token, `hv` kit, quy tắc contrast và bảng cuộn, dẫn link sang `docs/frontend-guidelines.md` thay vì chép lại.
  - Thêm mục UX vào `REVIEW.md` với các check M1–M3 dạng câu hỏi review.
  - Link `DESIGN.md` từ `AGENTS.md`/`CLAUDE.md`.
- **Acceptance:** các file tồn tại, link hoạt động và không trùng nội dung với `frontend-guidelines.md`.

### Could

**C1. Tắt lỗi console 401 trên route public**

- **Thay đổi:** không gọi `/auth/refresh` khi chưa có dấu hiệu phiên (cookie hoặc flag), hoặc nuốt 401 dự kiến.
- **Acceptance:** 0 lỗi console khi mở `/login` và `/s/<invalid>`.

**C2. `llms.txt`, sitemap, JSON-LD**

- **Thay đổi:** không áp dụng lúc này. Chỉ xem xét nếu sau này có site marketing public, và khi đó đặt ở domain hoặc route riêng, không phải app.

## Project DONE contract

Implementation (`--auto`/`--loop`) chỉ được coi là xong khi **tất cả** điều kiện sau đúng. Không được nới lỏng; nếu một check sai, ghi lý do và thay bằng check tương đương hoặc mạnh hơn.

1. **Responsive:**
   - `html.scrollWidth - clientWidth === 0` trên 23 route đã đăng nhập, cùng login, forgot, 404 và statement-invalid, ở 1440×900, 768×1024 và 375×812.
   - Thêm 320×720 cho các route đã liệt kê trong M1.
   - Đo lại bằng script capture tương đương `capture-app.mjs`.
2. **Contrast:** mọi `HvButton` và chữ mang thông tin trên login, dashboard, 404, footer statement và billing đạt ≥ 4.5:1 (≥ 3:1 cho chữ lớn). axe không có lỗi `color-contrast` trên các trang đó.
3. **Chặn index:**
   - `robots.txt` trả `text/plain` với `Disallow: /`.
   - `X-Robots-Tag: noindex` có mặt trên `/`, `/login`, `/s/*`, `/invite/*` và `/reset-password/*`.
   - `sitemap.xml` và `llms.txt` trả 404.
   - Có meta robots noindex trong HTML.
4. **Nội dung:**
   - Không còn placeholder liên hệ hay link giả trong `app-footer.tsx`.
   - 404 hoàn toàn tiếng Việt.
   - Mọi route có `document.title` riêng.
5. **Cấu trúc:**
   - Mỗi route có đúng 1 h1, kể cả `/billing/:periodId`.
   - `/center/permissions` không có target dưới 24 px.
6. **Preview:** discovery scan không còn `[meta-description]` hay `[open-graph]` ERROR/WARN. Các WARN `llms`, JSON-LD, markdown và canonical được chấp nhận theo lý do ở trên.
7. **Không hồi quy:** `npm run lint`, `typecheck`, `test` và `build` của `apps/web` đều xanh. Không sửa tay `src/components/ui`.
8. **Vision:** ba viewport được chụp lại và so với baseline `round-1/`. Không có lệch thương hiệu và không có phần tử bị cắt hoặc chồng.
9. **Riêng tư:** ảnh chụp có dữ liệu thật chỉ nằm trong thư mục git-ignored. Report và commit không chứa dữ liệu cá nhân hay thông tin đăng nhập.

## Câu hỏi chưa giải quyết

1. **Liên hệ ở footer trang sao kê:** nên hiển thị liên hệ của **trung tâm** (API statement có trả không?) hay của Teka? Hay ẩn hẳn?
2. **IA sidebar:** 3 mục "Quản lý lớp học", "Danh sách lớp học" và "Lớp & học sinh" có phục vụ vai trò khác nhau không, hay có thể gộp?
3. **Nút primary:** chọn chữ tối trên `mint-400` (giữ màu) hay nền `mint-700` chữ trắng (đổi cảm giác màu)?
4. **Sao kê hợp lệ:** cần một token `/s/:token` test (không phải prod) để audit trang phụ huynh chính. Hiện trang này chưa được đánh giá.
5. **Ảnh OG 1200×630:** có asset thương hiệu nào sẵn không, hay cần thiết kế mới?
