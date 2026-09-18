---
title: HTTPS cục bộ & tên miền .test
description: Caddy proxy tích hợp sẵn và Root CA tự tin cậy của Govard cung cấp HTTPS an toàn cho mỗi dự án local trên tên miền *.test mà không cần cấu hình.
---

# SSL và Tên miền (SSL and Domains)

Govard cung cấp kết nối HTTPS local cho các tên miền `.test` thông qua proxy Caddy dùng chung và certificate authority (CA) nội bộ của nó.

---

## Những việc Govard tự động xử lý

- Định tuyến DNS `.test` local thông qua `dnsmasq`.
- Cấp chứng chỉ SSL cho tất cả các domain của dự án.
- Xuất Root CA ra đường dẫn `~/.govard/ssl/root.crt`.
- Cài đặt vào trust-store của hệ thống (nỗ lực tối đa).
- Import vào NSS store của các trình duyệt khi có sẵn lệnh `certutil`.
- Tự động cập nhật trust store của container PHP khi chạy `govard env up` / `govard env restart` (nếu file Root CA đã được xuất).

---

## Cấu hình DNS cho tên miền `.test`

Govard chạy một dịch vụ `dnsmasq` tích hợp sẵn nhằm phân giải các domain `*.test` về môi trường local của bạn. Bạn cần cấu hình hệ điều hành để chuyển tiếp các truy vấn `.test` tới dịch vụ này.

### Linux — systemd-resolved (Khuyên dùng)

Hoạt động tốt trên Ubuntu, Debian, Arch, Fedora:

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' | sudo tee /etc/systemd/resolved.conf.d/govard-test.conf
[Resolve]
DNS=127.0.0.1
Domains=~test
EOF
sudo systemctl restart systemd-resolved
```

### Linux — resolvconf (Ubuntu/Debian cũ)

```bash
sudo apt-get install resolvconf
echo "nameserver 127.0.0.1" | sudo tee /etc/resolvconf/resolv.conf.d/tail
sudo resolvconf -u
```

### macOS

```bash
sudo mkdir -p /etc/resolver
echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/test
```

### Xác minh phân giải DNS (Verify DNS Resolution)

```bash
resolvectl query laravel.test
dig +short laravel.test
```

---

## Cài đặt tin cậy Root CA (Root CA Trust)

Lệnh `govard svc up` và `govard svc restart` mặc định sẽ tự động cấu hình tin cậy cho Govard Root CA.

```bash
govard svc up         # Tự động tin cậy CA
govard doctor trust   # Tin cậy thủ công (có thể chạy lại bất cứ lúc nào)
```

Bỏ qua tự động tin cậy khi cần thiết:

```bash
govard svc up --no-trust
```

**Những gì lệnh `doctor trust` thực hiện:**
1. Xuất Root CA từ Caddy ra thư mục `~/.govard/ssl/root.crt`.
2. Cài đặt vào trust store hệ thống (Linux/macOS).
3. Nỗ lực import vào các NSS store của Chromium/Firefox khi có sẵn `certutil`.

::: tip GỢI Ý
Trên Linux, hãy cài đặt gói `libnss3-tools` để có lệnh `certutil`, giúp Govard tự động import chứng chỉ vào trình duyệt của bạn:
```bash
sudo apt-get install libnss3-tools
```
:::

---

## Cấu hình tin cậy trên trình duyệt (Browser Trust)

Nếu trust store hệ thống đã cài đặt mà trình duyệt vẫn hiển thị cảnh báo bảo mật:

1. Tìm file **`~/.govard/ssl/root.crt`**.
2. Mở phần quản lý chứng chỉ của trình duyệt (ví dụ: `chrome://settings/certificates`).
3. Chọn tab **Authorities** → click **Import**.
4. Chọn file `root.crt` và đánh dấu tin cậy (trust) cho website.
5. Khởi động lại trình duyệt.

Sau khi hoàn tất, tất cả các domain `*.test` do Govard quản lý sẽ hiển thị biểu tượng "Khóa xanh" (Green Lock) an toàn.

---

## Quản lý tên miền (Domain Management)

### Các tên miền phụ (Extra Domains)

```bash
govard domain add brand-b.test
govard domain remove brand-b.test
govard domain list
```

Govard sẽ định tuyến các domain này qua cùng một proxy và luồng CA tương tự như domain chính của dự án.

### Truy cập RabbitMQ Management UI từ Host

Khi `stack.services.queue` của một dự án là `rabbitmq`, Caddy proxy dùng chung cũng route `http://<your-domain>:15672` tới RabbitMQ management UI của dự án đó theo hostname — cùng cách route theo Host-header như search engine trên `:9200`. Việc này tự động; không cần `linked_projects` hay cấu hình domain bổ sung. Caveat không-TLS tương tự như trên.

### Kết nối liên dự án từ Container PHP (Inter-Project Access)

Mặc định, các dự án Govard được cô lập. Để cho phép một dự án PHP local gọi một dự án khác qua Caddy proxy dùng chung, bạn phải khai báo rõ ràng mối quan hệ phụ thuộc trong file `.govard.yml` bằng trường `linked_projects`:

```yaml
linked_projects:
  - project-b
```

Khi một dự án được liên kết:
1. **Cô lập mặc định (Isolation by Default)**: Chỉ những dự án được liên kết rõ ràng mới được ghi nhận domain vào `/etc/hosts` của container.
2. **Khởi động lại có chọn lọc**: Khi `project-b` khởi động, Govard sẽ chỉ reload lại các dự án phụ thuộc vào nó (như `project-a`), hạn chế tối đa downtime.
3. **Phân giải tự động**: Khai báo tên dự án sẽ tự động ánh xạ domain chính và toàn bộ extra domain của dự án đó.

Khi file `~/.govard/ssl/root.crt` tồn tại, Govard cũng sẽ mount Root CA đó vào container `php` và `php-debug` rồi cập nhật trust store của container khi chạy `govard env up` / `govard env restart`, giúp các lệnh kiểm tra TLS bên trong container hoạt động bình thường.

Danh sách alias này được cập nhật khi chạy `govard env up`. Nếu gặp lỗi kết nối sau khi liên kết, hãy chạy:

```bash
govard doctor trust
govard env restart
```

### Multi-Store Magento

Đối với cấu hình Magento multi-site:
- Sử dụng `store_domains` để tự động định tuyến hostname và thiết lập base URL theo phạm vi (scope).
- Sử dụng dạng Object cấu hình (`type: website` hoặc `type: store`) để tự động inject biến môi trường `MAGE_RUN_CODE` / `MAGE_RUN_TYPE`.
- Chỉ sử dụng `extra_domains` cho các hostname bổ sung **không** nằm trong `store_domains`.

```yaml
store_domains:
  brand-b.test:
    code: brand_b
    type: store
```

Bạn **không** cần cấu hình thủ công các luật `SetEnvIf` trong `.htaccess` nữa nếu sử dụng luồng `store_domains` chuẩn này.

---

## Nguyên lý định tuyến hoạt động (How Routing Works)

1. Lệnh `govard env up` render stack của dự án và đăng ký toàn bộ route tương ứng.
2. Các lệnh `govard env start` và `govard env restart` áp dụng lại các route + host local sau khi có thay đổi vòng đời.
3. Govard inject các domain dự án đã liên kết vào container PHP để hỗ trợ gọi HTTP liên container.
4. Caddy đóng vai trò điểm cuối (terminate) HTTPS.
5. Caddy chuyển tiếp traffic tới web container của dự án tương ứng.
6. Govard quản lý CA local và chứng chỉ root xuất ra ngoài.

---

## Khắc phục sự cố (Troubleshooting)

### Trình duyệt báo lỗi "Kết nối của bạn không phải là riêng tư" (Connection not private)

Kiểm tra theo thứ tự:

```bash
govard svc up               # Đảm bảo các dịch vụ toàn cục đang chạy
govard doctor trust         # Cài đặt lại Root CA
ls ~/.govard/ssl/root.crt   # Xác minh file CA có tồn tại
```

Nếu vẫn lỗi:
- Import thủ công file `~/.govard/ssl/root.crt` vào trình duyệt của bạn.
- Cài đặt `certutil` (Linux: `sudo apt-get install libnss3-tools`).
- Khởi động lại trình duyệt.

### Tên miền không phân giải được (Domain does not resolve)

Kiểm tra:
- Cấu hình resolver `.test` (xem [Cấu hình DNS](#cau-hinh-dns-cho-ten-mien-test)).
- Đảm bảo `govard svc up` đang chạy (bao gồm dịch vụ dnsmasq).

```bash
govard svc up
resolvectl query myproject.test
```

### Chứng chỉ SSL không được tạo

```bash
govard env up
govard env logs
docker ps | grep caddy
```

### HTTPS không hoạt động sau khi restart container

```bash
govard env restart    # Áp dụng lại proxy routes + các ánh xạ domain local
```

### Lệnh `curl` trong container `php` hoặc `php-debug` báo lỗi `unable to get local issuer certificate`

```bash
govard doctor trust
govard env restart
```

Lệnh này xuất lại Govard Root CA, sau đó khởi tạo lại container PHP với CA được mount để `curl`, Composer và các client TLS khác tin cậy các endpoint `*.test`.

---

[Remote & Đồng bộ](/vi/workflows/remotes-and-sync) | [Ứng dụng Desktop](/vi/workflows/desktop-app)