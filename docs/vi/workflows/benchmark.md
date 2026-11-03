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

- Không đo công việc song song, độ trễ editor hay thời gian deploy.
- Hai nhánh không chạy cùng một stack. Govard chạy profile mặc định của dự án, gồm Varnish, Redis và OpenSearch; DDEV chạy mặc định Magento của nó, chỉ web và database. Phần lớn chênh lệch bộ nhớ đến từ đó, và trang này nói rõ thay vì gộp thành một điểm số.
- DDEV chạy trên cổng router khác (8080/8443) để không giành 80/443 với Govard proxy; request tới DDEV vì thế đi qua cổng khác request tới Govard.

## Kết quả

<!-- bench:start -->

| Metric | govard | ddev |
| --- | --- | --- |
| Start (up to first 200) | 12626.5 ms | 15041 ms |
| Stop | 11349.5 ms | 1261 ms |
| TTFB p50 | 16.8 ms | 91.4 ms |
| TTFB p95 | 21.3 ms | 108.1 ms |
| Static deploy | 5898 ms | 9571.5 ms |
| DI compile | 35090 ms | 17133.5 ms |
| Peak memory | 1624.5 MB | 506.5 MB |

- **govard**: 2 run(s), images warm (pulls excluded). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-5-gf71a9892 at commit f71a9892, measured 2026-11-03T06:46:03Z.
- **ddev**: 2 run(s), images warm (pulls excluded); stack: web+db (ddev magento default; no varnish, redis or search). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-5-gf71a9892 at commit f71a9892, measured 2026-11-03T06:30:36Z.

<!-- bench:end -->

## Chạy lại

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # một lần, khoảng mười phút
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` ghi lại bảng phía trên từ các file JSON trong `OUT`. Không ai phải gõ tay con số nào, và file JSON giữ cả máy, phiên bản lẫn commit đi kèm số đo.
