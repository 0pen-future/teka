const linkClassName = "text-mint-600 transition-colors hover:text-mint-700";

/**
 * The product pages (hướng dẫn, bảng giá, FAQ) don't exist yet, so their
 * entries render as plain text in the link column's style — swap each for
 * an anchor once its page ships. A dead `href="#"` would both lie to
 * assistive tech and scroll the page to the top.
 */
const placeholderClassName = "text-mint-600";

/**
 * Prototype footer: brand blurb, product/support link columns, and the
 * data-minimalism promise line. Only the public parent-statement layout
 * renders it — inside the app the chrome is the sidebar's job; the promise
 * line matters most to parents landing from a Zalo link with no other
 * context. `mt-auto` pushes it to the bottom of short pages inside the
 * layout's flex column.
 */
export function AppFooter() {
  return (
    <footer className="mt-auto w-full pt-[60px]">
      <div className="flex flex-wrap items-start gap-x-10 gap-y-6 border-t border-line-200 pt-[26px]">
        <div className="min-w-[220px] flex-[1.4]">
          <p className="font-display text-[20px] font-extrabold text-ink-900">Teka</p>
          <p className="mt-[6px] max-w-[300px] text-[13.5px] leading-[1.6] text-ink-500">
            Điểm danh 1 chạm, học phí tính đúng từng buổi. Chốt sổ cả lớp trong 10 phút, không sót
            một học sinh nào.
          </p>
        </div>
        <div className="min-w-[150px] flex-1">
          <p className="text-[12px] font-extrabold tracking-[0.5px] text-ink-400 uppercase">
            Sản phẩm
          </p>
          <div className="mt-[10px] flex flex-col gap-2 text-[13.5px] font-bold">
            <span className={placeholderClassName}>Hướng dẫn sử dụng</span>
            <span className={placeholderClassName}>Bảng giá</span>
            <span className={placeholderClassName}>Câu hỏi thường gặp</span>
          </div>
        </div>
        <div className="min-w-[170px] flex-1">
          <p className="text-[12px] font-extrabold tracking-[0.5px] text-ink-400 uppercase">
            Hỗ trợ
          </p>
          <div className="mt-[10px] flex flex-col gap-2 text-[13.5px] font-bold">
            <a
              href="https://zalo.me/0900000000"
              target="_blank"
              rel="noreferrer"
              className={linkClassName}
            >
              Zalo: 0900 000 000
            </a>
            <a href="mailto:hotro@teka.vn" className={linkClassName}>
              hotro@teka.vn
            </a>
          </div>
        </div>
      </div>
      <div className="mt-6 flex flex-wrap items-center gap-[14px] border-t border-line-100 py-[14px] text-[12.5px] text-ink-400">
        <span>© 2026 Teka. Dữ liệu của bạn thuộc về bạn.</span>
        <span className="sm:ml-auto">
          Chỉ lưu tên, ngày nhập học và lớp của học sinh — không gì thêm.
        </span>
      </div>
    </footer>
  );
}
