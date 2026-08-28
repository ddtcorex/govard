---
title: Môi trường Remote & Đồng bộ dữ liệu
description: Quản lý remote có tên với quyền hạn giới hạn, chặn ghi lên production an toàn, và đồng bộ file/media/database hai chiều có dry-run.
---

# Remote và Đồng bộ (Remotes and Sync)

Đây là tài liệu hướng dẫn chính thức về môi trường remote của Govard, các hoạt động đồng bộ và quy trình xử lý database từ xa.

---

## Cấu hình Remote

### Thêm một Remote

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/app
```

**Các cờ của `remote add`:**

| Cờ | Mô tả |
| :--- | :--- |
| `--host` | Tên miền hoặc IP của remote |
| `--user` | Tên đăng nhập SSH |
| `--path` | Đường dẫn dự án trên remote |
| `--port` | Port SSH (mặc định: 22) |
| `--capabilities` | Các quyền hạn, phân tách bằng dấu phẩy (`files,media,db,deploy`) |
| `--auth-method` | Phương thức đăng nhập (`keychain`, `ssh-agent`, `keyfile`) |
| `--key-path` | Đường dẫn tới SSH key (cho phương thức `keyfile`) |
| `--strict-host-key` | Kích hoạt xác minh nghiêm ngặt host-key |
| `--known-hosts-file` | Đường dẫn file known_hosts tùy chỉnh |
| `--protected` | Bảo vệ chống ghi đè cho remote này |

::: tip GỢI Ý
Để sử dụng thư mục home của user trên remote, hãy đóng dấu nháy đơn cho đường dẫn để shell local không tự động expand nó:
```bash
govard remote add staging --host staging.example.com --user deploy --path '~/public_html'
```
:::

### Xác minh kết nối

```bash
govard remote copy-id staging    # Copy public key local của bạn vào authorized_keys của remote
govard remote test staging       # Kiểm tra kết nối SSH + rsync, đo độ trễ và phân loại lỗi
```

Lệnh `remote test` phân loại chính xác các lỗi: `network`, `auth`, `permission`, `host_key`, `dependency`.

### Thực thi lệnh và Giám sát lịch sử (Exec & Audit)

```bash
govard remote exec staging -- ls -la
govard remote audit tail --lines 20
govard remote audit tail --status failure --lines 50
govard remote audit stats --lines 200
```

**Đường dẫn file nhật ký (Audit log paths):**
- `~/.govard/remote.log`
- `~/.govard/operations.log`

---

## Cơ chế bảo mật Remote (Remote Safety Model)

| Bảo vệ | Hành vi |
| :--- | :--- |
| **Bảo vệ môi trường Prod** | Các remote có tên chứa `prod` mặc định được bảo vệ chống ghi đè |
| **Kiểm soát quyền hạn (Capability)** | Mỗi thao tác đều kiểm tra quyền tương ứng: `files`, `media`, `db`, `deploy` |
| **Xác thực host-key** | Chỉ áp dụng dạng opt-in trên từng remote, không ép buộc mặc định |
| **Tích hợp 1Password** | Các trường cấu hình remote hỗ trợ tham chiếu secret dạng `op://...` |

---

## Tổng quan về Đồng bộ (Sync Overview)

Lệnh `govard sync` di chuyển các file nguồn, media, và dữ liệu database giữa môi trường local và remote được chỉ định.

```bash
govard sync --source staging --destination local --full --plan
govard sync --from staging --to local --media
govard sync -s dev --db --no-noise --no-pii
govard sync -s prod --file --path app/etc/config.php
govard sync -s dev --file app/design/frontend/MyTheme
```

Tự động chọn remote `staging` nếu không khai báo `--source`, và fallback về `dev`.
Cờ `--media` đơn lẻ sẽ mặc định chạy chế độ đồng bộ media dạng `optimized`.

### Các cờ chỉ định Endpoint

| Cờ | Mô tả |
| :--- | :--- |
| `-s, --source` / `--from` | Môi trường nguồn |
| `-d, --destination` / `--to` | Môi trường đích |
| `-e, --environment` | Viết tắt của `--source` |

### Các cờ phạm vi dữ liệu (Scope Flags)

| Cờ | Những gì được đồng bộ |
| :--- | :--- |
| `-A, --full` | Tất cả mọi thứ (files + media + database) |
| `-f, --file` | Mã nguồn và các file chung |
| `-m, --media [mode]` | File media đặc thù của framework; gọi `--media` đơn lẻ mặc định là `optimized` |
| `-b, --db` | Cơ sở dữ liệu của dự án |

### Các cờ truyền tải dữ liệu (Transfer Flags)

| Cờ | Mô tả |
| :--- | :--- |
| `--plan` | Chỉ hiển thị kế hoạch thực thi rồi thoát, không chạy thực tế |
| `-D, --delete` | Xóa các file ở đích nếu không tồn tại ở nguồn |
| `-R, --resume` | Bật tính năng tiếp tục truyền tải file dở dang (mặc định: `true`) |
| `--no-resume` | Tắt tính năng tiếp tục truyền tải file dở dang |
| `-C, --no-compress` | Tắt nén dữ liệu khi chạy rsync |
| `-y, --yes` | Bỏ qua các xác nhận tương tác |
| `-p, --path` | Chỉ định một file hoặc thư mục cụ thể tương đối với thư mục gốc dự án |
| `-I, --include` | Cấu hình pattern bao gồm của rsync (có thể lặp lại nhiều lần) |
| `-X, --exclude` | Cấu hình pattern loại trừ của rsync (có thể lặp lại nhiều lần) |

::: tip
Nếu không truyền `--path`, toàn bộ thư mục gốc của dự án sẽ được đồng bộ — `govard sync` sẽ cảnh báo điều này trong kế hoạch trước khi hỏi xác nhận. Bạn cũng có thể bỏ qua `-p`/`--path` và truyền path dưới dạng tham số cuối cùng, ví dụ: `govard sync -s dev --file app/design/frontend/MyTheme`.
:::

### Các bộ lọc bảo mật cơ sở dữ liệu (Database Privacy Filters)

| Cờ | Phân loại | Các bảng bị loại trừ (Magento 2) | Laravel | WordPress |
| :--- | :--- | :--- | :--- | :--- |
| `--no-noise` | Dữ liệu rác/tạm thời | `cron_schedule`, `session`, `cache_tag`, `report_event` | `cache`, `sessions`, `failed_jobs` | `redirection_404`, `wflogs` |
| `--no-pii` | Thông tin cá nhân | `customer_entity`, `sales_order`, `quote`, `admin_user` | `users`, `password_resets` | `users`, `usermeta`, `comments` |

::: info LƯU Ý
Các bộ lọc database được tối ưu hóa sâu nhất cho Magento 2. Đối với các framework khác, các pattern an toàn mặc định sẽ được sử dụng nếu khả dụng.
:::

---

## Hành vi đồng bộ (Sync Behavior)

### Tiếp tục truyền tải dở dang (Resumable Transfers)

Đồng bộ file và media mặc định sử dụng chế độ tiếp tục của rsync (`--partial` + `--append-verify`).

```bash
govard sync -s staging --file        # mặc định có thể resume
govard sync -s staging --file --no-resume  # tắt chế độ resume
```

### Bộ lọc Include và Exclude

Các cờ `--include` và `--exclude` (hoặc `-I` và `-X`) chỉ áp dụng cho phạm vi `-f, --file` và `-m, --media` — chúng sẽ bị bỏ qua đối với phạm vi DB.

Trong lệnh `govard bootstrap`, cờ `--exclude` hoạt động như bộ lọc bỏ qua toàn cục cho cả việc clone mã nguồn và đồng bộ media sau đó.

### Loại trừ media thông minh (Chỉ Magento)

Govard tích hợp bộ lọc tự động khi đồng bộ media Magento nhằm tối ưu hóa băng thông mạng và dung lượng lưu trữ ổ đĩa.

| Phân loại | Hành vi | Các đường dẫn bị loại trừ |
| :--- | :--- | :--- |
| **None** | `--media none` | Bỏ qua hoàn toàn việc đồng bộ media |
| **Minimal** | `--media minimal` | `*.jpg`, `*.png`, `*.webp`, `*.mp4`, `*.pdf` (chỉ đồng bộ asset code) |
| **Optimized** | Chế độ mặc định | `catalog/product/` (Magento), `*/cache/*` (WordPress) |
| **Catalog** | `--media catalog` | Như optimized nhưng bao gồm ảnh sản phẩm, bỏ qua cache sản phẩm (chỉ Magento) |
| **All** | `--media all` | Đồng bộ tất cả mọi thứ (sử dụng cẩn thận) |

::: info LƯU Ý
Tất cả các chế độ ngoại trừ **All** đều tự động loại trừ các thư mục rác của framework như `tmp/`, `cache/`, và `logs/`.
:::

Sử dụng `--media all` để tải về toàn bộ media. Sử dụng `--media minimal` nếu chỉ cần CSS/JS/Fonts.

### Các đích đến được bảo vệ (Protected Destinations)

::: warning CẢNH BÁO
Sử dụng `--delete` kết hợp với `--db` sẽ hiển thị cảnh báo chính sách an toàn. Các remote thuộc môi trường Production được bảo vệ chống ghi đè và sẽ chặn các thao tác phá hủy này.
:::

### Tích hợp với lệnh `bootstrap`

Lệnh `govard bootstrap` sử dụng chung các cờ đồng bộ và bộ lọc khi khởi tạo môi trường mới:

```bash
govard bootstrap --clone -e staging --no-pii --no-noise --delete
```

---

## Giải quyết tên Remote (Remote Name Resolution)

Govard chấp nhận **bất kỳ định danh hợp lệ nào** làm tên môi trường remote. Tên chỉ được chứa chữ cái viết thường, chữ số, dấu gạch ngang hoặc gạch dưới (ví dụ: `qa`, `preprod`, `demo`, `client-uat`, `load-test`).

Các cờ remote hỗ trợ:
- Tìm kiếm chính xác theo key remote (ví dụ: `qa`, `preprod`).
- Ánh xạ các tên viết tắt quen thuộc (ví dụ: `stg` → `staging`, `live` → `production`).
- Fallback so khớp không phân biệt chữ hoa/thường.

### Các tên viết tắt quen thuộc

| Input đầu vào | Sẽ giải quyết thành |
| :--- | :--- |
| `dev`, `development`, `develop` | `development` |
| `staging`, `stage`, `stg` | `staging` |
| `prod`, `production`, `live` | `production` |
| Các tên khác (`qa`, `preprod`, `demo`, v.v.) | Giữ nguyên gốc |

### Độ ưu tiên tự động chọn (Auto-Select Priority)

Khi không khai báo remote cụ thể cho lệnh `bootstrap` hoặc `sync`, Govard sẽ tìm kiếm theo thứ tự:

1. **`staging`** (hoặc các viết tắt: `stg`, `stage`)
2. **`development`** (hoặc các viết tắt: `dev`, `develop`)

Nếu cả `staging` và `development` đều không tồn tại trong cấu hình, bạn phải sử dụng cờ `-e` để khai báo rõ ràng tên remote:

```bash
govard sync -s qa --db
govard bootstrap -e preprod --yes
```

Các lệnh sau tương đương nhau nếu có tồn tại remote `staging`:

```bash
govard sync -s stg --db
govard sync --source staging --db
govard sync --from staging --db
```

### Ví dụ về cấu hình Remote tùy chỉnh

```bash
# Thêm môi trường QA
govard remote add qa --host qa.example.com --user deploy --path /var/www/app

# Thêm môi trường pre-production
govard remote add preprod --host preprod.example.com --user deploy --path /var/www/app

# Khởi tạo từ môi trường QA (bắt buộc truyền cờ -e vì qa không thuộc diện tự động chọn)
govard bootstrap -e qa --yes

# Đồng bộ DB từ preprod
govard sync -s preprod --db --no-pii
```

### Chính sách bảo mật cho Remote tùy chỉnh

Các tên remote tùy chỉnh **không tự động được bảo vệ**. Sử dụng cờ `--protected` hoặc thiết lập cấu hình `protected: true` để kích hoạt:

```bash
govard remote add preprod --host preprod.example.com --user deploy --path /var/www/app --protected
```

Hoặc cấu hình trong `.govard.yml`:

```yaml
remotes:
  preprod:
    host: preprod.example.com
    user: deploy
    path: /var/www/app
    protected: true
```

Chỉ các remote có tên chuẩn hóa thành `prod` (như `prod`, `production`, `live`) mới được tự động bảo vệ chống ghi đè.

---

## Snapshot trên Remote (Remote Snapshots)

```bash
govard snapshot create -e staging
govard snapshot list -e staging
govard snapshot restore latest -e staging
govard snapshot delete latest -e staging
```

Các snapshot remote chạy trực tiếp lệnh `mysqldump` và `tar` trên remote server mà không cần truyền tải dữ liệu qua mạng. Dữ liệu snapshot được lưu tại thư mục `~/.govard/snapshots/` trong thư mục dự án remote.

### Truyền tải hai chiều (Bidirectional Transfer)

```bash
# Tải snapshot từ staging về local
govard snapshot pull before-upgrade -e staging

# Đẩy snapshot local lên production (bị chặn mặc định bởi chính sách bảo mật)
govard snapshot push fallback-state -e prod
```

---

## Quy trình làm việc Database Remote

### Dump dữ liệu

```bash
govard db dump                        # Dump DB local vào thư mục var/ của dự án
govard db dump -e staging             # Dump DB remote → lưu trên remote (~backup/)
govard db dump -e staging --local     # Dump DB remote → stream trực tiếp về lưu ở local var/
govard db dump --no-noise --no-pii    # Dump kèm bộ lọc bảo mật dữ liệu
```

### Import dữ liệu

```bash
govard db import --file backup.sql --drop
govard db import --stream-db -e staging --drop
```

Cờ `--stream-db` tải dữ liệu trực tiếp từ remote và import vào database local. Cờ `--drop` thực hiện reset sạch database trước khi import.

### Truy vấn, Thông tin và Giám sát trực tiếp

```bash
govard db query "SELECT COUNT(*) FROM sales_order"
govard db info -e staging
govard db top -e staging    # Giám sát các tiến trình đang chạy trên remote
```

---

## Các thao tác Remote trên Desktop

Giao diện Desktop gọi trực tiếp các api backend tương ứng với CLI:

- **Mở Database (Remote)** → Gọi lệnh `govard open db -e <remote> --client`
- **Mở terminal SSH (Remote)** → Ưu tiên các terminal gốc của Linux, fallback về giao thức `ssh://`
- **Mở SFTP (Remote)** → Ưu tiên ứng dụng FileZilla, fallback về giao thức `sftp://`

Đối với phương thức cấu hình `auth.method: ssh-agent`, ứng dụng Desktop tái sử dụng `SSH_AUTH_SOCK` và thăm dò socket tại `/run/user/<uid>/keyring/ssh` trên môi trường Linux.

---

## Quy trình khuyên dùng (Recommended Patterns)

**Xem trước kế hoạch trước khi chạy thực tế:**

```bash
govard sync --source staging --destination local --full --plan
```

**Chỉ đồng bộ một file cụ thể:**

```bash
govard sync --source prod --file --path app/etc/config.php
```

**Tạo bản dump DB local an toàn từ remote:**

```bash
govard db dump -e staging --local --no-noise --no-pii
```

**Khởi tạo dự án từ staging loại bỏ dữ liệu nhạy cảm:**

```bash
govard bootstrap --clone -e staging --no-pii --no-noise --yes
```

**Tình huống: Bootstrap Magento hiệu quả**

Giả sử bạn đang bootstrap một dự án Magento 2 lớn nhưng chỉ muốn code và một phần media:

```bash
# Clone code, sync DB không PII, và sync media KHÔNG có ảnh sản phẩm nặng (mặc định: optimized)
govard bootstrap --clone -e staging --no-pii --no-noise --yes
```

**Tình huống: Đồng bộ media có chọn lọc với exclude**

Nếu bạn cần đồng bộ media nhưng muốn bỏ qua một thư mục lớn không nằm trong smart exclude mặc định:

```bash
# Sync media từ staging nhưng bỏ qua thư mục large-assets
govard sync -s staging --media -X "large-assets/*"
```

**Tình huống: Cần ảnh sản phẩm khi bootstrap**

Nếu bạn thực sự cần ảnh sản phẩm cho công việc frontend:

```bash
govard bootstrap --clone -e staging --media all --yes
```

**Tình huống: "Cây chổi" vs "Cây nhíp"**

Kết hợp smart mode có sẵn (cây chổi) với custom exclude (cây nhíp) để kiểm soát tối đa:

```bash
# Lấy toàn bộ media nhưng bỏ qua thư mục backup cũ trên server
govard bootstrap --clone -e staging --media all -X "pub/media/backup_2022/*" --yes
```

**Tình huống: Đồng bộ siêu nhanh (Minimal)**

Nếu bạn chỉ làm frontend CSS/JS và không quan tâm ảnh:

```bash
# Chỉ đồng bộ asset tĩnh (css, js, fonts, json)
govard sync -s staging --media minimal
```

---

[Tham khảo Framework](/vi/reference/frameworks) | [SSL và Tên miền](/vi/workflows/ssl-and-domains)