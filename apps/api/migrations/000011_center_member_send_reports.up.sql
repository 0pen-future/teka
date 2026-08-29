-- Quyền gửi báo cáo theo membership: thành viên (không phải owner) được owner
-- cấp cờ này sẽ đọc bảng kê/công nợ toàn trung tâm và chạy gửi báo cáo bằng
-- Zalo của chính mình. Cờ sống theo stint hiện tại — đóng/mở lại membership
-- phải reset về FALSE (app-enforced trong OpenMembership/CloseMembership).
ALTER TABLE center_members
    ADD COLUMN can_send_reports BOOLEAN NOT NULL DEFAULT FALSE;
