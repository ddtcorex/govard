---
title: FAQ & Khắc phục sự cố Govard
description: Giải đáp các câu hỏi thường gặp khi cài đặt, di chuyển sang, và khắc phục sự cố môi trường phát triển cục bộ Govard.
---

# FAQ & Khắc phục sự cố (FAQ & Troubleshooting)

Các câu hỏi, lỗi thường gặp và giải pháp xử lý cho Govard.

---

## Các lỗi khi Cài đặt

### Q: Script cài đặt bị lỗi phân quyền (permission error)

Thêm cờ `--local` để cài đặt không cần quyền `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --local
```

Việc này sẽ cài đặt binary vào `~/.local/bin` thay vì `/usr/local/bin`.

### Q: Tôi bị xung đột binary tại `/usr/bin` và `/usr/local/bin`

Do bạn đã cài đặt từ nhiều kênh khác nhau. Chọn một kênh và dọn dẹp kênh còn lại:

```bash
which govard           # Kiểm tra xem binary nào đang hoạt động
ls /usr/bin/govard /usr/local/bin/govard  # Xem cả hai đường dẫn
```

Xóa thủ công binary ở kênh không dùng. Không nên cài đặt chéo nhiều kênh trên cùng một máy.

### Q: Lệnh `govard self-update` bị lỗi phân quyền

Govard cần quyền ghi vào đường dẫn chứa binary đang chạy. Nếu cài đặt ở đường dẫn hệ thống:

```bash
sudo govard self-update
```

Hoặc cài đặt lại vào thư mục local của user bằng cờ `--local`.

---

## Các lỗi về Docker

### Q: Lệnh `govard env up` bị lỗi khi tải (pull) image

Nếu quá trình pull image thất bại, hãy thử chế độ fallback build local:

```bash
govard env up --fallback-local-build
```

Lệnh này sẽ tự động build các image do Govard quản lý trực tiếp tại local từ các blueprint tích hợp sẵn.

### Q: Lỗi xung đột port khi khởi động môi trường

```bash
govard doctor       # Kiểm tra xung đột port trên hệ thống
govard env ps       # Xem các container nào đang chạy
```

Đảm bảo không có dịch vụ nào khác đang chiếm dụng các port 80, 443 hoặc các port đã map của dự án.

### Q: Lệnh `govard env up` báo lỗi trùng định danh dự án "project identity collision"

Một dự án khác đang chạy đã đăng ký trùng `project_name` hoặc `domain`.

```bash
govard project list    # Xem toàn bộ dự án đang được theo dõi
```

Thay đổi giá trị `project_name` hoặc `domain` trong file `.govard.yml` sang một giá trị độc nhất khác.

---

## Lỗi SSL / HTTPS

### Q: Trình duyệt báo lỗi "Kết nối của bạn không phải là riêng tư" (Connection not private)

Chạy theo thứ tự sau:

```bash
govard svc up       # Đảm bảo các dịch vụ toàn cục đang chạy
govard doctor trust # Import lại Root CA
```

Nếu hệ thống import tự động thất bại, hãy import thủ công file `~/.govard/ssl/root.crt` vào trình duyệt của bạn.

### Q: Tính năng import tự động không hoạt động trên trình duyệt của tôi

Bạn cần cài đặt thêm gói `certutil`:

```bash
# Trên Ubuntu/Debian
sudo apt-get install libnss3-tools
# Sau đó chạy lại:
govard doctor trust
```

### Q: Kết nối HTTPS bị lỗi sau khi restart container dự án

```bash
govard env restart   # Áp dụng lại các proxy route và host entry
```

### Q: Lệnh `curl` trong container `php` hoặc `php-debug` lỗi `unable to get local issuer certificate`

Chạy theo thứ tự sau:

```bash
govard doctor trust
govard env restart
```

Lệnh này xuất Govard Root CA ra `~/.govard/ssl/root.crt`, sau đó khởi tạo lại container PHP với CA được mount và thiết lập tin cậy bên trong container.

---

## Lỗi DNS

### Q: Tên miền `myproject.test` không phân giải được

1. Đảm bảo dịch vụ systemd-resolved đã được cấu hình:
   ```bash
   cat /etc/systemd/resolved.conf.d/govard-test.conf
   ```
2. Xác minh dịch vụ dnsmasq đang chạy:
   ```bash
   govard svc up
   ```
3. Kiểm tra phân giải DNS:
   ```bash
   resolvectl query myproject.test
   dig +short myproject.test
   ```

### Q: DNS phân giải được nhưng không phản hồi (lỗi 502 Bad Gateway)

```bash
govard env ps        # Kiểm tra xem các container có thực sự đang chạy
govard env up        # Khởi động lại nếu cần thiết
```

---

## Lỗi Cấu hình (Configuration)

### Q: Các thay đổi cấu hình của tôi không có tác dụng

Govard render lại file compose khi chạy lệnh `env up`. Hãy khởi động lại môi trường:

```bash
govard env up
```

Nếu bạn thay đổi `stack.php_version` hoặc các cài đặt stack khác, các container cần phải được khởi tạo lại.

### Q: Lệnh `govard config set` không cập nhật đúng file cấu hình

Lệnh `govard config set` chỉ ghi trực tiếp vào `.govard.yml` (cấu hình cơ sở). Các file profile và local override là read-only dưới góc nhìn của CLI.

### Q: Làm sao để đổi phiên bản Composer và phiên bản nào chạy nhanh nhất?

Chỉnh sửa trường `stack.composer_version` trong file `.govard.yml`. Govard tối ưu hóa tốc độ cho các phiên bản:
- `1`
- `2`
- `2.2` (LTS)

Các phiên bản này được tích hợp sẵn trong image và chuyển đổi tức thì. Các phiên bản khác (ví dụ `2.7.2`) sẽ được tải về tự động trong lần đầu chạy `env up`.

### Q: Lệnh `doctor --fix` báo trạng thái "skipped" (bỏ qua) đối với một số lỗi tùy chọn

Đây là hành vi bình thường — việc bỏ qua các sửa đổi tùy chọn (optional fixes) được ghi nhận dưới dạng `INFO (Skipped)` thay vì `ERROR`. Môi trường của bạn hoàn toàn khỏe mạnh.

---

## Lỗi Remote / Đồng bộ (Remote / Sync)

### Q: Lệnh `govard remote test` bị lỗi xác thực "auth"

```bash
govard remote copy-id staging    # Sao chép SSH key của bạn lên remote
ssh-add ~/.ssh/id_rsa            # Đảm bảo key đã được nạp vào SSH agent
```

### Q: Quá trình đồng bộ chạy rất lâu hoặc bị timeout

- Sử dụng cờ `--no-compress` nếu CPU máy của bạn bị quá tải:
  ```bash
  govard sync -s staging --full --no-compress
  ```
- Kiểm tra các file bị loại trừ — cờ `--no-noise` có thể giảm đáng kể dung lượng truyền tải:
  ```bash
  govard sync -s staging --db --no-noise
  ```

### Q: Lỗi "permission denied" khi chạy rsync

Govard sẽ gợi ý cách sửa phân quyền đối với Magento 2 khi lỗi này xảy ra. Để sửa thủ công:

```bash
govard remote exec staging -- chmod -R 755 /var/www/app/var
```

### Q: Đường dẫn `~/` trong các cờ remote bị shell local tự động expand

Đóng dấu nháy đơn cho đường dẫn để ngăn shell local tự động expand nó:

```bash
govard remote add staging --host host.example.com --user deploy --path '~/public_html'
#                                                                          ^-- dấu nháy đơn
```

### Q: `govard deploy` nằm ở đâu, và diễn tập thế nào?

`govard deploy` phát hành một revision git lên remote qua SSH và rsync. Trước khi
đụng tới server thật, hãy diễn tập toàn bộ pipeline trên một container ngay tại máy:

```bash
govard deploy sandbox up --profile full --php 8.3
govard deploy --remote sandbox --yes
govard deploy sandbox down --purge
```

- Tham chiếu engine, chiến lược publish, rollback: [Triển khai](/vi/workflows/deployment)
- Cấu hình mẫu (Luma, Hyvä, nhiều theme, chế độ developer và production, docroot
  symlink và docroot thật): [Case study triển khai](/vi/workflows/deploy-case-studies)

`govard deploy check staging` trả lời câu "remote này đã sẵn sàng chưa" trước khi tạo
bất cứ thứ gì, và `govard deploy plan staging` in ra toàn bộ danh sách task mà không
cần kết nối.

---

## Lỗi Database

### Q: Lệnh `db import` bị lỗi "table doesn't exist" (bảng không tồn tại)

Sử dụng cờ `--drop` để reset sạch database trước khi import:

```bash
govard db import --file backup.sql --drop
```

### Q: Mật khẩu database bị sai sau khi chạy bootstrap

Chạy lệnh auto-config để tự động inject lại thông tin kết nối chuẩn:

```bash
govard config auto   # Magento 2: rebuild lại env.php với cấu hình DB của container
```

### Q: PHPMyAdmin không hiển thị database của dự án

Khởi động lại đầy đủ môi trường dự án để đăng ký lại:

```bash
govard env up
```

Sau đó truy cập `govard open db`.

---

## Lỗi Xdebug

### Q: Xdebug không kết nối được tới IDE của tôi

1. Kiểm tra trạng thái Xdebug: `govard debug status`.
2. Đảm bảo cookie `XDEBUG_SESSION` trùng khớp với giá trị `stack.xdebug_session` cấu hình trong `.govard.yml` (mặc định: `PHPSTORM`).
3. Kiểm tra xem IDE của bạn đã bật lắng nghe trên port 9003 hay chưa.

### Q: Xdebug làm chậm trang web của tôi ngay cả khi không debug

Cấu hình một tên session Xdebug cụ thể và chỉ kích hoạt nó thông qua cookie/extension trình duyệt. Xdebug chỉ định tuyến request sang container `php-debug` **khi và chỉ khi** phát hiện cookie session tương ứng.

---

## Lỗi Desktop

### Q: Ứng dụng Desktop bị crash khi khởi động trên Ubuntu 24.04

Đây là lỗi giới hạn namespace user của AppArmor. Trình cài đặt tự động xử lý việc này, nhưng bạn có thể cấu hình thủ công:

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```

### Q: Giao diện Desktop hiển thị dữ liệu giả (mock data) thay vì dự án thật

Bạn đang mở trực tiếp file HTML tĩnh (không có backend hoạt động). Hãy khởi chạy desktop đúng cách:

```bash
govard desktop
# hoặc ở chế độ dev:
DISPLAY=:1 govard desktop --dev
```

---

## Lỗi Cập nhật (Update)

### Q: Lệnh `govard self-update` bỏ qua kiểm tra dependencies trên CI

Đây là hành vi có chủ đích — self-update phát hiện môi trường không tương tác (non-interactive) và bỏ qua các bước kiểm tra hệ thống nặng nề để tránh CI bị timeout.

### Q: Sau khi chạy `self-update`, ứng dụng Desktop vẫn hiển thị phiên bản cũ

Binary của desktop cũng được cập nhật bởi lệnh `self-update`. Nếu phiên bản cũ vẫn tồn tại, hãy khởi động lại hoàn toàn ứng dụng Desktop.

---

## Các mẹo chung (General Tips)

### Kiểm tra sức khỏe hệ thống

```bash
govard doctor           # Chẩn đoán toàn diện hệ thống
govard doctor --json    # Xuất kết quả định dạng JSON
govard doctor --pack    # Đóng gói file chẩn đoán để gửi báo cáo lỗi
```

### Xem những gì đang chạy

```bash
govard status           # Tất cả các môi trường Govard đang chạy
govard env ps           # Các container của dự án hiện tại
govard project list     # Toàn bộ dự án đang được theo dõi
```

### Dọn dẹp các môi trường rác

```bash
govard env cleanup      # Xóa các file compose cũ
govard project list --orphans  # Tìm các dự án Docker mồ côi
govard project delete <name>   # Xóa hoàn toàn một dự án
```

### Reset một dự án không mất mã nguồn

```bash
govard env down -v      # Dừng container + xóa các volume dữ liệu (database)
govard env up           # Khởi động lại mới tinh
govard config auto      # Cập nhật lại cấu hình ứng dụng (Magento 2)
```

---

[Đóng góp](/vi/developer/contributing) | [Nhật ký thay đổi](/vi/more/changelog)