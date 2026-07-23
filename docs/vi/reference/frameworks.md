---
title: Framework được hỗ trợ — Magento, Mage-OS, Laravel, Symfony, WordPress
description: Govard tự động nhận diện Magento 1/2, Mage-OS, Laravel, Symfony, Drupal, Shopware, CakePHP, PrestaShop, WordPress, Next.js và áp dụng cấu hình mặc định theo từng framework.
---

# Frameworks

Govard tự động nhận diện các framework được hỗ trợ và áp dụng các cấu hình runtime mặc định cùng với các ghi đè phù hợp với từng phiên bản.

---

## Bảng hỗ trợ (Support Matrix)

| Framework | Tự động nhận diện | Profile theo phiên bản | Web Root mặc định |
| :--- | :---: | :---: | :--- |
| Magento 2 | ✅ | ✅ | `/pub` |
| Mage-OS | ✅ | cấu hình mặc định | `/pub` |
| Magento 1 / OpenMage | ✅ | cấu hình mặc định | thư mục gốc dự án |
| Laravel | ✅ | ✅ | `/public` |
| Next.js | ✅ | cấu hình mặc định | thư mục gốc dự án |
| Emdash | ✅ | cấu hình mặc định | thư mục gốc dự án |
| Drupal | ✅ | ✅ | `/web` |
| Symfony | ✅ | ✅ | `/public` |
| Shopware | ✅ | cấu hình mặc định | `/public` |
| CakePHP | ✅ | cấu hình mặc định | `/webroot` |
| PrestaShop | ✅ | cấu hình mặc định | thư mục gốc dự án |
| WordPress | ✅ | ✅ | `/` |
| Django | ✅ | cấu hình mặc định | thư mục gốc dự án |
| Tùy chỉnh (Custom) | thủ công | thủ công | thư mục gốc dự án |

---

## Cấu hình mặc định (Runtime Defaults)

| Framework | PHP | Node | Python | DB | Cache | Search | Queue |
| :--- | :---: | :---: | :---: | :--- | :--- | :--- | :--- |
| Magento 2 | 8.4 | 24 | — | mariadb 11.4 | valkey 8.0.0 | opensearch 2.19.0 | none |
| Mage-OS | 8.4 | 24 | — | mariadb 11.8 | redis 7.4 | opensearch 3.0 | none |
| Magento 1 / OpenMage | 8.1 | — | — | mariadb 10.11 | none | none | none |
| Laravel | 8.4 | — | — | mariadb 11.4 | none | none | none |
| Next.js | — | 24 | — | none | none | none | none |
| Emdash | — | 22 | — | none | none | none | none |
| Drupal | 8.4 | — | — | mariadb 11.4 | none | none | none |
| Symfony | 8.4 | — | — | mariadb 11.4 | none | none | none |
| Shopware | 8.4 | — | — | mariadb 11.4 | none | none | none |
| CakePHP | 8.4 | — | — | mariadb 11.4 | none | none | none |
| PrestaShop | 8.1 | — | — | mariadb 10.11 | none | none | none |
| WordPress | 8.3 | — | — | mariadb 11.4 | none | none | none |
| Django | — | — | 3.12 | postgres 16 | none | none | none |
| Tùy chỉnh (Custom) | 8.4 | — | — | mariadb 11.4 | none | none | none |

Ký hiệu `—` nghĩa là Govard không ép buộc giá trị mặc định cho thành phần stack đó.

---

## Ghi đè theo phiên bản (Version-Aware Overrides)

| Framework | Phiên bản | Ghi đè PHP | Khác |
| :--- | :--- | :--- | :--- |
| Laravel | 10 | 8.2 | |
| Laravel | 11 | 8.3 | |
| Laravel | 12 | 8.4 | |
| Symfony | 6 | 8.2 | |
| Symfony | 7 | 8.3 | |
| Drupal | 10 | 8.3 | |
| Drupal | 11 | 8.4 | |
| WordPress | 6 | 8.3 | |
| Magento 2 | 2.4.9+ | 8.4 | MariaDB 11.4, Redis 7.2, OpenSearch 3.0.0, RabbitMQ 4.1.0 |
| Magento 2 | 2.4.8 | 8.4 | MariaDB 11.4, Redis 7.2, OpenSearch 2.19.0 hoặc 3.0.0 |
| Magento 2 | 2.4.7 | 8.3 | MariaDB 10.6 hoặc 10.11, Redis 7.2, OpenSearch 2.12.0-2.19.0 |
| Magento 2 | 2.4.6 | 8.2 | MariaDB 10.6 hoặc 10.11, Redis 7.0-7.2, OpenSearch 2.5.0-2.19.0 |

```bash
# Kiểm tra profile được áp dụng thực tế
govard config profile --json
govard config profile --framework laravel --framework-version 11 --json
```

---

## 🧱 Magento 2

Magento 2 là framework được hỗ trợ sâu sắc nhất trong Govard.

### Các tính năng chính

- `govard config auto` tự động cấu hình DB, cache, search, Varnish và các URL cơ sở vào `app/etc/env.php`.
- `govard tool magento [command]` chạy Magento CLI (`bin/magento`) bên trong container PHP.
- `govard tool magerun [command]` (Phím tắt: `mr`) chạy `n98-magerun2` bên trong container PHP.
- `govard tool magento cron:install` cài đặt các crontab bên trong container.
- Hỗ trợ Selenium/MFTF tùy chọn (`mftf: true` trong cấu hình features).
- Hỗ trợ LiveReload tùy chọn cho Grunt/Vite workflow (`livereload: true` trong features).
- Định tuyến riêng biệt `php-debug` khi bật Xdebug.

### Quy trình thông thường

```bash
govard env up
govard config auto
govard tool magento cache:clean
govard test phpunit
```

### 🏎️ LiveReload & Phát triển Frontend

Govard hỗ trợ quy trình LiveReload dựa trên Grunt tiêu chuẩn của Magento 2.

1. **Bật tính năng** trong file `.govard.yml`:
    ```yaml
    stack:
      features:
        livereload: true
    ```
2. **Áp dụng thay đổi**: Chạy `govard env up`. Lệnh này sẽ mở port `35729` ra máy host của bạn.
3. **Bắt đầu watcher**:
    ```bash
    govard shell
    # Bên trong shell:
    grunt watch
    ```
4. **Xác minh cấu hình**: Việc này được thiết lập **tự động** nếu bạn chạy `govard config auto`. Hệ thống sẽ inject đoạn mã sau vào `app/etc/env.php`:
    ```html
    <script src="http://localhost:35729/livereload.js?snipver=1"></script>
    ```
5. **Cài đặt trình duyệt**: Chỉ cần cài đặt [LiveReload Browser Extension](http://livereload.com/extensions/) hoặc dựa vào cơ chế tự động inject script ở trên.

::: tip GỢI Ý
Không cần phải inject script thủ công qua `default.xml` nữa. Mọi thứ được xử lý tự động bởi cơ chế auto-configuration của Govard qua `env.php`.
:::

::: info LƯU Ý
Vì port `35729` được ánh xạ trực tiếp tới máy host của bạn, bạn chỉ có thể chạy `livereload: true` cho duy nhất một dự án tại một thời điểm. Nếu bạn chạy nhiều dự án, hãy đảm bảo chỉ bật tính năng này cho dự án đang hoạt động.
:::

### Pipeline Nâng cấp Tự động (Native Upgrade Pipeline)

```bash
# Thử nghiệm nâng cấp trong một profile độc lập
cp .govard.yml .govard.upgrade-test.yml
GOVARD_ENV=upgrade-test govard upgrade --version 2.4.8-p4 --dry-run
GOVARD_ENV=upgrade-test govard upgrade --version 2.4.8-p4
```

Những gì lệnh `govard upgrade` thực hiện cho Magento 2:
- Xác định chính xác phiên bản PHP/MariaDB/Search tương ứng cho phiên bản Magento đích.
- Tự động gộp Composer (Composer merge) thông minh (giữ nguyên các module và custom repo của bạn).
- Tự động nới lỏng các ràng buộc phiên bản cho các công cụ dev (`phpunit`, `phpmd`).
- Xử lý các lệnh `composer update`, `setup:upgrade`, và compile static content.

### Setup Multi-Website / Multi-Store

```yaml
framework: "magento2"
domain: "primary.test"
store_domains:
  store-a.test:
    code: base
    type: website
  store-b.test:
    code: store_b
    type: store
```

```bash
govard domain add store-a.test
govard domain add store-b.test
govard config auto
govard tool magento cache:flush
```

**Những gì Govard tự động xử lý:**
- Định tuyến tất cả các domain qua proxy dùng chung với giao thức HTTPS.
- Cấu hình base URL toàn cục từ `domain`.
- Chạy lệnh `bin/magento config:set` phù hợp cho từng store view trong cấu hình `store_domains`.
- Inject biến host `MAGE_RUN_CODE` / `MAGE_RUN_TYPE` (dưới dạng object với `type` rõ ràng) tự động vào nginx/Apache.

**Những gì bạn vẫn cần làm:**
- Tạo các website, store và store view tương ứng trong admin panel của Magento.
- Xóa cache/config sau khi thay đổi ánh xạ store.

---

## 🌱 Mage-OS

Mage-OS là bản fork cộng đồng, có thể thay thế trực tiếp cho Magento 2 Open Source. Govard nhận diện Mage-OS qua `mage-os/product-community-edition` hoặc `mage-os/project-community-edition` trong `composer.json`, và tái sử dụng Docker image, template nginx cùng compose stack Varnish của Magento 2 — mọi công cụ Magento 2 ở trên (`govard tool magento`, `govard tool magerun`, `govard config auto`, định tuyến multi-site) đều áp dụng không thay đổi.

Cấu hình runtime mặc định: PHP 8.4.

### Cài đặt mới & Pipeline Nâng cấp Tự động

```bash
govard bootstrap --framework mageos --fresh
govard upgrade --version 1.3.1
```

`govard bootstrap`/`govard upgrade` sử dụng `mage-os/project-community-edition` và repository công khai của Mage-OS (`https://repo.mage-os.org`) thay vì repository riêng tư của Magento.

---

## 🛒 Magento 1 / OpenMage

```bash
govard tool magerun [command]
```

Cấu hình runtime mặc định: PHP 8.1 + MariaDB 10.11. Không bắt buộc sử dụng các dịch vụ cache/search/queue.

### Pipeline Nâng cấp Tự động

```bash
govard upgrade --version <version>
```

Xử lý: Đồng bộ Composer, xóa cache (`var/cache`, `var/session`, v.v.), bảo trì compiler, và thực thi nâng cấp database qua `n98-magerun`.

### Multi-Store với Định tuyến tường minh (Typed Routing)

```yaml
framework: "magento1"
domain: "primary.test"
store_domains:
  store-a.test:
    code: base
    type: website
  store-b.test:
    code: store_b
    type: store
  store-c.test: store_c   # dạng scalar = hành vi cũ (thử cho cả code website + store)
```

Dạng Object với trường `type` cụ thể giúp Govard tự động inject cấu hình biến host `MAGE_RUN_CODE` / `MAGE_RUN_TYPE` vào nginx/Apache — không cần cấu hình thủ công các luật `SetEnvIf` trong `.htaccess`.

---

## 🎨 Laravel

```bash
govard tool artisan [command]
```

Mặc định: thư mục web root `/public`, MariaDB 11.4, PHP tương ứng theo phiên bản.

### Pipeline Nâng cấp Tự động

```bash
govard upgrade --version 12
```

- Cập nhật ràng buộc framework trong `composer.json`.
- Chạy lệnh `composer update`.
- Chạy lệnh `php artisan migrate --force`.

---

## 🌐 Drupal

```bash
govard tool drush [command]
```

Mặc định: thư mục web root `/web`, MariaDB 11.4, PHP tương ứng theo phiên bản.

---

## ⚡ Symfony

```bash
govard tool symfony [command]
```

Mặc định: thư mục web root `/public`, MariaDB 11.4, PHP tương ứng theo phiên bản.

### Pipeline Nâng cấp Tự động

```bash
govard upgrade --version 7
```

- Cập nhật các ràng buộc của gói `symfony/framework-bundle`.
- Chạy lệnh `composer update`.
- Chạy lệnh `doctrine:migrations:migrate`.
- Chạy lệnh `cache:clear`.

---

## 🛍️ Shopware

```bash
govard tool shopware [command]
```

Mặc định: thư mục web root `/public`, MariaDB 11.4, PHP 8.4.

---

## 🍰 CakePHP

```bash
govard tool cake [command]
```

Mặc định: thư mục web root `/webroot`, MariaDB 11.4.

---

## 🏪 PrestaShop

```bash
govard tool prestashop [command]
```

Mặc định: thư mục web root là thư mục gốc dự án, MariaDB 10.11, PHP 8.1. Govard tự động nhận diện các dự án PrestaShop và hỗ trợ clone/cấu hình cho các bản cài đặt sẵn có; hiện chưa có luồng cài đặt mới (fresh-install) hay pipeline nâng cấp tự động (native upgrade pipeline) cho PrestaShop.

---

## 📰 WordPress

```bash
govard tool wp [command]
```

Mặc định: thư mục web root `/`, MariaDB 11.4, PHP 8.3.

### Cài đặt mới (Fresh Bootstrap)

WordPress fresh bootstrap tải mã nguồn gốc trực tiếp từ `wordpress.org` và cài đặt qua các script khởi tạo PHP — **không** yêu cầu công cụ `wp-cli` trong luồng cài đặt ban đầu.

```bash
govard bootstrap --framework wordpress --fresh
```

### Pipeline Nâng cấp Tự động

```bash
govard upgrade --version 6.7
```

- Chạy `wp core update --version=<version>`
- Chạy `wp core update-db`
- Chạy `wp cache flush`

---

## ⚡ Next.js

```bash
govard shell           # mở container web tại thư mục /app
govard tool npm [command]
govard tool npx [command]
```

Mặc định: Node 24, không ép buộc cấu hình database. Khởi chạy web tại thư mục gốc của dự án.

---

## 🔵 Emdash

Cấu hình local runtime ưu tiên Node: Node 22, không quản lý các dịch vụ PHP/DB/cache/search/queue.

```bash
govard shell           # container web tại thư mục /app
govard tool pnpm [command]
govard open admin      # mở trang /_emdash/admin
```

Khởi tạo mới hoàn toàn:

```bash
govard bootstrap --framework emdash --fresh
govard env up
```

**Tự động nhận diện Package Manager**: Govard đọc các thông tin từ `package.json` (trường `packageManager`), `pnpm-workspace.yaml` và các file lock.

> Phạm vi hiện tại là chạy Node + SQLite local + upload local. Govard chưa tự động hóa các luồng Cloudflare D1/R2.

---

## 🐍 Django

Cấu hình local runtime ưu tiên Python: Python 3.12 (có thể cấu hình qua `stack.python_version`), PostgreSQL 16, không quản lý các dịch vụ PHP/cache/search/queue.

```bash
govard shell           # container web tại thư mục /app
govard tool manage [command]   # chạy python manage.py [command]
govard db connect               # kết nối psql vào database postgres
```

Cài đặt mới (khởi tạo một dự án hoàn toàn mới):

```bash
mkdir myproject && cd myproject
govard init --framework django
govard bootstrap --fresh --framework django --framework-version 5.1
```

Cài đặt mới (clone một dự án có sẵn, sau đó bootstrap):

```bash
git clone <your-django-repo> myproject && cd myproject
govard init --framework django
govard env up
govard bootstrap --framework django
```

**Nhận diện:** bất kỳ dự án nào có file `manage.py` ở thư mục gốc.

> Phạm vi hiện tại chỉ hỗ trợ `requirements.txt` + `pip` (chưa hỗ trợ Poetry/`pyproject.toml`), chỉ hỗ trợ PostgreSQL (chưa có tùy chọn SQLite/MySQL), và dùng `manage.py runserver` cho local dev (chưa hỗ trợ Gunicorn). Cả hai luồng đều tự động chạy `pip install` + `manage.py migrate`. `--fresh` khởi tạo dự án qua `django-admin startproject config .`, cấu hình `settings.py` để dùng container Postgres mà Govard đã cung cấp sẵn, và cấu hình `ALLOWED_HOSTS`/`CSRF_TRUSTED_ORIGINS` theo domain của dự án.

---

## 🔧 Custom Stack

```bash
govard init --framework custom
```

Trình chọn tương tác cho các thành phần:
- Web server (`nginx`, `apache`, `hybrid`)
- Engine database và phiên bản tương ứng
- Dịch vụ cache
- Công cụ tìm kiếm
- Dịch vụ queue (hàng đợi)
- Tùy chọn Varnish

---

## Đóng góp một Framework mới

Muốn thêm một framework chưa có trong danh sách này? Xem [Thêm Framework mới](/vi/developer/adding-a-framework) để biết cấu trúc registry nội bộ và hướng dẫn từng file.

---

[← Cấu hình](/vi/reference/configuration) | [Remote & Đồng bộ →](/vi/workflows/remotes-and-sync)
