/**
 * The prototype's compact money form — grouped digits with a trailing "đ"
 * (`-150.000đ`). Deliberately not the shared `formatMoney` ("₫" with a
 * space): the teaching screens replicate the prototype's tables and CSV
 * labels, which use this tighter form.
 */
export function vnd(amountDong: number): string {
  return `${amountDong.toLocaleString("vi-VN")}đ`;
}
