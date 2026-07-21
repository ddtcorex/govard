---
title: Tài liệu tham khảo lệnh CLI Govard
description: Tài liệu đầy đủ về các lệnh CLI, alias và shortcut của Govard để quản lý môi trường phát triển Docker cục bộ.
---

# Lệnh CLI (CLI Commands)

Đây là tài liệu tham khảo chính thức cho các lệnh CLI của Govard.

---

## Các phím tắt và Tên viết tắt (Aliases and Shortcuts)

### Phím tắt quản lý Lifecycle gốc

| Phím tắt | Lệnh tương đương |
| :--- | :--- |
| `govard up` | `govard env up` |
| `govard down` | `govard env down` |
| `govard restart` | `govard env restart` |
| `govard ps` | `govard env ps` |
| `govard logs` | `govard env logs` |

### Tên viết tắt của các lệnh

| Tên viết tắt | Lệnh đầy đủ |
| :--- | :--- |
| `govard boot` | `govard bootstrap` |
| `govard cfg` | `govard config` |
| `govard dbg` | `govard debug` |
| `govard gui` | `govard desktop` |
| `govard diag` | `govard doctor` |
| `govard ext` | `govard extensions` |
| `govard prj` | `govard project` |
| `govard rmt` | `govard remote` |
| `govard sh` | `govard shell` |
| `govard snap` | `govard snapshot` |

### Viết tắt của lệnh `govard tool`

| Tên viết tắt | Lệnh đầy đủ |
| :--- | :--- |
| `govard tool mr` | `govard tool magerun` |

### Viết tắt của lệnh `govard sync`

- `--from` là viết tắt của `--source`
- `--to` là viết tắt của `--destination`
- `-e, --environment` là tùy chọn môi trường nguồn được tiếp tục hỗ trợ

---

## 🌿 Các lệnh môi trường (Environment Commands)

### `govard init`

Phát hiện framework của dự án và tạo cấu hình `.govard.yml`.

```bash
govard init
govard init --framework magento2
govard init --framework custom
govard init --migrate-from warden
```

Khi di chuyển từ Warden, lệnh `govard init --migrate-from warden` tự động ánh xạ `WARDEN_TABLE_PREFIX` sang cấu hình `table_prefix` của Govard cho các dự án Magento 2, Magento 1 và OpenMage.

### `govard bootstrap`

Chạy các quy trình khởi tạo dự án khi clone hoặc cài đặt mới hoàn toàn.

```bash
govard bootstrap
govard bootstrap --clone --environment staging --yes
govard bootstrap --framework magento2 --fresh --framework-version 2.4.9
govard bootstrap -e staging --no-pii --no-noise
```

**Lựa chọn chế độ (Mode selection):**
- `--fresh` + `--framework` + `--framework-version` — cài đặt mới hoàn toàn qua scaffolder của framework.
- `--clone` + `--environment` — rsync toàn bộ mã nguồn từ một remote server.

**Lựa chọn nguồn (Source selection):**
- `-e, --environment` — tên của remote nguồn; chấp nhận các tên chuẩn (`staging`, `production`, `dev`) cũng như các định danh tùy chỉnh (`qa`, `preprod`, `demo`, `client-uat`).
- `--remote` — tên viết tắt của `--environment`.
- `--db-dump` — import database trực tiếp từ một đường dẫn file SQL local.

**Các bộ lọc hiệu năng & bảo mật dữ liệu:**

| Cờ (Flag) | Tác dụng |
| :--- | :--- |
| `-N, --no-noise` | Loại bỏ các dữ liệu rác/tạm thời (logs, sessions, cache tags, lịch sử cron) |
| `-S, --no-pii` | Loại bỏ các dữ liệu cá nhân nhạy cảm (thông tin khách hàng, đơn hàng, tài khoản admin, password) |
| `--delete` | Xóa các file ở đích nếu không tồn tại ở nguồn |
| `--no-compress` | Tắt nén khi chạy rsync |
| `-X, --exclude` | Các pattern loại trừ rsync tùy chỉnh (có thể lặp lại nhiều lần) |
| `--no-db` | Bỏ qua bước import database |
| `--no-media` | Bỏ qua bước đồng bộ file media |
| `--media [mode]` | Chế độ đồng bộ media (`none`, `minimal`, `optimized`, `all`) |
| `--no-composer` | Bỏ qua việc chạy `composer install` |
| `--no-admin` | Bỏ qua bước tạo tài khoản admin (chỉ áp dụng cho Magento 2) |
| `--no-stream-db` | Sử dụng một file tạm local để truyền DB thay vì stream trực tiếp |
| `--no-up` | Bỏ qua bước khởi động container local trước khi chạy bootstrap |

Đối với các dự án Magento có thiết lập `table_prefix`, các bộ lọc bảo mật DB sẽ tự động áp dụng chính xác cho các bảng có tiền tố tương ứng.

**Các cờ đặc thù của Magento:**

| Cờ | Tác dụng |
| :--- | :--- |
| `--include-sample` | Cài đặt dữ liệu mẫu (cho cài đặt mới) |
| `--hyva-install` | Tự động cài đặt theme Hyva |

**Xem trước kế hoạch & Xác nhận:**
- `--plan` — hiển thị kế hoạch thực thi rồi thoát, không chạy thực tế.
- `-y, --yes` — bỏ qua bước xác nhận tương tác (tiện lợi cho CI/non-interactive).

### `govard env`

Quản lý lifecycle của dự án và là wrapper của Docker Compose.

```bash
govard env up
govard env start
govard env stop
govard env restart
govard env down
govard env ps
govard env logs php -f
govard env pull
govard env build
govard env cleanup
```

**Các cờ của `govard env up`:**

| Cờ | Tác dụng |
| :--- | :--- |
| `--pull` | Tải về các image mới nhất trước khi chạy |
| `--fallback-local-build` | Build các image bị thiếu ở local |
| `--remove-orphans` | Xóa các container không còn trong cấu hình (orphaned) |
| `--quickstart` | Đường dẫn khởi động nhanh nhất |
| `--update-lock` | Tự động cập nhật `govard.lock` nếu phát hiện sai lệch |
| `--no-tuning` | Bỏ qua các prompt cấu hình tự động cho framework |

**Các file được render lại khi chạy `env up`:**
- `~/.govard/compose/<project-hash>.yml`
- `~/.govard/nginx/<project>/default.conf`
- `~/.govard/apache/<project>/httpd.conf`
- `~/.govard/nginx/<project>/mage-run-map.conf`

**Các cờ của `govard env down`:**
- `-v, --volumes` — xóa các docker volume dữ liệu đi kèm.
- `--rmi local` — xóa các image local được dựng cho dự án.

### `govard svc`

Quản lý các dịch vụ toàn cục dùng chung (proxy, Mailpit, PHPMyAdmin, Portainer).

```bash
govard svc up
govard svc restart --no-trust
govard svc logs --tail 50
govard svc sleep
govard svc wake
```

> **Portainer** có thể truy cập tại `https://portainer.govard.test`
> Đăng nhập mặc định: `admin` / `AdminGovard123$`

### `govard domain`

Quản lý các domain phụ local cho dự án hiện tại.

```bash
govard domain add brand-b.test
govard domain remove brand-b.test
govard domain list
```

### `govard status`

Liệt kê tất cả các môi trường Govard đang chạy trong toàn bộ workspace của bạn.

```bash
govard status
```

### `govard desktop`

Khởi chạy ứng dụng Wails desktop.

```bash
govard desktop
govard desktop --dev
govard desktop --background
```

Xem tài liệu [Ứng dụng Desktop](/vi/workflows/desktop-app) để biết thêm chi tiết.

---

## 🛠️ Các lệnh phát triển (Development Commands)

### `govard shell`

Mở terminal kết nối trực tiếp vào bên trong container ứng dụng.

```bash
govard shell
govard shell --no-tty
```

- PHP frameworks → Vào container `php` tại thư mục `/var/www/html`
- Node-first frameworks (Next.js, Emdash) → Vào container `web` tại thư mục `/app`

### `govard debug`

Quản lý trạng thái hoạt động và các session của Xdebug.

```bash
govard debug status
govard debug on
govard debug off
govard debug shell
```

Các request chỉ được định tuyến tới `php-debug` khi cookie `XDEBUG_SESSION` trùng khớp với giá trị của `stack.xdebug_session`.

### `govard test`

Khởi chạy các công cụ test bên trong container ứng dụng.

```bash
govard test phpunit
govard test phpstan
govard test mftf
govard test integration
```

### `govard custom`

Chạy các lệnh tùy chỉnh được cấu hình trong `.govard/commands` hoặc `~/.govard/commands`.

```bash
govard custom list
govard custom hello
govard custom deploy -- --dry-run
```

### `govard project`

Xem và quản lý các dự án Govard đã đăng ký trên hệ thống.

```bash
govard project list
govard project list --orphans
govard project open billing
govard project delete demo
govard project delete --yes demo
```

::: warning CẢNH BÁO
Lệnh `govard project delete` mặc định sẽ xóa hoàn toàn các volume database của dự án đó. Mã nguồn của dự án **không bao giờ** bị xóa.
:::

**Quy trình xóa dự án:**
1. Chạy các lifecycle hook `pre-delete`.
2. Thực thi lệnh `docker compose down -v` (xóa container + volume).
3. Hủy đăng ký các domain trên proxy.
4. Xóa thông tin dự án khỏi registry (`projects.json`).
5. Chạy các hook `post-delete`.

---

## 🔗 Các lệnh Remote, Đồng bộ và Dữ liệu (Remote, Sync, & Data)

### `govard remote`

Quản lý các môi trường remote được định danh để sử dụng cho đồng bộ, deploy, shell và truy cập cơ sở dữ liệu.

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/app
govard remote copy-id staging
govard remote test staging
govard remote exec staging -- ls -la
govard remote audit tail --status failure --lines 50
```

Đối với các đường dẫn remote tương đối với thư mục home, hãy đóng dấu nháy đơn cho giá trị đường dẫn:

```bash
govard remote add staging --host staging.example.com --user deploy --path '~/public_html'
```

Các tính năng chính:
- Capabilities (Quyền hạn): `files`, `media`, `db`, `deploy`.
- Các phương thức đăng nhập: `keychain`, `ssh-agent`, `keyfile`.
- Tự động bảo vệ chống ghi đè cho môi trường production.
- Ghi nhật ký lịch sử thao tác: `~/.govard/remote.log`.

→ Hướng dẫn đầy đủ: [Remote & Đồng bộ](/vi/workflows/remotes-and-sync)

### `govard sync`

Đồng bộ các file, media, hoặc database giữa môi trường local và các remote server.

```bash
govard sync --source staging --destination local --full --plan
govard sync --from staging --to local --media
govard sync -s prod --file --path app/etc/config.php
govard sync --db --no-noise --no-pii
```

Tự động chọn remote `staging` nếu không truyền cờ `--source`, và fallback về `dev`.
Khi cờ `--media` được gọi mà không truyền mode cụ thể, Govard sẽ mặc định chạy ở chế độ `optimized`.

**Các cờ chính:**

| Cờ | Tác dụng |
| :--- | :--- |
| `-s, --source` / `--from` | Môi trường nguồn |
| `-d, --destination` / `--to` | Môi trường đích |
| `--file`, `--media`, `--db`, `--full` | Phạm vi đồng bộ dữ liệu |
| `--plan` | Chỉ hiển thị kế hoạch thực thi rồi thoát |
| `-I, --include` | Pattern bao gồm của rsync (có thể khai báo nhiều lần) |
| `-X, --exclude` | Pattern loại trừ của rsync (có thể khai báo nhiều lần) |
| `-m, --media [mode]` | Phạm vi đồng bộ media (`none`, `minimal`, `optimized`, `all`); cờ `--media` đơn lẻ mặc định là `optimized` |
| `-N, --no-noise` | Loại bỏ các dữ liệu rác khi đồng bộ |
| `-P, --no-pii` | Loại bỏ thông tin cá nhân nhạy cảm khi đồng bộ |

### `govard db`

Các tiện ích quản lý và truy vấn database local và remote.

```bash
govard db connect
govard db dump
govard db dump -e staging --local
govard db query "SELECT COUNT(*) FROM sales_order"
govard db info
govard db top
govard db import --file backup.sql --drop
govard db import --stream-db -e staging --drop
govard db clone-volume warden_magento2_dbdata
```

### `govard deploy`

Chạy các deploy lifecycle hook được cấu hình cho dự án hiện tại.

```bash
govard deploy
```

### `govard snapshot`

Quản lý các bản snapshot dữ liệu DB và media local/remote nhanh chóng.

```bash
govard snapshot create
govard snapshot create -e staging
govard snapshot list
govard snapshot list -e staging
govard snapshot restore latest
govard snapshot pull latest -e staging
govard snapshot push before-deploy -e prod
```

### `govard open`

Mở nhanh các đường dẫn dịch vụ/ứng dụng trên trình duyệt.

```bash
govard open app
govard open admin
govard open mail
govard open db
govard open db --pma
govard open db --client
govard open db -e staging
```

### `govard tunnel`

Quản lý các đường link public tunnel (yêu cầu cài đặt `cloudflared`).

```bash
govard tunnel start
govard tunnel status
govard tunnel stop
```

::: important QUAN TRỌNG
Binary `cloudflared` phải được bạn tự cài đặt riêng trên hệ thống.
Cài đặt thông qua [kho lưu trữ chính thức của Cloudflare](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/install-run/install-threads/) hoặc tải từ [releases trên GitHub](https://github.com/cloudflare/cloudflared/releases).
:::

---

## 🔧 Các lệnh gọi công cụ framework (Tool Commands)

Khởi chạy các CLI của framework bên trong container ứng dụng:

```bash
govard tool magento [command]    # Magento 2
govard tool magerun [command]    # Magento 1 / Magento 2 (Viết tắt: mr)
govard tool artisan [command]    # Laravel
govard tool drush [command]      # Drupal
govard tool symfony [command]    # Symfony
govard tool shopware [command]   # Shopware
govard tool cake [command]       # CakePHP
govard tool wp [command]         # WordPress
govard tool prestashop [command] # PrestaShop

# Các công cụ quản lý package & build dùng chung
govard tool composer [command]
govard tool php [command]        # Chạy trực tiếp CLI PHP (vd: tích hợp editor/IDE)
govard tool npm [command]
govard tool yarn [command]
govard tool npx [command]
govard tool pnpm [command]
govard tool grunt [command]
```

Đối với các dự án node-first, các lệnh quản lý package sẽ được chạy trong container `web` tại thư mục `/app`.

`govard tool php` yêu cầu thư mục hiện tại phải đúng là project root. Với các tích hợp editor/IDE (xem phần dưới), dùng `govard vscode` thay thế.

---

## 🧩 Các lệnh tích hợp Editor (Editor Integration Commands)

### `govard vscode setup`

Ghi (hoặc merge vào) các cấu hình VSCode cần thiết để chạy công cụ PHP bên trong container thay vì trên host:

```bash
# Chạy từ trong project (hoặc bất kỳ thư mục con nào của project)
govard vscode setup
#   -> .vscode/settings.json: intelephense.environment.phpVersion, phpstan.paths,
#                             phpunit.paths (nếu có vendor/bin/phpunit),
#                             và (nếu có vendor/bin/phpcs) phpcs.standard + phpcs.autoConfigSearch=false
#   -> .vscode/launch.json:   cấu hình "Listen for Xdebug (Govard)" (port 9003)

# Chỉ chạy 1 lần, áp dụng cho mọi project Govard
govard vscode setup --global
#   -> tạo wrapper script ~/.govard/bin/govard-php, govard-php-cs-fixer, và govard-phpcs
#   -> user settings.json: php.validate.executablePath, phpstan.binCommand,
#                          php-cs-fixer.executablePath, phpcs.executablePath, phpunit.command
```

Settings phản ánh đúng profile đang dùng gần nhất của project (vd. profile upgrade ghim version PHP mới hơn) nếu có đăng ký, thay vì luôn đọc `.govard.yml` gốc — nên `intelephense.environment.phpVersion` khớp với thực tế đang chạy.

Coding standard cho PHPCS được tự nhận diện từ `composer.json` (`magento/magento-coding-standard` -> `Magento2`, `wp-coding-standards/wpcs` -> `WordPress`, `drupal/coder` -> `Drupal`), fallback về `PSR12` nếu không khớp gói nào. `phpcs.autoConfigSearch` bị tắt vì nếu không, extension sẽ tự dò file `phpcs.xml`/`.dist` và truyền path tuyệt đối *trên host* làm `--standard` — container không đọc được path đó.

Nếu có `vendor/bin/phpstan` nhưng project **chưa có** config `phpstan.neon`/`.dist`/`dist.neon` riêng, `setup` sẽ set `phpstan.options` với mặc định `--level=0` (`--autoload-file=vendor/autoload.php` cộng `app/code`+`app/design` cho Magento 2 hoặc `app`+`src` cho framework khác — đúng convention `govard test phpstan` đã dùng khi fallback) để PHPStan có gì đó mà phân tích. Cái này cố tình nằm trong `.vscode/settings.json`, không phải tạo file `phpstan.neon` ở root project — file đó thường bị git track và không phải của mình để tạo ra. Ngay khi project có config riêng, chạy lại `setup` sẽ tự xoá `phpstan.options` để không bao giờ đè lên rule thật của project — config của project luôn được ưu tiên.

`phpunit.command` (recca0120.vscode-phpunit) không cần wrapper script — đây là template mà extension tự tokenize, nên được set thẳng thành `govard vscode phpunit ${phpunitargs}`. Bạn có panel Testing (chạy/rerun từng test) mà không cần cài PHPUnit trên host. Debug từng test qua extension này chưa được wire — cần forward biến môi trường Xdebug vào lệnh `docker exec`.

Mỗi nhóm cấu hình cần đúng 1 extension VSCode tương ứng (Intelephense, PHPStan, PHP CS Fixer, PHPCS, PHPUnit, PHP Debug). Nếu chưa cài, `setup` sẽ cảnh báo và hỏi có muốn cài ngay qua `code --install-extension` không — đồng ý thì setting tương ứng vẫn được wire luôn trong lần chạy đó. Truyền `--yes` để tự cài hết những gì thiếu mà không hỏi (hữu ích khi chạy script); nếu không có TTY và không có `--yes`, các extension còn thiếu sẽ tự bị bỏ qua, không hỏi.

Các key hiện có và các configuration khác trong `launch.json` được giữ nguyên — chỉ những key do Govard quản lý mới bị thêm/ghi đè. Lưu ý: settings.json được parse như JSON thuần, nên comment (nếu có) sẽ bị mất khi ghi lại.

### `govard vscode <tool>`

Các lệnh chạy tool thực tế mà cấu hình do `setup` ghi ra sẽ trỏ vào:

```bash
govard vscode php [args]
govard vscode composer [args]
govard vscode phpstan [args]       # vendor/bin/phpstan
govard vscode php-cs-fixer [args]  # vendor/bin/php-cs-fixer
govard vscode phpcs [args]         # vendor/bin/phpcs
govard vscode phpunit [args]       # vendor/bin/phpunit, kèm memory_limit=-1
```

Khác với `govard tool`, các lệnh này tự tìm project bằng cách đi ngược thư mục từ vị trí hiện tại lên để tìm `.govard.yml` gần nhất — vì các editor thường gọi tool với thư mục làm việc không phải workspace root (vd: thư mục chứa file đang mở), nên không thể chắc chắn cwd khớp chính xác.

---

## ⚙️ Các lệnh cấu hình (Configuration Commands)

```bash
govard config get stack.php_version
govard config set stack.php_version 8.4
govard config set table_prefix demo_
govard config profile              # Hiển thị cấu hình profile đề xuất cho framework
govard config profile --json      # Output thông tin profile dạng JSON
govard config profile apply       # Áp dụng profile đề xuất vào .govard.yml
govard config auto                # Magento 2: inject các thiết lập kết nối vào env.php
```

### `govard config profile`

Hiển thị profile môi trường được khuyến nghị cho framework đã phát hiện.

```bash
govard config profile
govard config profile --json
```

Output bao gồm framework được phát hiện, phiên bản PHP đề xuất, cấu hình database, cache, search và các dịch vụ stack đi kèm khác.

### `govard config profile switch`

Chuyển đổi sang một profile môi trường cấu hình khác. Tính năng này cho phép bạn chạy cùng một dự án với các cấu hình runtime khác nhau (ví dụ: chạy PHP 8.2 để test production, chạy PHP 8.3 để code phát triển).

```bash
govard config profile switch upgrade
govard config profile switch staging
govard config profile switch          # Lựa chọn dạng tương tác trực quan
```

Các file profile được lưu trữ dưới dạng `.govard.<name>.yml` trong thư mục gốc dự án. Profile đang được chọn sẽ được ghi nhớ trên từng dự án tại đường dẫn `~/.govard/projects.json`.

Sau khi chuyển đổi, chạy `govard env up` để áp dụng môi trường mới. Bạn sẽ được nhắc xác nhận khi profile thay đổi yêu cầu khởi động lại container.

### `govard config profile clear`

Reset môi trường về lại profile mặc định (không sử dụng profile phụ).

```bash
govard config profile clear
```

### `govard extensions`

Khởi tạo các khung template mở rộng tại thư mục `.govard/*`.

```bash
govard extensions init
govard extensions init --force
```

### `govard blueprint cache`

Quản lý bộ nhớ cache của các registry blueprint tải từ xa.

```bash
govard blueprint cache list
govard blueprint cache clear
```

---

## 🩺 Diagnostics (Chẩn đoán lỗi)

### `govard doctor`

Khởi chạy hệ thống chẩn đoán lỗi môi trường kèm giải pháp sửa đổi cụ thể.

```bash
govard doctor
govard doctor --fix
govard doctor --json
govard doctor --pack
govard doctor trust
```

Các thành phần được kiểm tra bao gồm: Docker, Compose, các port kết nối, dung lượng ổ đĩa, tình trạng thư mục Govard home, sức khỏe thư mục compose, SSH agent và kết nối mạng ra ngoài.

- **`--fix`** — Tự động phát hiện và sửa các lỗi phổ biến được tìm thấy.
- **`trust`** — Cài đặt Root CA vào keychain hệ thống + browser NSS store.

---

## 🔁 Các lệnh tiện ích (Utility Commands)

### `govard lock`

Tạo hoặc kiểm định file `govard.lock` phục vụ cho việc phát hiện sai lệch cấu hình môi trường giữa các máy.

```bash
govard lock generate
govard lock check
govard lock diff
govard lock generate --file .govard/govard.lock
```

### `govard self-update`

Tải về phiên bản Govard mới nhất, kiểm định mã checksum và thay thế các binary đã cài đặt một cách an toàn.

```bash
govard self-update
```

### `govard upgrade`

Pipeline hỗ trợ nâng cấp framework native.

```bash
govard upgrade --version 2.4.8-p4     # Magento 2
govard upgrade --version 11            # Laravel
```

**Các cờ:**

| Cờ | Tác dụng |
| :--- | :--- |
| `--version` | Phiên bản đích nâng cấp (bắt buộc) |
| `--dry-run` | Xem trước các bước thực thi không chạy thực tế |
| `--no-db-upgrade` | Bỏ qua chạy các câu lệnh migration database |
| `--no-env-update` | Bỏ qua cập nhật profile và restart container |
| `-y, --yes` | Tự động đồng ý qua các câu hỏi xác nhận |

### `govard version`

```bash
govard version
```

### `govard redis`

Tiện ích thao tác nhanh với Redis/Valkey.

```bash
govard redis cli
govard redis flush
govard redis info
```

### `govard varnish`

Tiện ích thao tác nhanh với Varnish.

```bash
govard varnish purge
govard varnish status
```

### `govard rabbitmq`

Tiện ích thao tác nhanh với RabbitMQ.

```bash
govard rabbitmq status
govard rabbitmq queues
govard rabbitmq cli list_exchanges
```

---

## 🌐 Các cờ toàn cục (Global Flags)

Tất cả các lệnh của Govard đều hỗ trợ:

- `-h, --help` — Hiển thị trợ giúp của lệnh

---

[← Bắt đầu](/vi/getting-started/getting-started) | [Cấu hình →](/vi/reference/configuration)