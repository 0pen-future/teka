---
phase: 2
title: "Crawl blocking (robots, X-Robots-Tag, meta)"
status: pending
priority: P1
effort: "0.25d"
dependencies: []
---

# Phase 2: Crawl blocking

## Goal

Không trang nào của app bị index, đặc biệt các URL mang token (`/invite/*`, `/reset-password/*`, và link `/s/*` cũ giờ ra 404).
Hai lớp: `robots.txt` (crawler ngoan không crawl) và `X-Robots-Tag` + meta robots
(chặn index cả URL đã lộ, vì `Disallow` đơn thuần không làm được việc này).

## Files

- Create: `apps/web/public/robots.txt`
- Create: `apps/web/nginx-security-headers.conf` (snippet include)
- Modify: `apps/web/nginx.conf`
- Modify: `apps/web/Dockerfile` (`COPY nginx-security-headers.conf /etc/nginx/snippets/teka-security-headers.conf`)
- Modify: `apps/web/index.html` (meta robots)
- Modify: `docs/deployment.md` quanh dòng 67–69 ("baseline security headers"): thêm noindex/robots vào mô tả

## Design

Hiện mỗi `location` lặp 3 header bảo mật vì `add_header` không kế thừa. Thêm header
thứ tư vào 4 chỗ dễ sót, nên gom tất cả vào một snippet:

```nginx
# nginx-security-headers.conf
add_header X-Content-Type-Options "nosniff" always;
add_header X-Frame-Options "DENY" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header X-Robots-Tag "noindex, nofollow" always;
```

Mỗi location giữ `add_header Cache-Control ...` riêng và thêm `include /etc/nginx/snippets/teka-security-headers.conf;`.
Đặt snippet ngoài `conf.d/` để nginx không nạp nó như một server block.

Location mới:

```nginx
location = /robots.txt {
    default_type text/plain;
    add_header Cache-Control "no-cache" always;
    include /etc/nginx/snippets/teka-security-headers.conf;
    try_files $uri =404;
}
# Không có trang nào muốn được index hay trích dẫn; trả 404 thay vì SPA HTML.
location = /sitemap.xml { return 404; }
location = /llms.txt    { return 404; }
location = /llms-full.txt { return 404; }
```

`robots.txt`:

```
User-agent: *
Disallow: /
```

`index.html`: `<meta name="robots" content="noindex, nofollow" />`.

**Đánh đổi:** `Disallow: /` ngăn crawler đọc trang, nên nó cũng không thấy meta
robots. Đó là lý do header `X-Robots-Tag` ở tầng HTTP là lớp chính; meta chỉ là
lớp dự phòng khi HTML được phục vụ qua đường khác (vd. `vite preview`).

## Steps

1. Trong `apps/web/Dockerfile` (stage `nginx-unprivileged`), thêm `COPY` snippet vào `/etc/nginx/snippets/` ngay dưới dòng copy `nginx.conf`.
2. Viết snippet, sửa mọi `location` dùng `include`, thêm các location mới.
3. Thêm `robots.txt` và meta robots.
4. Build image và chạy thử container cục bộ trên cổng cố định (vd. 58090), dừng container sau khi kiểm tra.

## Verification

```bash
docker build --build-arg VITE_API_URL=/api/v1 -t teka-web-robots-check apps/web
docker run --rm -d --name teka-web-robots-check -p 58090:8080 teka-web-robots-check
curl -sI localhost:58090/robots.txt | grep -i 'content-type: text/plain'
for p in / /login /s/x /invite/x /reset-password/x /assets/missing.js; do curl -sI localhost:58090$p | grep -i 'x-robots-tag: noindex'; done
curl -s -o /dev/null -w '%{http_code}\n' localhost:58090/sitemap.xml   # 404
curl -s -o /dev/null -w '%{http_code}\n' localhost:58090/llms.txt      # 404
docker stop teka-web-robots-check
```

- Kiểm tra lại header bảo mật cũ vẫn có mặt trên `/`, `/assets/*`, `/manifest.webmanifest`.
- `nginx -t` trong container không lỗi.

## Rollback

Revert commit. Không ảnh hưởng dữ liệu. Traefik/Cloudflare không cần đổi, vì header đi xuyên qua.
