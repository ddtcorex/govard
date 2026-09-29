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

`tunnel start` ghi lại tiến trình mà nó đã khởi động — PID và argv đã dùng để khởi động — vào
`$GOVARD_HOME_DIR/tunnels/<project>.pid`, và `tunnel stop` gửi tín hiệu tới đúng tiến trình đó.
Lệnh không bao giờ dò máy tìm tiến trình theo tên, nên không có mẫu nào để nó vượt tay; thứ nó từ
chối là mọi PID có argv khác với argv mà nó đã ghi, từng token một kể từ đối số đầu tiên. Một
`cloudflared` do bạn tự chạy, một cái thuộc về project khác, và một cái thuộc về công cụ khác đều
không bị đụng tới — kể cả một unit `cloudflared tunnel` của công cụ khác, vốn chia sẻ cả binary lẫn
động từ `tunnel` và chỉ bị phân biệt bởi các đối số phía sau. Argv được kiểm tra trước mọi tín hiệu:
nếu PID đã ghi bị một chương trình không liên quan chiếm lại, hoặc không đọc được argv, `tunnel stop`
sẽ từ chối kèm lỗi và không gửi tín hiệu nào — hãy tự xoá bản ghi sau khi đã kiểm tra tunnel.

Một bản ghi mà govard không phân tích nổi — tệp bị cắt cụt hoặc bị sửa tay — khiến cả `start`, `stop`
và `status` đều hỏng kèm lỗi thay vì giả định là không có tunnel, vì một bản ghi không ai đọc hiểu
không giống việc không có bản ghi và giả định như vậy sẽ bỏ rơi một tunnel đang chạy. Hãy xoá tệp đó
sau khi đã tự kiểm tra tunnel.

Khi không có bản ghi, `tunnel stop` không làm gì, có in ra điều đó và thoát với mã 0, nên chạy hai
lần vẫn an toàn. Base URL của project được khôi phục trong mọi trường hợp, trừ khi tín hiệu bị
từ chối — lúc đó tunnel vẫn còn sống, nên base URL vẫn trỏ tới nó.

Kiểm tra trạng thái:

```bash
govard tunnel status
```

`status` đọc cùng bản ghi đó thay vì dò trên máy, nên nó báo `INACTIVE` mỗi khi Govard không có
tunnel nào của chính nó đang chạy — kể cả khi một chương trình khác vừa chiếm đúng PID đã ghi.

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
| `tunnel status` báo không có tunnel | Govard không có tunnel nào của chính nó đang chạy. `status` đọc PID đã ghi, nên nó cũng báo vậy khi tunnel đã chết và để lại bản ghi cũ (lệnh sẽ xoá bản ghi đó), hoặc khi một chương trình khác đã chiếm mất PID đó. |

---

[SSL và Tên miền](/vi/workflows/ssl-and-domains) | [Lệnh CLI](/vi/reference/cli-commands#govard-tunnel) | [Kiến trúc](/vi/developer/architecture)
