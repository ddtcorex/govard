---
title: Govard và Lando
description: Vì sao trang benchmark không có cột Lando, Lando hiện có gì cho Magento, và khi nào nên chọn bên nào.
---

# Govard và Lando

Lando là công cụ phát triển local đa dụng: Landofile mô tả service và tooling, một
proxy định tuyến tên miền local, và recipe gói sẵn những stack phổ biến. Govard
hẹp hơn: một CLI, một stack khai báo theo framework, và profile Magento đóng gói
sẵn Varnish, Redis, OpenSearch, RabbitMQ cùng phần nối `env.php`. Trang này giải
thích vì sao [trang benchmark](/vi/workflows/benchmark) không có cột Lando, và cần
gì để cột đó xuất hiện.

## Vì sao không có cột đo

Danh mục plugin chính thức của Lando (xem ngày 2026-11-04, Lando 3.26.9) có recipe
cho Drupal, Backdrop, Joomla, Laravel, Symfony, WordPress, LAMP, LEMP, MEAN và
Dotnet. Không có recipe Magento. Vì thế một dự án Magento trên Lando là Landofile
do chính bạn viết: recipe LAMP hoặc LEMP, cộng plugin Varnish, Redis,
Elasticsearch, MariaDB, cộng phần nối môi trường và cron riêng cho Magento.

Đo cấu hình đó là đo người viết nó, không phải đo công cụ. Hai người dùng Lando có
thể tạo ra hai stack khác nhau nhiều hơn mức Govard và DDEV khác nhau, nên một con
số "Lando" duy nhất sẽ là phát biểu về Landofile của chúng tôi nhưng mang tên công
cụ. Trang benchmark chỉ công bố thứ tái lập được từ một commit và một máy cụ thể;
đây không phải trường hợp đó.

Điều sẽ thêm được cột: một recipe Magento chính thức có mặc định nói rõ được, hoặc
một recipe cộng đồng có phiên bản công khai để giữ cố định. Khi đó phép đo có định
nghĩa mà người đọc tự chạy lại được.

## Những gì so được mà không cần đồng hồ

| | Govard | Lando |
| :--- | :--- | :--- |
| Hỗ trợ Magento | profile đóng gói sẵn: Varnish, Redis, OpenSearch, RabbitMQ, container `php-debug` riêng, nối `env.php` | stack tự ghép từ LAMP hoặc LEMP cộng plugin service |
| Cấu hình | `.govard.yml` | Landofile: `recipe`, `services`, `tooling`, `proxy` |
| Tên miền local | `*.test` với CA cục bộ | tên miền qua proxy của Lando |
| Cổng proxy | 80 và 443, dùng chung cho mọi dự án Govard | 80 và 443 |
| Nhập dự án có sẵn | `govard init --migrate-from ddev` và `--migrate-from warden`, mỗi khoá đều được báo là đã map, dùng mặc định hay bỏ qua | không có gì để nhập từ đây; Landofile không có tương đương bên Govard |

## Chuyển dự án từ Lando sang Govard

Không có `--migrate-from lando`, và hai mô hình không khớp để tự động hoá:
Landofile nêu tên service và image, còn `.govard.yml` nêu framework và profile, rồi
framework quyết định thứ được chạy.

Đường thủ công ngắn:

```bash
govard init --framework magento2
```

Sau đó đặt các phiên bản mà Landofile đã khai báo vào `.govard.yml`
(`stack.php_version`, `stack.db_version`), chạy `govard env up`, rồi so những gì
được khởi động với những gì Landofile định nghĩa. Nếu thiếu một service và nó chặn
công việc, đó đúng là việc mà
[form stack request](https://github.com/ddtcorex/govard/issues/new?template=request.yml)
sinh ra để giải quyết.

## Khi nào nên chọn Lando

- Đội làm nhiều thứ cùng lúc: Drupal, WordPress và Laravel cạnh nhau, nơi một quy
  ước Landofile chung đáng giá hơn chiều sâu theo từng framework.
- Bạn phụ thuộc vào một tích hợp hosting mà Lando có sẵn dưới dạng plugin, như
  Pantheon, Acquia, Platform.sh hay Lagoon.
- Bạn muốn tự mô tả stack và thích ghép service hơn là nhận một profile do người
  khác đóng gói.

## Khi nào nên chọn Govard

- Dự án là Magento hoặc Mage-OS và bạn muốn stack chạy được thật, gồm Varnish,
  search engine và cache, mà không phải tự ghép.
- Deploy, remote có `prod` chặn ghi, audit và snapshot nên nằm trong cùng công cụ
  dùng để phát triển.
- Bạn đến từ DDEV hoặc Warden, nơi bộ nhập đã làm phần nhàm chán và báo lại những
  gì nó không mang theo được.
