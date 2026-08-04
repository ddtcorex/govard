---
title: Bắt đầu với Govard
description: Con đường ngắn nhất từ cài đặt Govard đến chạy dự án local đầu tiên — khởi tạo, nhận diện framework, và khởi động môi trường.
---

# Hướng dẫn nhanh

Hướng dẫn này sẽ dẫn dắt bạn qua con đường ngắn nhất từ lúc cài đặt đến khi có một dự án Govard hoạt động ở local.

---

## 1. Khởi tạo một dự án

Di chuyển tới thư mục gốc của dự án và chạy:

```bash
cd /path/to/your/project
govard init
```

Govard sẽ kiểm tra file `composer.json` hoặc `package.json`, nhận diện framework và tạo cấu hình `.govard.yml`.

### Các framework được tự động nhận diện

| Framework | Cách nhận diện |
| :--- | :--- |
| Magento 2 | `composer.json` có `magento/magento2-base` |
| Mage-OS | `composer.json` có `mage-os/product-community-edition` hoặc `mage-os/project-community-edition` |
| Magento 1 / OpenMage | Pattern cấu trúc file trong `composer.json` |
| Laravel | Có file `artisan` + `composer.json` |
| Next.js | `package.json` có dependency `next` |
| Emdash | Chứa các dấu hiệu nhận biết của dự án Emdash |
| Drupal | `composer.json` có `drupal/core` |
| Symfony | Có `symfony/framework-bundle` |
| Shopware | Có `shopware/core` |
| CakePHP | Có `cakephp/cakephp` |
| PrestaShop | `config/defines.inc.php` |
| Django | `manage.py` |
| WordPress | Có `wp-config.php` hoặc `wp-login.php` |
| Custom (Tùy chỉnh) | Chọn stack qua prompt tương tác (`govard init --framework custom`) |

### Ép buộc nhận diện một Framework cụ thể

```bash
govard init --framework magento2
govard init --framework custom
```

### Di chuyển từ các công cụ khác

Nếu bạn đang chuyển từ Warden hoặc DDEV, Govard có thể tự động phát hiện cấu hình và sao chép volume database local của bạn mà không làm mất dữ liệu:

```bash
govard init --migrate-from warden
govard init --migrate-from ddev
```

---

## 2. Khởi động môi trường

```bash
govard env up
```

Lệnh này sẽ render một file compose cho từng dự án tại thư mục `~/.govard/compose/` và khởi động stack tương ứng.

### Các lệnh phổ biến

```bash
govard up --quickstart           # Alias của: govard env up
govard env up --pull             # Tải (pull) image mới nhất trước khi khởi động
govard env up --fallback-local-build  # Tự động build image local nếu pull thất bại
```

### Quy trình khởi động

1. Nhận diện ngữ cảnh framework.
2. Xác thực cấu hình, Docker, các port và điều kiện tiên quyết.
3. Render file compose vào thư mục `~/.govard/compose/`.
4. Khởi động các container ở chế độ chạy ngầm (detached mode).
5. Xác minh proxy và host routing.

### Các phím tắt gốc (Root Shortcuts)

| Phím tắt | Lệnh tương đương |
| :--- | :--- |
| `govard up` | `govard env up` |
| `govard down` | `govard env down` |
| `govard restart` | `govard env restart` |
| `govard ps` | `govard env ps` |
| `govard logs` | `govard env logs` |

---

## 3. Cấu hình ứng dụng

### Dự án Magento 2

Tự động cấu hình kết nối container vào file `app/etc/env.php`:

```bash
govard config auto
```

### Xem cấu hình hiện tại

```bash
govard config get php_version
govard config get stack.services.db
```

---

## 4. Đi vào Workspace (Shell)

```bash
govard shell
```

- **PHP frameworks** (Magento, Laravel, v.v.): Mở terminal trong container `php` tại `/var/www/html`
- **Node-first frameworks** (Next.js, Emdash): Mở terminal trong container `web` tại `/app`

---

## 5. Truy cập Ứng dụng và Dịch vụ toàn cục

Govard định tuyến các domain dự án qua Caddy proxy dùng chung và cung cấp các dịch vụ tích hợp sẵn cho việc phát triển:

### Các URL của dự án

| Mục tiêu | URL | Lệnh |
| :--- | :--- | :--- |
| App URL | `https://<project>.test` | Mở trực tiếp trên trình duyệt |
| Admin panel | `https://<project>.test/admin` | `govard open admin` |
| Kiểm thử Mail (Mailpit) | `https://mail.govard.test` | `govard open mail` |
| Quản lý Database (PHPMyAdmin) | `https://pma.govard.test` | `govard open db` |
| Quản lý Docker (Portainer) | `https://portainer.govard.test` | `govard open portainer` |

### Thông tin đăng nhập các Dịch vụ toàn cục

| Dịch vụ | URL | Credentials |
| :--- | :--- | :--- |
| Portainer | `https://portainer.govard.test` | `admin` / `AdminGovard123$` |
| PHPMyAdmin | `https://pma.govard.test` | Sử dụng thông tin DB của dự án |
| Mailpit | `https://mail.govard.test` | Không yêu cầu đăng nhập |

### Lệnh nhanh (Quick Commands)

```bash
# Mở dịch vụ trên trình duyệt
govard open mail     # Mailpit
govard open db       # PHPMyAdmin
govard open admin    # Project admin panel

# Quản lý dịch vụ toàn cục (global services)
govard svc up        # Khởi động các dịch vụ toàn cục
govard svc down      # Dừng các dịch vụ toàn cục
govard svc ps        # Xem trạng thái dịch vụ
```

---

## 🔁 Quy trình làm việc hàng ngày

```bash
# Bắt đầu làm việc
govard up

# Theo dõi logs
govard logs php -f

# Bật Xdebug
govard debug on

# Vào shell container
govard shell

# Dừng làm việc
govard down
```

---

## 🌐 Bootstrap dự án từ Remote (Clone)

Để clone một môi trường có sẵn từ một remote server:

```bash
govard bootstrap --clone -e staging --no-pii --no-noise
```

Đối với việc cài đặt mới hoàn toàn một framework:

```bash
govard bootstrap --framework magento2 --fresh --framework-version 2.4.9
govard env up
govard open admin
```

---

## 🩺 Khắc phục sự cố bước đầu

```bash
govard doctor
govard doctor trust   # Sửa lỗi cảnh báo SSL trên trình duyệt
```

Nếu trình duyệt của bạn hiển thị cảnh báo HTTPS trust sau khi setup, chạy `govard doctor trust` để import lại Root CA.

---

## 📋 Đọc thêm gì tiếp theo

| Chủ đề | Đường dẫn |
| :--- | :--- |
| Danh sách các lệnh CLI | [Lệnh CLI](/vi/reference/cli-commands) |
| Các tùy chọn cấu hình | [Cấu hình](/vi/reference/configuration) |
| Ghi chú về từng framework | [Frameworks](/vi/reference/frameworks) |
| Cấu hình SSL và DNS | [SSL và Tên miền](/vi/workflows/ssl-and-domains) |
| Môi trường Remote | [Remote & Đồng bộ](/vi/workflows/remotes-and-sync) |
| Ứng dụng Desktop | [Ứng dụng Desktop](/vi/workflows/desktop-app) |

---

**[← Cài đặt](/vi/getting-started/installation)** | **[Lệnh CLI →](/vi/reference/cli-commands)**