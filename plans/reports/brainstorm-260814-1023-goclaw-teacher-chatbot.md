# Brainstorm — Chatbot AI cho giáo viên (GoClaw sidecar)

Status: accepted contract, chờ plan. Ngày: 2026-08-14.

## Contract

**Outcome.** Giáo viên mở panel chat trong web app Teka, trò chuyện với AI
assistant chuyên **soạn nội dung sư phạm**: nhận xét học sinh, giáo án, tin
nhắn gửi phụ huynh — bám vào ngữ cảnh lớp/học sinh đang xem (classbook,
records của prototype v2). Agent chạy trên **GoClaw** (nextlevelbuilder/goclaw)
deploy như một sidecar service cạnh stack Teka.

**Constraints.**
- GoClaw license **CC BY-NC 4.0 (phi thương mại)**. Quyết định của user:
  dùng cho giai đoạn prototype/pre-revenue. Điều kiện bắt buộc: phải mua
  commercial license hoặc thay engine **trước khi thu phí** (PRD goal G5).
  Ghi ADR khi implement.
- GoClaw là service stateful mới trong docker-compose (`make dev` phải dựng
  được cả stack); Go 1.26+ single binary, cần Postgres riêng hoặc schema riêng.
- Tích hợp qua API chính thức của GoClaw (WebSocket realtime / REST webhook,
  Bearer/HMAC). Không fork/patch goclaw.
- Auth: chỉ giáo viên đã đăng nhập Teka dùng được chat; mapping
  teka-teacher ↔ goclaw workspace/user phải cách ly dữ liệu theo giáo viên.
- Dữ liệu teaching (classbook, giáo án, nhận xét) hiện sống ở **client-side
  store** (`apps/web/src/features/teaching/lib/teaching-store.ts`, prototype
  v2) — chưa có backend. Bản đầu: web client tự đính ngữ cảnh lớp/học sinh
  vào tin nhắn gửi agent. Tool endpoint server-side chỉ làm khi teaching data
  đã dọn về API.
- Không gửi secrets/API key về client; LLM key nằm trong goclaw config.

**Non-goals.**
- Kênh Zalo/Telegram/khác (dù goclaw hỗ trợ) — web chat trước.
- Agent thực hiện thao tác ghi (điểm danh, phiếu thu, gửi thông báo).
- Hỏi đáp dữ liệu tài chính/điểm danh (billing, statements) — vòng sau.
- Tự viết agent loop trong apps/api (đã chọn goclaw thay thế).
- Chat phụ huynh–giáo viên (PRD non-goal, không đổi).

**Acceptance criteria.**
1. `make dev` dựng goclaw service cùng stack; healthcheck pass.
2. Giáo viên đăng nhập, mở panel chat, yêu cầu "soạn nhận xét cho <học sinh>
   tháng này" → nhận draft stream theo thời gian thực, nội dung phản ánh đúng
   ngữ cảnh lớp/học sinh được đính kèm.
3. Lịch sử hội thoại giữ theo từng giáo viên; giáo viên A không thấy hội
   thoại/dữ liệu của B.
4. Không có secret nào lộ ra client bundle hoặc network tab.
5. ADR ghi quyết định dùng GoClaw + điều kiện license trước thương mại hoá.

## Hướng đã so sánh

| Hướng | Được | Mất |
|---|---|---|
| **GoClaw sidecar (chọn)** | Agent pipeline + memory + streaming có sẵn; đường mở multi-channel (Zalo) sau | License NC (rủi ro đã chấp nhận có điều kiện); thêm service vận hành; dự án trẻ |
| Native trong apps/api | Không vướng license, khớp kiến trúc, gọi thẳng service nội bộ | Tự viết loop + memory; chỉ web |
| Hybrid (native + contract mở) | Nhỏ nhất, mở đường gateway sau | Không được chọn |

User chọn GoClaw sidecar sau khi được cảnh báo license — không tự ý đảo
quyết định này; chỉ mở lại nếu có bằng chứng mới (vd. goclaw không có
commercial licensing path).

## Rủi ro chưa giải quyết

1. Chưa xác minh trong docs.goclaw.sh: cơ chế provision workspace/user qua
   API để map teacher ↔ goclaw identity (cần scout ở bước plan).
2. Cách nhúng web chat: client nối thẳng WebSocket goclaw vs proxy qua
   apps/api (auth, CORS, rate-limit) — quyết ở plan; nghiêng về proxy qua API.
3. Teaching data còn ở client store — chất lượng ngữ cảnh phụ thuộc payload
   client gửi kèm; sẽ nâng cấp thành server-side tools khi teaching có backend.

## Next

`/ak:plan` với contract này → plan dir `plans/260814-1023-goclaw-teacher-chatbot/`.
