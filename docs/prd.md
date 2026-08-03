# PRD — Hệ thống quản lý lớp dạy thêm (V1)

**Trạng thái:** Draft — chưa validate bằng phỏng vấn khách hàng
**Người viết:** Nguyễn Văn Thược
**Ngày:** 03/08/2026

> **Cảnh báo trước khi đọc tiếp:** Spec này được viết từ giả thuyết, chưa có dữ liệu phỏng vấn. Toàn bộ phần Requirements chỉ nên đưa vào build sau khi hoàn thành tối thiểu 10 cuộc phỏng vấn chủ lớp. Nếu build trước, đây là một bản thiết kế đẹp cho một sản phẩm có thể không ai cần.

---

## 1. Problem Statement

Giáo viên dạy thêm quản lý từ 3 lớp trở lên, mỗi lớp khai giảng một ngày khác nhau, học sinh liên tục nhập học giữa chu kỳ. Hệ quả: học phí phải tính theo **số buổi thực học của từng cá thể**, không tính được theo lớp. Đây là điểm Excel sập — không phải vì Excel yếu, mà vì bài toán đã đổi bản chất từ "quản lý danh sách" sang "tính tiền theo từng cá thể".

Cái giá của việc không giải: mỗi tháng giáo viên mất vài giờ đối chiếu thủ công, và quan trọng hơn — **thu thiếu, quên thu, thu nhầm**. Với lớp 150 học sinh, chỉ cần sót 3 người là mất 1,5–2 triệu/tháng. Khoản thất thoát này thường không được phát hiện vì không có sổ đối chiếu.

Phương án thay thế hiện tại (thuê trợ giảng làm sổ sách) đắt hơn nhiều lần và vẫn không loại bỏ được sai sót.

## 2. Goals

| # | Mục tiêu | Cách đo |
|---|---|---|
| G1 | Giáo viên chốt học phí toàn bộ lớp trong dưới 10 phút | Thời gian từ lúc mở màn hình chốt sổ đến lúc gửi xong thông báo |
| G2 | Không còn học sinh bị sót khỏi danh sách thu | Số học sinh có buổi học nhưng không có phiếu thu = 0 |
| G3 | Rút ngắn thời gian thu đủ học phí | Số ngày trung bình từ ngày chốt sổ đến ngày thu đủ ≥90% học phí kỳ |
| G4 | Dữ liệu điểm danh đủ tin cậy để giáo viên không phải dò lại | Tỉ lệ buổi được điểm danh trong vòng 24h ≥ 90% |
| G5 | Chứng minh có người trả tiền | ≥30% giáo viên dùng thử chuyển sang trả phí sau 2 chu kỳ tháng |

**G4 là North Star của V1.** Mọi giá trị phía sau đều là hệ quả toán học của dữ liệu điểm danh. Nếu tỉ lệ này dưới 90%, sản phẩm vô giá trị bất kể báo cáo đẹp đến đâu.

## 3. Non-Goals

| Không làm | Lý do |
|---|---|
| App cài đặt cho phụ huynh | Phụ huynh dạy thêm dùng sản phẩm ~2 lần/tháng. Không đủ tần suất để biện minh cho việc cài app. Rào cản adoption cao gấp nhiều lần link web. |
| Chat, bảng tin, mạng xã hội lớp học | Không phải nỗi đau. Phụ huynh cần biết đã trả bao nhiêu và con có đi học không. Thêm chat là cạnh tranh trực diện với Zalo — cuộc chiến không thể thắng. |
| Cấu hình chính sách học phí tùy biến | SaaS cho micro-business chết vì cấu hình, không chết vì thiếu tính năng. V1 chỉ hỗ trợ một mô hình. |
| Điểm số, nhận xét, giao bài, học liệu | Không liên quan tới dòng tiền. Là sản phẩm khác. |
| Tính năng cho học sinh | Học sinh không có job đủ mạnh trong bài toán này. |
| Cổng thanh toán tự động (thu hộ) | Cần giấy phép trung gian thanh toán hoặc đối tác. V1 chỉ hiển thị QR chuyển khoản, giáo viên tự xác nhận. |

## 4. Quyết định phạm vi quan trọng nhất

**V1 chỉ hỗ trợ một mô hình học phí duy nhất: trả sau, tính theo buổi thực học, chốt theo tháng dương lịch.**

Lý do chọn mô hình này chứ không phải mô hình phổ biến nhất: đây là mô hình duy nhất mà Excel thực sự sập. Lớp thu trước theo khoá cố định thì Excel vẫn dùng tốt — họ không đau, không trả tiền. Ta nhắm đúng nhóm đang đau.

Các mô hình sau **không hỗ trợ ở V1**, và giáo viên dùng chúng tạm thời không phải khách hàng: thu trước theo khoá; gói buổi trả trước; học phí trọn gói không phụ thuộc số buổi; giá khác nhau theo từng học sinh trong cùng lớp.

## 5. User Stories

### Giáo viên (persona chính: chủ lớp 100–300 học sinh)

Ưu tiên từ cao xuống thấp:

1. Là chủ lớp, tôi muốn điểm danh một buổi trong dưới 15 giây, để tôi làm được ngay sau khi tan lớp thay vì để dồn.
2. Là chủ lớp, tôi muốn hệ thống tự tính học phí của từng học sinh theo số buổi họ thực học, để tôi không phải đối chiếu tay khi có người nhập học giữa tháng.
3. Là chủ lớp, tôi muốn thấy ngay ai đã đóng và ai còn nợ bao nhiêu, để tôi không phải lục lại tin nhắn Zalo.
4. Là chủ lớp, tôi muốn gửi thông báo học phí riêng cho từng phụ huynh chỉ bằng một thao tác, để tôi không phải soạn 150 tin nhắn.
5. Là chủ lớp, tôi muốn review lại toàn bộ bảng học phí trước khi gửi, để tôi chịu trách nhiệm được với con số mình gửi đi.
6. Là chủ lớp, tôi muốn thêm học sinh vào lớp đang chạy mà không phải tính lại gì, để việc tuyển sinh giữa kỳ không tạo thêm việc.
7. Là chủ lớp, tôi muốn hệ thống cảnh báo khi có buổi chưa điểm danh, để tôi không dạy xong mà quên tính tiền.
8. Là chủ lớp, tôi muốn sửa lại điểm danh của buổi đã qua, vì thực tế tôi hay điểm danh muộn hoặc nhầm.

### Phụ huynh

1. Là phụ huynh, tôi muốn mở một link và thấy ngay tháng này con học bao nhiêu buổi và phải đóng bao nhiêu, để tôi không phải hỏi lại thầy cô.
2. Là phụ huynh, tôi muốn thấy chi tiết từng buổi con đi và vắng, để tôi tin con số học phí là đúng.
3. Là phụ huynh, tôi muốn có mã QR chuyển khoản ngay trên màn hình đó, để tôi trả luôn khi đang cầm điện thoại.

### Edge case cần cover

- Học sinh nhập học giữa chu kỳ → chỉ tính từ buổi đầu tiên có mặt
- Học sinh nghỉ hẳn giữa chu kỳ → chốt sổ tới buổi cuối cùng, giữ lại nợ nếu có
- Buổi bị hủy do giáo viên → không tính tiền cho ai
- Lớp chưa có buổi nào trong kỳ → không sinh phiếu thu, không gửi thông báo
- Giáo viên sửa điểm danh sau khi đã gửi thông báo → phải có cơ chế xử lý (xem Open Questions)

**Nhóm edge case: một phụ huynh nhiều con**

- Hai con **khác lớp** → mỗi con tính học phí độc lập, nhưng phụ huynh nhận **một** tin nhắn và trả **một** khoản
- Hai con **cùng lớp** → hai dòng điểm danh riêng biệt; giao diện phải phân biệt được (thường trùng họ, dễ tick nhầm)
- Một con học **hai lớp** cùng giáo viên → gộp vào cùng phiếu thu của con đó
- Phụ huynh trả **thiếu** so với tổng của các con → cần quy tắc phân bổ (xem Q8)
- Một con nghỉ hẳn, con kia còn học → không xoá người liên hệ, không xoá lịch sử công nợ của con đã nghỉ

*Không cover ở V1:* bố và mẹ cùng nhận thông báo (P1). Trường hợp hai người khác nhau đóng cho hai con — nếu gặp, giáo viên tạo hai người liên hệ riêng, mô hình 1:n xử lý được mà không cần tính năng gì thêm.

## 6. Requirements

### P0 — Must have

**R1. Quản lý lớp và học sinh**
- Tạo lớp với ngày khai giảng, lịch cố định trong tuần, đơn giá/buổi
- Thêm học sinh vào lớp bất kỳ lúc nào, có ghi nhận ngày bắt đầu
- Một học sinh có thể thuộc nhiều lớp

**Phạm vi dữ liệu — danh sách đóng, không được mở rộng nếu chưa rà soát pháp lý**

| Trường | Chủ thể | Ghi chú |
|---|---|---|
| Họ tên học sinh | Trẻ em | Bắt buộc |
| Ngày nhập học, lớp học | Trẻ em | Bắt buộc, phục vụ tính tiền |
| Tuổi/khối lớp | Trẻ em | **Đề nghị bỏ** — xem phân tích dưới |
| Số điện thoại người liên hệ | Người lớn | Bắt buộc, để gửi thông báo |

**Mô hình quan hệ — quyết định kiến trúc, không được đơn giản hoá thêm**

Số điện thoại **không** nằm trên bản ghi học sinh. Tách thành thực thể riêng:

```
Người liên hệ  ──1:n──  Học sinh  ──< Ghi danh >──  Lớp
   (SĐT)                              (đơn giá)
```

- Một người liên hệ có nhiều con. Mỗi học sinh trỏ về **đúng một** người liên hệ.
- **Không tạo thực thể "gia đình".** Gia đình chỉ là tập các học sinh cùng trỏ về một người liên hệ — suy ra được, không cần lưu.
- V1 mỗi học sinh chỉ có một số điện thoại. Bố và mẹ cùng nhận thông báo là **P1**, không phải P0.
- **Đơn giá nằm ở bản ghi ghi danh, không nằm ở lớp.** Mặc định kế thừa đơn giá lớp. V1 không cho sửa, nhưng cấu trúc phải sẵn sàng — đây là điểm duy nhất cho phép thêm giảm giá anh chị em ở P1 mà không phải viết lại.

Nếu gắn SĐT thẳng vào học sinh, mọi thứ trong Mục 6 liên quan tới gộp thông báo và gộp công nợ sẽ phải làm lại từ đầu.

Không thu thập: ảnh, ngày sinh đầy đủ, địa chỉ, trường đang học, sức khỏe, điểm số, ghi chú hành vi. Bất kỳ đề xuất thêm trường nào cũng phải kèm câu trả lời "trường này phục vụ tính tiền như thế nào" — nếu không trả lời được thì không thêm.

*Acceptance criteria bổ sung:*
- [ ] Given giáo viên tạo học sinh, When form hiển thị, Then không có trường nào ngoài danh sách trên
- [ ] Given giáo viên xoá học sinh, When xác nhận, Then dữ liệu cá nhân bị xoá thật, chỉ giữ lại bản ghi tài chính đã ẩn danh

*Acceptance criteria:*
- [ ] Given lớp đã có 10 buổi học, When thêm học sinh mới, Then học sinh chỉ được tính tiền từ buổi kế tiếp trở đi
- [ ] Given một học sinh học 2 lớp của cùng giáo viên, When chốt sổ, Then hệ thống sinh một phiếu thu gộp

**R2. Điểm danh 1 chạm**
- Mở buổi học → mặc định toàn bộ học sinh có mặt
- Giáo viên chỉ tick người vắng
- Xác nhận buổi bằng một thao tác
- Sửa lại được điểm danh của buổi đã qua

*Acceptance criteria:*
- [ ] Given lớp 30 học sinh và 2 người vắng, When giáo viên điểm danh, Then số thao tác tối đa là 3 (tick 2 người vắng + xác nhận)
- [ ] Given buổi học đã xác nhận từ 3 ngày trước, When giáo viên mở lại, Then vẫn sửa được và học phí tự cập nhật
- [ ] Given buổi học đã qua nhưng chưa điểm danh, When giáo viên mở app, Then thấy cảnh báo ngay màn hình đầu

**R3. Tính học phí theo cá thể**
- Học phí kỳ = (số buổi có mặt trong kỳ × đơn giá) + nợ kỳ trước
- Tính độc lập cho từng học sinh
- Tính lại tự động khi điểm danh thay đổi

*Acceptance criteria:*
- [ ] Given 3 học sinh trong cùng lớp có số buổi khác nhau, When chốt sổ, Then mỗi người ra một số tiền khác nhau, đúng công thức
- [ ] Given học sinh còn nợ 500k kỳ trước, When chốt kỳ này, Then số nợ cũ hiển thị tách riêng khỏi phát sinh kỳ này

**R4. Chốt sổ và review**
- Một màn hình duy nhất hiển thị toàn bộ học sinh × số buổi × thành tiền của kỳ
- Giáo viên review, sửa được thủ công từng dòng (có ghi chú lý do)
- Xác nhận chốt → khoá kỳ, sinh phiếu thu

*Acceptance criteria:*
- [ ] Given giáo viên đang ở màn hình chốt sổ, When có buổi chưa điểm danh trong kỳ, Then hệ thống chặn chốt và chỉ rõ buổi nào
- [ ] Given kỳ đã chốt, When giáo viên sửa điểm danh của kỳ đó, Then hệ thống cảnh báo và ghi nhận điều chỉnh sang kỳ sau

**R5. Báo cáo học phí gửi qua Zalo — hai lớp**

Quyết định: nội dung tóm tắt nằm **ngay trong tin nhắn Zalo**, chi tiết nằm sau link. Không chọn một trong hai.

**Đơn vị của báo cáo là người liên hệ, không phải học sinh.** Một tin nhắn, một link, một tổng số tiền — bất kể phụ huynh có bao nhiêu con và bao nhiêu lớp.

*Lớp 1 — tin nhắn Zalo (phụ huynh không cần bấm gì):* với mỗi con, một dòng gồm tên con, số buổi học, số buổi vắng, thành tiền. Cuối cùng là nợ cũ và **tổng phải đóng của cả gia đình**.

*Lớp 2 — link chi tiết (cho ai muốn kiểm chứng):* tách theo từng con, từng lớp, liệt kê từng buổi có mặt/vắng kèm ngày, công thức tính, mã QR chuyển khoản cho tổng số tiền.

Lý do không bỏ hẳn link: tin nhắn là dữ liệu **tĩnh và không đo được**. Nếu giáo viên sửa điểm danh sau khi gửi, tin nhắn cũ vĩnh viễn sai. Link tự cập nhật, đo được tỉ lệ mở, và là chỗ duy nhất có thể gắn thanh toán ở P1. Bỏ link là tự đóng đường nâng cấp.

- Token gắn với **người liên hệ + kỳ**, ngẫu nhiên không đoán được, không yêu cầu đăng nhập, không index bởi search engine
- Link hết hiệu lực sau khi kỳ được thanh toán xong hoặc sau 90 ngày

*Acceptance criteria:*
- [ ] Given phụ huynh có 2 con ở 2 lớp khác nhau, When chốt sổ và gửi, Then nhận đúng **một** tin nhắn với một tổng số tiền
- [ ] Given phụ huynh có 2 con, When mở link, Then thấy phần của từng con tách riêng và tổng cộng ở cuối
- [ ] Given phụ huynh nhận tin nhắn, When đọc mà không bấm link, Then vẫn biết đủ số tiền phải đóng và số buổi từng con đã học
- [ ] Given giáo viên sửa điểm danh sau khi gửi, When phụ huynh mở link cũ, Then thấy số liệu đã cập nhật
- [ ] Given token sai hoặc hết hiệu lực, When truy cập, Then hiện trang lỗi trung tính, không lộ thông tin học sinh nào
- [ ] Given phụ huynh mở link trên điện thoại, When trang load, Then thấy đủ thông tin trong một màn hình không cần cuộn ngang

**R6. Gửi hàng loạt**
- Sau khi chốt sổ, sinh nội dung riêng cho từng phụ huynh và gửi bằng một thao tác
- Cơ chế gửi Zalo — xem Open Questions Q1 (vẫn blocking)
- Ràng buộc thiết kế: nội dung tóm tắt phải vừa giới hạn độ dài của template ZNS. Nếu ZNS không đáp ứng, đây là lý do buộc phải đổi kênh gửi chứ không phải đổi nội dung.

**R7. Bảng trạng thái thu tiền — hai chế độ xem**

Công nợ **ghi nhận theo học sinh** (để biết mỗi lớp thu được bao nhiêu), nhưng **thu tiền theo người liên hệ** (vì phụ huynh trả một khoản cho nhiều con). Hai chế độ xem trên cùng một dữ liệu:

- *Xem theo lớp:* danh sách học sinh, trạng thái chưa đóng / đã đóng / đóng thiếu. Dùng khi giáo viên đang đứng lớp.
- *Xem theo người liên hệ:* mỗi dòng là một phụ huynh, gộp tất cả các con, tổng phải thu và đã thu. Dùng khi giáo viên đi thu tiền và nhắc nợ. **Đây là chế độ mặc định.**

- Giáo viên đánh dấu đã thu ở mức người liên hệ; hệ thống tự phân bổ xuống từng con
- Tổng: đã thu / còn phải thu

*Acceptance criteria:*
- [ ] Given 150 học sinh, When giáo viên mở bảng, Then lọc được nhanh nhóm chưa đóng
- [ ] Given giáo viên đánh dấu đã thu, When quay lại sau, Then trạng thái được giữ nguyên và trừ vào công nợ
- [ ] Given phụ huynh có 2 con và đã trả đủ, When xem theo lớp, Then cả 2 con đều hiện trạng thái đã đóng
- [ ] Given phụ huynh có 2 con và trả thiếu, When xem theo lớp, Then thấy rõ phần thiếu được phân bổ vào con nào
- [ ] Given giáo viên nhắc nợ, When phụ huynh có 2 con cùng nợ, Then chỉ gửi một lời nhắc duy nhất

### P1 — Should have (sau khi V1 chạy đúng)

- **Bảng "tiền đang thất thoát"**: nợ cũ chưa thu, buổi đã dạy chưa tính, học sinh có mặt nhưng chưa có trong danh sách thu. Đây là tính năng đồng thời là luận điểm bán hàng mạnh nhất — nó quy nỗi đau ra tiền.
- Mã QR chuyển khoản có nội dung đối soát tự sinh, khớp giao dịch bán tự động
- Nhắc nợ tự động sau X ngày
- Xử lý nghỉ có phép và học bù sang lớp khác
- Phân quyền trợ giảng: điểm danh được, không xem được tiền
- Thêm liên hệ phụ (bố và mẹ cùng nhận thông báo). Khi làm, quan hệ chuyển từ 1:n sang n:n và phải bổ sung khái niệm liên hệ chính để tránh đếm nợ hai lần.

### P2 — Future considerations (thiết kế để không chặn đường)

- Xác nhận đã đọc thông báo *(painpoint khởi đầu của dự án — xếp P2 là có chủ ý: nó chỉ có giá trị sau khi thông báo học phí đã có trạng thái)*
- Xuất hồ sơ minh bạch phục vụ yêu cầu công khai theo TT29/2024 và TT19/2026
- Xếp lịch nhiều ca, cảnh báo trùng giờ/trùng phòng
- Thu hộ qua cổng thanh toán, ăn phí giao dịch
- Điểm số, nhận xét, giao bài

**Ràng buộc kiến trúc cần giữ từ V1:** mô hình dữ liệu phải tách rời `buổi học` — `sự có mặt` — `khoản phải thu` — `khoản đã thu`. Nếu gộp học phí vào bản ghi học sinh, toàn bộ P1 và P2 sẽ phải viết lại.

## 7. Success Metrics

### Leading (đo trong 2–6 tuần)

| Chỉ số | Ngưỡng đạt | Ngưỡng xuất sắc |
|---|---|---|
| Tỉ lệ buổi được điểm danh trong 24h | 90% | 97% |
| Thời gian chốt sổ một lớp | < 10 phút | < 4 phút |
| Tỉ lệ giáo viên hoàn tất chốt sổ trọn vẹn 1 kỳ | 60% | 80% |
| Tỉ lệ phụ huynh mở link | 50% | 75% |
| Số dòng giáo viên phải sửa tay khi review | < 5% số học sinh | < 1% |

Chỉ số cuối cùng là chỉ báo niềm tin. Nếu giáo viên phải sửa tay nhiều, họ sẽ ngừng tin con số và quay lại Excel — kể cả khi hệ thống vẫn chạy.

### Lagging (đo trong 2–6 tháng)

| Chỉ số | Ngưỡng đạt |
|---|---|
| Retention tháng thứ 3 | ≥ 50% |
| Chuyển đổi sang trả phí sau 2 chu kỳ | ≥ 30% |
| Giảm tỉ lệ nợ đọng cuối kỳ so với trước khi dùng | Giảm ≥ 30% |
| Số học sinh bị sót khỏi danh sách thu | 0 |

Chu kỳ đánh giá phải tính theo **tháng**, không theo tuần: sản phẩm này chỉ tạo giá trị đầy đủ một lần mỗi chu kỳ chốt sổ. Một giáo viên cần tối thiểu 2 tháng để hình thành thói quen.

## 8. Open Questions

**Blocking — phải trả lời trước khi build**

| # | Câu hỏi | Ai trả lời |
|---|---|---|
| Q1 | Gửi tin nhắn hàng loạt qua Zalo bằng cơ chế nào? Zalo OA/ZNS có yêu cầu duyệt template, có chi phí theo tin, và có ràng buộc ngành nghề. Nếu không dùng được, phương án dự phòng là gì (sinh sẵn nội dung để giáo viên tự gửi, hay SMS)? Tôi chưa nắm chắc điều kiện ZNS hiện hành — cần kiểm chứng trực tiếp. | Engineering |
| Q2 | **Đã thu hẹp, chưa đóng.** Phạm vi dữ liệu đã giới hạn ở tên + ngày nhập học + lớp (xem R1). Việc này giảm rủi ro nhưng **không miễn trừ nghĩa vụ**: tên học sinh vẫn là dữ liệu cá nhân của trẻ em. Nghị định 13/2023 có quy định riêng về xử lý dữ liệu trẻ em, theo tôi nhớ là yêu cầu đồng ý của cả cha mẹ/người giám hộ và của chính trẻ từ đủ 7 tuổi trở lên — **tôi không chắc chắn về mốc tuổi và phạm vi áp dụng, phải đọc lại văn bản gốc.** Hai việc còn phải chốt: (a) xác định bạn là bên kiểm soát hay bên xử lý dữ liệu — nếu để giáo viên là bên kiểm soát và bạn là bên xử lý, phần lớn nghĩa vụ lấy đồng ý chuyển sang giáo viên, đổi lại bạn phải cung cấp công cụ để họ làm việc đó; (b) cơ chế thu thập đồng ý ở thời điểm nhập học. | Legal |
| Q3 | Thông tư 19/2026 (ban hành 31/3/2026) sửa đổi những gì trong TT29/2024? Nếu quy định siết chặt thêm, thị trường có thể co lại — hoặc ngược lại, yêu cầu công khai minh bạch sẽ trở thành wedge bán hàng. Chưa đọc được toàn văn. | Stakeholder |
| Q4 | Mô hình học phí "trả sau theo buổi thực học" chiếm bao nhiêu phần trăm thị trường thực tế? Nếu dưới 30%, quyết định ở Mục 4 sai. | Nghiên cứu thị trường |

**Non-blocking — giải quyết trong lúc build**

| # | Câu hỏi | Ai trả lời |
|---|---|---|
| Q5 | Khi giáo viên sửa điểm danh sau khi đã gửi thông báo cho phụ huynh: gửi lại thông báo mới hay chuyển chênh lệch sang kỳ sau? | Product |
| Q6 | ~~Link phụ huynh có nên hết hạn không?~~ **Đã chốt:** tóm tắt gửi thẳng trong tin nhắn Zalo, chi tiết đặt sau link có token, hết hiệu lực sau khi thanh toán xong hoặc sau 90 ngày. Xem R5. | Đã đóng |
| Q7 | Giáo viên có cần xem lịch sử học phí các kỳ trước ngay từ V1 không? | Nghiên cứu người dùng |
| Q8 | Phụ huynh có nhiều con trả thiếu tổng số tiền — phân bổ thế nào? Đề xuất mặc định: trả nợ cũ trước, rồi phát sinh kỳ này; trong cùng mức thì chia theo thứ tự lớp bắt đầu sớm hơn. Giáo viên phải override được. Cần kiểm chứng xem giáo viên thực tế xử lý ra sao — nhiều người có thể coi đây là "nợ của gia đình" chứ không phân bổ theo từng con. | Product + Nghiên cứu người dùng |
| Q9 | Giảm giá anh chị em: có bao nhiêu phần trăm chủ lớp đang áp dụng? Nếu phổ biến, áp lực đưa vào V1 sẽ rất lớn dù Mục 4 đã loại. Cấu trúc dữ liệu đã sẵn sàng (đơn giá ở mức ghi danh), nhưng quyết định có build hay không cần dữ liệu. | Nghiên cứu thị trường |

## 9. Timeline Considerations

**Không có deadline cứng.** Nhưng có một ràng buộc nhịp độ quan trọng: sản phẩm chỉ chứng minh được giá trị qua **chu kỳ chốt sổ cuối tháng**. Nghĩa là mỗi vòng học hỏi mất tối thiểu 30 ngày. Điều này có hai hệ quả:

1. Phải kịp có bản dùng được trước một mốc chốt sổ cụ thể, nếu không mất trọn một tháng
2. Không thể "ship nhanh học nhanh" theo tuần — phải thiết kế để thu được tín hiệu sớm hơn từ hành vi điểm danh hàng ngày

**Phasing đề xuất**

| Giai đoạn | Nội dung | Điều kiện chuyển tiếp |
|---|---|---|
| 0 | 10 cuộc phỏng vấn chủ lớp + thu thập file Excel thật | Ít nhất 6/10 kể được một khoản tiền cụ thể bị mất |
| 0.5 | Bán trước: chào gói trả phí, xin cam kết | Ít nhất 3 người đồng ý trả tiền trước khi có sản phẩm |
| 1 | Build R1–R7 | Chạy thật với 3 giáo viên trong 1 chu kỳ tháng |
| 2 | P1 sau khi G4 đạt ≥90% | — |

**Không được bỏ qua Giai đoạn 0 và 0.5.** Toàn bộ spec này được viết trên giả thuyết. Nếu Giai đoạn 0 thất bại, spec này nên bị vứt đi chứ không nên sửa.

## 10. Parking lot

Ý tưởng tốt nhưng không thuộc phạm vi hiện tại: quản lý tuyển sinh và lead; marketing cho lớp học; kết nối phụ huynh tìm giáo viên; học liệu và ngân hàng đề; ứng dụng cho học sinh; báo cáo thuế và kế toán cho trung tâm.
