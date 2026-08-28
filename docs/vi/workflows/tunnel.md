---
title: Tunnel công khai — Chia sẻ dự án Local
description: Expose dự án Govard local ra public qua Cloudflare Tunnel với tự động rewrite base-URL và Caddy alias routing.
---

# Tunnel

Chia sẻ dự án Govard local ra public URL mà không cần deploy — phù hợp demo cho khách hàng, test webhook hoặc kiểm tra trên thiết bị di động.

---

## Yêu cầu trước

Cài đặt [`cloudflared`](https://github.com/cloudflare/cloudflared/releases) trên host:

```bash
# Linux (.deb)
curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o /tmp/cloudflared.deb
sudo dpkg -i /tmp/cloudflared.deb

# macOS (Homebrew)
brew install cloudflared
```

Kiểm tra:

```bash
cloudflared --version
```

Govard không bundle `cloudflared` — bạn tự quản lý cài đặt/nâng cấp.

---

## Bắt đầu nhanh

```bash
# Khởi tunnel cho project hiện tại (tự dò URL từ output cloudflared)
govard tunnel start

# Hoặc truyền URL đã tạo sẵn
govard tunnel start https://my-demo.trycloudflare.com

# Dry-run: chỉ in kế hoạch, không chạy
govard tunnel start --plan
```

Trong khi tunnel chạy:

- Govard đăng ký domain tunnel như **alias Caddy** cho project (Caddy route nó như `*.test`, không cần đổi DNS).
- Giữ nguyên `Host` header gốc — Magento/Laravel không thấy host lạ nên không redirect.
- Base URL của framework được **rewrite** sang URL tunnel qua `BaseURLManager` (Magento 2 ghi `web/unsecure/base_url` …). URL gốc được khôi phục khi `tunnel stop` hoặc `Ctrl+C`.

Dừng tunnel:

```bash
govard tunnel stop
# hoặc Ctrl+C tiến trình start
```

Kiểm tra trạng thái:

```bash
govard tunnel status
```

---

## Các cờ

| Cờ | Tác dụng |
| :--- | :--- |
| `[url]` | URL tunnel tùy chọn. Nếu không truyền, Govard tự parse từ stdout `cloudflared`. |
| `--provider <name>` | Provider tunnel. Hiện chỉ có `cloudflare`. |
| `--no-tls-verify` | Bỏ qua xác thực TLS cho endpoint tunnel. |
| `--plan` | In kế hoạch khởi động rồi thoát — không chạy process. |

---

## Cách hoạt động

1. `govard tunnel start` xác định target URL (arg, flag `--url`, hoặc tự dò).
2. Provider `BuildStartPlan` tạo route alias Caddy và kế hoạch patch base-URL.
3. Process tunnel (`cloudflared tunnel --url http://localhost:80` …) được spawn.
4. Khi thoát (chủ động hoặc bị ngắt), Govard xóa alias Caddy và revert base URL.

> **Phạm vi:** `tunnel` chỉ rewrite domain chính. Multi-store `store_domains` vẫn giữ host `.test` — chúng vẫn resolve local qua `dnsmasq`.

---

## Khắc phục sự cố

| Triệu chứng | Cách sửa |
| :--- | :--- |
| `cloudflared: command not found` | Cài `cloudflared` trước (xem Yêu cầu). |
| Tunnel URL báo 404 của Govard | Chạy `govard env up` trước — project phải đang chạy để Caddy có backend. |
| Base URL không khôi phục sau Ctrl+C | Chạy `govard tunnel stop` hoặc `govard config auto` (Magento 2) để áp lại URL local. |
| `tunnel status` báo không có tunnel | Không có tunnel đang chạy — `tunnel start` phải chạy ở terminal khác. |

---

[SSL và Tên miền](/vi/workflows/ssl-and-domains) | [Lệnh CLI](/vi/reference/cli-commands#govard-tunnel) | [Kiến trúc](/vi/developer/architecture)
