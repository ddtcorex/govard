---
title: Tài liệu cấu hình Govard
description: Cách hoạt động của cấu hình dự án phân lớp và blueprint framework trong Govard, bao gồm thứ tự ưu tiên ghi đè.
---

# Cấu hình (Configuration)

Govard sử dụng cấu hình phân tầng cho dự án kết hợp với các framework blueprint.

---

## Thứ tự ưu tiên của các lớp cấu hình

Govard tải cấu hình theo thứ tự sau (lớp sau sẽ ghi đè lên lớp trước):

| Ưu tiên | File | Mô tả |
| :---: | :--- | :--- |
| 1 | `.govard.yml` | Cấu hình cơ bản của team — file chính được phép ghi |
| 2 | `.govard.<profile>.yml` | Ghi đè profile dùng chung cho team |
| 3 | `.govard.local.yml` | Ghi đè cấu hình local của developer (cũ) |
| 4 | `.govard/.govard.local.yml` | Ghi đè cấu hình local của developer (**Khuyên dùng**) |
| 5 | `.govard.<env>.yml` | Ghi đè cấu hình môi trường (cũ) |
| 6 | `.govard/.govard.<env>.yml` | Ghi đè cấu hình môi trường (**Khuyên dùng**) |

### Model sở hữu (Ownership Model)

- **`.govard.yml`** — cấu hình cơ sở thuộc sở hữu của team; là mục tiêu cho mọi lệnh ghi `govard config set`.
- **Ghi đè Profile/local/env** — là read-only dưới góc nhìn của CLI; không bao giờ bị Govard tự động ghi đè.

---

## Profiles

Sử dụng profiles khi team cần nhiều mô hình runtime khác nhau cho cùng một dự án.

```bash
govard config profile switch upgrade   # Chuyển sang profile nâng cấp
govard env up --profile upgrade       # Hoặc dùng trực tiếp cờ --profile
govard db dump --profile perf
govard config profile clear            # Reset về mặc định (không dùng profile)
```

Govard tải `.govard.<profile>.yml` và tạo một file compose cô lập + các volume dữ liệu riêng biệt, vì vậy việc chuyển đổi profile không làm ảnh hưởng đến dữ liệu hiện tại của nhau.

**Các lệnh profile:**
- `govard config profile` - Hiển thị profile được khuyên dùng cho framework được nhận diện
- `govard config profile switch <name>` - Chuyển sang một profile cụ thể (lưu trên từng dự án)
- `govard config profile clear` - Reset về profile mặc định

---

## Ghi đè môi trường (Environment Override)

```bash
export GOVARD_ENV=staging
govard env up
```

Khi có `GOVARD_ENV=staging`, Govard sẽ tải thêm:
- `.govard.staging.yml`
- `.govard/.govard.staging.yml`

---

## Các biến môi trường toàn cục (Global Environment Variables)

| Biến | Tác dụng |
| :--- | :--- |
| `GOVARD_HOME_DIR` | Ghi đè thư mục `~/.govard` |
| `GOVARD_BLUEPRINTS_DIR` | Ghi đè vị trí tìm kiếm blueprint |
| `GOVARD_IMAGE_REPOSITORY` | Ghi đè tiền tố repository image được quản lý |
| `GOVARD_DOCKER_DIR` | Ghi đè local Docker build context cho các build fallback |

---

## Ví dụ về file `.govard.yml`

```yaml
project_name: "my_project"
framework: "magento2"
framework_version: "2.4.7"
domain: "myproject.test"
table_prefix: "demo_"
lock:
  strict: false
blueprint_registry:
  provider: "http"
  url: "https://example.com/govard-blueprints.tar.gz"
  checksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  trusted: false
stack:
  php_version: "8.4"
  node_version: "24"
  db_version: "11.4"
  web_root: "/public"
  cache_version: "7.4"
  search_version: "3.4.0"
  queue_version: "3.13.7"
  xdebug_session: "PHPSTORM"
  xdebug_version: "3.4.5"
  composer_version: "latest"
  services:
    web_server: "nginx"
    db: "mariadb"
    search: "opensearch"
    cache: "redis"
    queue: "none"
  features:
    xdebug: true
    varnish: false
    isolated: false
    mftf: false
    livereload: false
linked_projects:
    - "other-project"
    - "external-host.com:127.0.0.1"
```

---

## Các trường cấu hình chính

### Định danh dự án (Project Identity)

| Trường | Mô tả |
| :--- | :--- |
| `project_name` | Tên dự án độc nhất (không được trùng lặp với bất kỳ dự án nào khác) |
| `framework` | Framework được tự động nhận diện hoặc ép buộc |
| `framework_version` | Phiên bản framework (dùng cho các version-aware profile) |
| `domain` | Domain chính của dự án (ví dụ: `myproject.test`) |
| `extra_domains` | Các hostname bổ sung được định tuyến qua local proxy |
| `store_domains` | Magento multi-store hostname → ánh xạ mã scope |
| `table_prefix` | Tiền tố bảng database cho Magento 2, Mage-OS, Magento 1, OpenMage hoặc PrestaShop; bỏ qua hoặc để trống nếu không dùng |
| `linked_projects` | Danh sách các dependency (tên dự án hoặc IP:domain) để kết nối liên dự án |

::: important QUAN TRỌNG
`project_name` và `domain` phải là **độc nhất** trên tất cả các dự án Govard đang được theo dõi. Govard sẽ chặn lệnh `init` và `env up` nếu có dự án khác đã sử dụng trùng tên/domain.
:::

#### `store_domains` — Dạng đơn giản (Legacy)

```yaml
store_domains:
  brand-b.test: brand_b
  brand-c.test: brand_c
```

#### `store_domains` — Dạng Object (Định tuyến rõ ràng)

```yaml
store_domains:
  brand-b.test:
    code: base
    type: website
  brand-c.test:
    code: brand_c
    type: store
```

Dạng Object hướng dẫn Govard tự động cấu hình các mapping host `MAGE_RUN_CODE` / `MAGE_RUN_TYPE` một cách tự động.

#### Tiền tố bảng `table_prefix` — Magento Schemas

Sử dụng `table_prefix` khi các bảng cơ sở dữ liệu Magento có tiền tố, ví dụ `demo_core_config_data`:

```yaml
table_prefix: "demo_"
```

Govard sử dụng giá trị này cho Magento 2/Mage-OS `env.php`, Magento 1/OpenMage `local.xml`, PrestaShop `parameters.php`, SQL của lệnh `config auto`, các bộ lọc dữ liệu khi sync DB và quá trình migrate từ Warden. Giá trị này chỉ được chứa chữ cái, chữ số và dấu gạch dưới.

---

### Runtime Stack

| Trường | Tùy chọn | Mô tả |
| :--- | :--- | :--- |
| `stack.services.web_server` | `nginx`, `apache`, `hybrid` | Web server |
| `stack.services.db` | `mariadb`, `mysql`, `none` | Dịch vụ database |
| `stack.services.search` | `opensearch`, `elasticsearch`, `none` | Công cụ tìm kiếm |
| `stack.services.cache` | `redis`, `valkey`, `none` | Dịch vụ cache |
| `stack.services.queue` | `rabbitmq`, `none` | Dịch vụ queue (hàng đợi) |
| `stack.php_version` | ví dụ: `8.4`, `none` | Phiên bản PHP (`none` = không có PHP container) |
| `stack.node_version` | ví dụ: `24` | Phiên bản Node.js |
| `stack.db_version` | ví dụ: `11.4` | Phiên bản database |
| `stack.web_root` | ví dụ: `/pub`, `/public` | Thư mục web root |
| `stack.composer_version` | `1`, `2`, `2.2`, `latest`, hoặc bất kỳ phiên bản nào | Phiên bản Composer |
| `stack.xdebug_session` | ví dụ: `PHPSTORM` | Tên session Xdebug |
| `stack.xdebug_version` | ví dụ: `3.4.5` | Ghi đè phiên bản PECL Xdebug được cài trong container `php-debug` (mặc định: phiên bản Govard khuyến nghị theo PHP version — hiện là `3.4.5` cho PHP 8.1-8.4, `3.5.3` cho PHP 8.5 vì phiên bản này không có lựa chọn 3.4.x). Việc này sẽ buộc build image cục bộ vì phiên bản cụ thể được đóng gói sẵn trong image. |
| `stack.features.livereload` | `true`, `false` | Bật map port LiveReload (35729) |
| `stack.features.varnish` | `true`, `false` | Bật dịch vụ cache Varnish |
| `stack.features.xdebug` | `true`, `false` | Bật Xdebug và dịch vụ php-debug |
| `stack.features.isolated` | `true`, `false` | Cách ly network không cho truy cập từ bên ngoài |
| `stack.features.mftf` | `true`, `false` | Bật Magento Functional Testing Framework |

Đối với các framework ưu tiên Node, hệ thống tự động nhận diện package manager từ `package.json`, `pnpm-workspace.yaml` hoặc các file lock.

#### Tối ưu hóa phiên bản Composer
Govard cung cấp hỗ trợ trực tiếp cho các phiên bản Composer phổ biến nhằm đảm bảo môi trường khởi động ngay lập tức:
- **Tích hợp sẵn (Tốc độ tức thì)**: `1`, `2`, `2.2`, `latest`. Các phiên bản này được đóng gói sẵn trong PHP image và không cần tải về khi chạy.
- **Động (Tự động tải về)**: Có thể cấu hình bất kỳ phiên bản chi tiết nào (ví dụ: `2.7.2`). Govard sẽ tự động tải về và xác minh binary vào lần chạy `env up` đầu tiên.

---

### An toàn và Tính tái lặp (Safety and Reproducibility)

| Trường | Mô tả |
| :--- | :--- |
| `lock.strict` | Dừng chạy `env up` khi trạng thái lock file bị thiếu hoặc không khớp |
| `lock.ignore_fields` | Các trường cần bỏ qua khi kiểm tra tính tuân thủ (ví dụ: `host.docker_version`) |
| `blueprint_registry.*` | Đăng ký nguồn blueprint remote tùy chọn với yêu cầu về checksum + độ tin cậy |

---

### Remotes

Các cấu hình remote nằm dưới khóa `remotes.<name>`. Tên có thể là bất kỳ định danh hợp lệ nào — Govard chấp nhận các tên tiêu chuẩn (`dev`, `staging`, `prod`) cũng như **bất kỳ tên tùy chỉnh nào** sử dụng chữ thường, chữ số, dấu gạch ngang hoặc gạch dưới (ví dụ: `qa`, `preprod`, `demo`, `client-uat`).

```yaml
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /var/www/app
    port: 22
    capabilities:
      files: true
      media: true
      db: true
    protected: false
    auth:
      method: ssh-agent

  qa:
    host: qa.example.com
    user: deploy
    path: /var/www/app
    auth:
      method: keychain

  preprod:
    host: preprod.example.com
    user: deploy
    path: /var/www/app
    protected: true   # bật tính năng chống ghi đè cho môi trường tùy chỉnh
    auth:
      method: ssh-agent
```

::: info LƯU Ý
Chỉ các remote có tên chuẩn hóa thành `prod` (`prod`, `production`, `live`) là **tự động** được bảo vệ chống ghi đè (write-protected). Tất cả các remote khác — bao gồm cả tên tùy chỉnh — mặc định không được bảo vệ. Sử dụng cấu hình `protected: true` để kích hoạt thủ công.
:::

Các trường con chính:

| Trường | Mô tả |
| :--- | :--- |
| `capabilities` | Các cờ phạm vi: `files`, `media`, `db`, `deploy` |
| `protected` | Bảo vệ chống ghi đè cho remote này |
| `auth.method` | `keychain`, `ssh-agent`, hoặc `keyfile` |
| `auth.key_path` | Đường dẫn tới key SSH (cho phương thức `keyfile`) |
| `auth.strict_host_key` | Bật xác thực host-key nghiêm ngặt |
| `auth.known_hosts_file` | Đường dẫn file known_hosts tùy chỉnh |

Các trường thông tin remote hỗ trợ các tham chiếu `op://...` được giải quyết thông qua 1Password CLI.

→ Hướng dẫn đầy đủ: [Remote & Đồng bộ](/vi/workflows/remotes-and-sync)

---

### Tiện ích mở rộng dự án (Project Extensions)

| Đường dẫn | Mục đích |
| :--- | :--- |
| `.govard/docker-compose.override.yml` | Ghi đè Compose được merge sau khi include framework |
| `.govard/commands/*` | Các lệnh tùy chỉnh được hiển thị qua `govard custom` |
| `.govard/hooks/*` | Các script được tham chiếu bởi `hooks.*.run` |
| `.govard/nginx/custom/*.conf` | Directive nginx bổ sung, được include vào bên trong `server {}` đã render (chỉ áp dụng cho nginx) |
| `.govard/apache/custom/*.conf` | Directive Apache bổ sung, được include vào bên trong `<VirtualHost>` đã render (chỉ áp dụng cho Apache) |

**Các sự kiện lifecycle hook:**

- `pre-up` / `post-up`
- `pre-down` / `post-down`
- `pre-deploy` / `post-deploy`
- `pre-delete` / `post-delete`

::: tip GỢI Ý
Govard tạo mã hash vân tay cho `.govard/docker-compose.override.yml`, `.govard/nginx/custom/`, và `.govard/apache/custom/`. Nếu bất kỳ thứ nào trong số này thay đổi, lệnh `env up` tiếp theo sẽ tự động re-render cấu trúc compose.

Khi ghi đè dịch vụ, nên ưu tiên các bổ sung nhỏ (thêm biến môi trường, label, port). Việc thay thế hoàn toàn danh sách như `services.web.volumes` có thể làm mất các mount quan trọng do Govard quản lý. `.govard/nginx/custom/` và `.govard/apache/custom/` tồn tại chính xác để bạn không cần thay thế toàn bộ cấu hình web server chỉ để thêm một directive.
:::

---

## Lệnh cấu hình

```bash
govard config get stack.php_version
govard config set stack.php_version 8.4
govard config profile --json
govard config profile apply --framework laravel --framework-version 11
```

`govard config set` chỉ ghi trực tiếp vào `.govard.yml` (cấu hình cơ sở).

---

## Blueprint Registry

Nếu `blueprint_registry` được kích hoạt:

- `provider` phải là `git` hoặc `http`
- `url` là bắt buộc
- `checksum` phải là mã SHA-256 hex dài 64 ký tự
- `trusted` phải là `true`
- Các package remote được cache tại `~/.govard/blueprint-registry/`

Govard sẽ thất bại ngay lập tức nếu checksum không khớp.

---

## Kết nối liên dự án (Inter-Project Connectivity)

Mặc định, các dự án Govard được cô lập. Để cho phép một dự án giao tiếp với một dự án Govard khác qua tên miền `.test` của nó, hãy sử dụng trường `linked_projects`.

### Các hành vi chính

- **Hiển thị theo dạng Opt-in**: Các hostname của dự án khác chỉ được inject vào `/etc/hosts` nếu dự án đó được khai báo rõ ràng trong `linked_projects`.
- **Tự động phân giải Domain**: Khai báo tên dự án sẽ tự động map domain chính và tất cả các domain phụ của nó về IP của proxy dùng chung.
- **Khởi động lại Container có chọn lọc**: Khi bạn khởi động một dự án, Govard sẽ xác định các dự án đang chạy nào phụ thuộc vào nó và **chỉ** khởi động lại các dự án cụ thể đó để cập nhật mapping host.
- **Mapping thủ công**: Bạn cũng có thể cung cấp các cấu hình mapping thủ công theo định dạng `hostname:ip`.

```yaml
linked_projects:
  - "my-api-project"             # Tên dự án
  - "custom.site:192.168.1.10"   # Ánh xạ thủ công
```

---

[← Lệnh CLI](/vi/reference/cli-commands) | [Frameworks →](/vi/reference/frameworks)