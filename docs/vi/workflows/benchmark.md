# Benchmark

Các tuyên bố về hiệu năng của Govard chỉ đáng tin bằng chính harness sinh ra chúng. Trang này công bố số đo do `scripts/bench/` tạo ra, trên một máy được nêu tên, từ một commit được nêu tên, kèm đúng lệnh đã chạy. Nếu bạn chạy lại các lệnh đó trên máy tương đương, bạn sẽ nhận được con số trong cùng khoảng.

## Đo những gì

Mỗi lần chạy đo một tool trên một dự án đã chuẩn bị sẵn, bắt đầu từ trạng thái dừng:

| Chỉ số | Định nghĩa |
| :--- | :--- |
| Start | từ `govard env up` đến khi storefront trả 200, image đã ấm |
| Stop | từ `govard env down` đến khi không còn container nào của dự án |
| TTFB p50 / p95 | 50 request tuần tự vào trang chủ; mọi mẫu phải trả 200 |
| Static deploy | `bin/magento setup:static-content:deploy -f` trong container |
| DI compile | `bin/magento setup:di:compile` trong container |
| Peak memory | tổng bộ nhớ thường trú của các container thuộc dự án |

Thời gian kéo image không nằm trong số đo start. Khối provenance bên dưới ghi rõ điều đó; lần chạy có kéo image được gắn nhãn `cold`.

## Không đo gì

- Chưa đo đối thủ nào. Nhánh DDEV cần dừng Govard proxy vì `ddev-router` và Govard proxy cùng giành cổng 80 và 443; adapter đó là thay đổi riêng.
- Không đo công việc song song, độ trễ editor hay thời gian deploy.

## Kết quả

<!-- bench:start -->
No benchmark results yet.
<!-- bench:end -->

## Chạy lại

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # một lần, khoảng mười phút
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` ghi lại bảng phía trên từ các file JSON trong `OUT`. Không ai phải gõ tay con số nào, và file JSON giữ cả máy, phiên bản lẫn commit đi kèm số đo.
