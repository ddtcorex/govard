---
title: Di chuyển từ DDEV hoặc Warden sang Govard
description: Hướng dẫn từng bước di chuyển dự án từ DDEV hoặc Warden sang Govard mà không mất dữ liệu — Govard tự đọc cấu hình hiện có.
---

# Hướng dẫn di chuyển (Migration Guide) 🚚

Việc chuyển đổi dự án từ một công cụ phát triển local khác sang Govard được thiết kế để diễn ra liền mạch. Govard hiểu các cấu hình từ các công cụ phổ biến như Warden hay DDEV và có thể tự động thực hiện quá trình chuyển đổi mà không làm mất dữ liệu.

Tài liệu này hướng dẫn bạn cách di chuyển một dự án hiện có (ví dụ: từ Warden) sang Govard.

---

## Di chuyển từ DDEV sang Govard

DDEV lưu cấu hình trong `.ddev/config.yaml` và các tệp ghi đè `.ddev/config.*.yaml`.
Govard đọc tất cả theo thứ tự, tệp sau thắng, đúng như DDEV làm.

```bash
govard init --migrate-from ddev
```

Lệnh in ra một dòng cho mỗi khoá đã xử lý, nên xem được kết quả trước khi khởi
động bất cứ thứ gì:

- `mapped ...` cho giá trị đã mang sang `.govard.yml`;
- `defaulted ...` cho service mà tệp DDEV không nhắc tới, giữ mặc định của profile
  thay vì bị tắt đi;
- `not migrated ...` cho khoá Govard không có tương đương, kèm lý do.

### Những gì được mang sang

| Khoá DDEV | Trường Govard |
| :--- | :--- |
| `name` | `project_name` và tên miền `*.test` |
| `type` | `framework` |
| `docroot` | `stack.web_root` |
| `php_version`, `nodejs_version`, `composer_version` | các trường `stack.*` tương ứng |
| `webserver_type` | `stack.services.web_server` |
| `database.type`, `database.version` | `stack.services.db`, `stack.db_version` |
| `additional_hostnames` | `extra_domains`, mở rộng thành `<host>.<project>.test` |
| `additional_fqdns` | `extra_domains`, giữ nguyên |
| `xdebug_enabled` | `stack.features.xdebug` |

### Những gì được báo cáo chứ không áp dụng

`upload_dirs`, `webimage_extra_packages`, `omit_containers`, `hooks`,
`performance_mode`, `mutagen_enabled`, `web_extra_exposed_ports`,
`database_snapshot`, `timeout`, `disable_settings_management`, cổng router và
`composer_root` đều được nêu tên trong output kèm lý do không có tương đương.
Không có khoá nào bị bỏ im lặng.

Những service DDEV không quản lý (`search`, `cache`, `queue`, `varnish`) giữ mặc
định của profile, nên dự án Magento sau khi migrate vẫn có OpenSearch, Redis và
RabbitMQ. Kiểm tra bằng `govard doctor`: migrate đúng thì runtime được báo là đã
đồng bộ với profile, thay vì cảnh báo search và cache đang là `none`.

Một ngoại lệ có chủ ý: profile của framework ghim phiên bản PHP, Node và Composer,
nên ghim của profile có thể đè lên phiên bản DDEV đã khai. Việc đè đó được in
thành dòng drift (`Synced stack.node_version 20→24`) chứ không xảy ra im lặng.

## Di chuyển từ Warden sang Govard

Khi chuyển đổi một dự án từ Warden, các mục tiêu chính là:
1. Ánh xạ các phiên bản PHP/Node/Database tương ứng sang hệ sinh thái của Govard (`.govard.yml`).
2. Đảm bảo không mất dữ liệu bằng cách sao chép Docker volume Database.
3. Dọn dẹp các tệp tạm thời/cache chứa các đường dẫn bị hardcode.

### Bước 1: Dừng môi trường Warden

Trước khi bắt đầu di chuyển, hãy đảm bảo môi trường cũ đã được dừng hẳn để tránh ghi đè làm hỏng dữ liệu trong quá trình đồng bộ database.

```bash
cd /path/to/your/project
warden env stop
```

### Bước 2: Xóa Caches của ứng dụng

Mặc dù đường dẫn ứng dụng của Warden và Govard trong container có thể giống nhau (ví dụ `/var/www/html`), nhưng Redis hoặc file-based cache sinh ra khi chạy Warden có thể gây lỗi nếu chúng kỳ vọng các biến môi trường nội bộ hoặc trạng thái cấu hình khác nhau.

Đối với dự án **Magento 2**, hãy xóa các thư mục code/cache được sinh ra tự động:

```bash
rm -rf var/cache/* var/page_cache/* generated/code/*
```

### Bước 3: Chạy lệnh di chuyển tự động của Govard

Chạy lệnh `govard init` với cờ `--migrate-from warden`.

Govard sẽ parse cấu hình file `.env` (các biến của Warden) và `.warden/warden-env.yml` để tự động đề xuất profile runtime phù hợp nhất.
Đối với các dự án Magento 2, Magento 1 và OpenMage, Govard cũng di chuyển giá trị `WARDEN_TABLE_PREFIX` của Warden vào `.govard.yml` dưới dạng `table_prefix`.

```bash
govard init --migrate-from warden
```

**Trong quá trình này:**
1. Govard sẽ tạo file `.govard.yml` ánh xạ các cấu hình Warden của bạn sang stack của Govard.
2. Nó sẽ tự động phát hiện volume database Warden của bạn (thông thường là `<project>_dbdata`).
3. Nếu `WARDEN_TABLE_PREFIX` được thiết lập, Govard sẽ lưu giữ nó dưới dạng `table_prefix` để các bảng Magento như `demo_core_config_data` tiếp tục hoạt động bình thường.
4. Bạn sẽ nhận được prompt hỏi:
   > `Do you want to clone the existing database volume from Warden ('myproject_dbdata') into Govard? [y/N]`
5. Nhập **`y` (Yes)**. Govard sẽ tự động sao chép dữ liệu raw SQL sang một docker volume cô lập riêng của nó.

*(Lưu ý: Việc clone database sử dụng lệnh raw `cp -a` được mount bên trong Docker, giúp đảm bảo các file permission được giữ nguyên hoàn toàn bảo mật và quá trình này hoàn tất chỉ trong vài giây, bỏ qua các pipeline mysqldump chậm chạp).*

### Bước 4: Khởi động môi trường mới

Sau khi các cấu hình đã được phân tích và database được clone an toàn, bạn chỉ cần khởi động Govard:

```bash
govard env up
```

Đợi cho đến khi bạn thấy thông báo `✅ php runtime is ready`.

### Bước 5: Đồng bộ sau khi di chuyển (Tùy chọn)

Vì Govard kết nối trực tiếp qua `app/etc/env.php` (trên Magento), bạn nên cập nhật tự động các chuỗi kết nối bằng injector cấu hình tích hợp sẵn của Govard:

```bash
govard config auto
```

Nếu bạn sử dụng Elasticsearch/OpenSearch, hãy nhớ rằng các volume index tìm kiếm **không** được di chuyển tự động (chỉ di chuyển các cơ sở dữ liệu quan hệ chính như MariaDB/MySQL). Bạn nên tiến hành re-index lại dữ liệu thông qua framework của bạn:

```bash
govard tool magento indexer:reindex
```

### Bước 6: Dọn dẹp Warden (Tùy chọn)

Sau khi bạn xác nhận tên miền `.test` hoạt động mượt mà và Admin Panel tải cực nhanh, bạn có thể gỡ bỏ hoàn toàn Warden để giải phóng dung lượng ổ cứng SSD:

```bash
warden env down -v
```

---

## Di chuyển Database thủ công

Nếu bạn đã bỏ qua prompt gợi ý tự động trong khi chạy `govard init`, bạn vẫn luôn có thể clone volume database thủ công vào bất kỳ lúc nào!

Hãy đảm bảo container DB của Govard đã được dừng trước:

```bash
govard env stop db
```

Sau đó chạy lệnh clone thủ công, chỉ định tên Docker volume cũ của bạn:

```bash
govard db clone-volume <source_volume_name>
```

Ví dụ đối với Warden:
```bash
govard db clone-volume warden_myproject_dbdata
```

Ví dụ đối với DDEV:
```bash
govard db clone-volume ddev-myproject-db
```

---

**[← Dự án đầu tiên](/vi/getting-started/getting-started)** | **[Remote & Đồng bộ →](/vi/workflows/remotes-and-sync)**