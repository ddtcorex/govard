# Govard và DDEV

Cả hai công cụ đều chạy một dự án Magento trong container và cấp cho nó một tên
miền local. Chúng khác nhau ở thứ được khởi động, ở phần tự động hoá nằm ngoài
container, và ở cách cấu hình dự án. Trang này nêu các khác biệt; con số nằm ở
[trang benchmark](/vi/workflows/benchmark) để chúng không tách rời khỏi phương
pháp đã tạo ra chúng.

## Mặc định khởi động những gì

| | Govard | DDEV |
| :--- | :--- | :--- |
| Web + database | có | có |
| Varnish, Redis, OpenSearch, RabbitMQ | nằm trong profile mặc định | phải cài add-on |
| Cổng router | 80 và 443, dùng chung cho mọi dự án Govard | mặc định 80 và 443, đổi được |
| Tệp cấu hình | `.govard.yml` | `.ddev/config.yaml` |

Khác biệt này là thứ đầu tiên cần đọc trong bất kỳ so sánh nào: một stack có
Varnish và search engine không phải là cùng một sản phẩm với stack không có, và
trang benchmark nói rõ mỗi nhánh chạy những service nào.

## Govard làm được gì ngoài phạm vi so sánh này

- `govard deploy` có sandbox rehearsal, rollback và resume.
- Remote có phân quyền theo phạm vi, `prod` chặn ghi.
- `govard audit` lưu phiên, có profiler.
- SSH gateway dùng chung, snapshot, và desktop app trên cùng một engine.

Những thứ đó không được đo ở đây; chúng có hoặc không có.

## Chuyển dự án từ DDEV sang Govard

```bash
govard init --migrate-from ddev
```

Bộ nhập đọc `.ddev/config.yaml` và báo cáo từng khoá đã map, đã dùng mặc định,
hay bị bỏ qua. Không có khoá nào bị bỏ im lặng.

## Khi nào nên chọn DDEV

- Windows: hỗ trợ native của DDEV đã chín; đường được hỗ trợ của Govard là WSL2
  (xem [Windows và WSL2](/vi/more/windows)).
- Hệ add-on lớn cho những service Govard không đóng gói sẵn.
- Đội đã chuẩn hoá trên DDEV và đang hài lòng. Chuyển đổi tốn hơn phần thu lại,
  trừ khi đang có vấn đề thật.

## Khi nào nên chọn Govard

- Chiều sâu Magento và Mage-OS: nối `env.php`, table prefix, Varnish, nhiều
  store domain, và container `php-debug` không làm chậm container thường.
- Deploy và việc remote nằm trong cùng công cụ dùng để phát triển.
- CA cục bộ và tên miền `*.test` không cần thiết lập từng dự án.
