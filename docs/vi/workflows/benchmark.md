---
title: Benchmark của Govard
description: Số đo hiệu năng Govard có thể tái lập từ harness nằm trong repo; start, stop, thời gian phản hồi đầu tiên, static deploy, DI compile và bộ nhớ, đo trên một máy được nêu tên.
---

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

## Vì sao các con số khác nhau

Mỗi nhánh đo trên bản sao riêng của cùng một dự án và cùng một database, và một
lượt chạy đầu bị bỏ để làm nóng. Dù vậy, hai cột vẫn không đo cùng một công việc.
Đọc phần này trước khi kết luận.

**Bộ container.** Govard dựng bảy container (nginx, php, php-debug, Varnish,
Redis, OpenSearch, MariaDB). DDEV dựng hai (web, database) cộng router dùng
chung. Bộ nhớ đỉnh gần như bằng đúng phần chênh đó, và riêng search engine đã
chiếm phần lớn.

**Page cache, và điều gì xảy ra khi không có nó.** Govard đặt Varnish phía trước
nên request bình thường là cache hit: response mang `age` và
`x-magento-cache-debug: HIT`. DDEV không có cache ở reverse proxy nên cùng request
đó phải render PHP. Đo cả hai chiều trên máy này thì hướng đảo nhau:

- để cache như mặc định: Govard 17 ms, DDEV 174 ms;
- xoá cache trước mỗi mẫu (ban Varnish ở một bên, `cache:flush full_page` ở bên
  kia): Govard 342 ms, DDEV 185 ms.

Nghĩa là Govard thắng ở request đã có cache và thua ở lần render lạnh, vì đường
đi của nó nhiều hơn một chặng (Varnish rồi nginx rồi php-fpm) so với nginx cộng
php-fpm của DDEV. Cái nào quan trọng tuỳ vào việc lượt truy cập của bạn có trúng
cache ấm hay không.

**Chi phí lớp bọc CLI.** Chạy cùng một lệnh vô nghĩa (`php -r 'echo 1;'`) trực
tiếp và qua lớp bọc của từng tool: `govard tool` 40 ms, `ddev exec` 331 ms. Dòng
này đính chính một khẳng định trước đó trên trang rằng lớp bọc của Govard cộng
thêm khoảng bốn giây: con số đó đến từ một lệnh `docker exec` thô đã thất bại
trong 1,1 giây chứ không hề chạy. Đo một lệnh rỗng theo cả hai đường là cách
chốt đúng, và harness giờ làm việc đó ở mọi lượt.

**Static deploy và DI compile.** Với cây mã nguồn riêng và một lượt làm nóng bị
bỏ, static deploy là 4,9 s cho Govard và 5,1 s cho DDEV, DI compile là 31,8 s so
với 46,0 s. Các con số công bố trước đó (16,4 s so với 2,8 s) là lỗi đo: hai
nhánh dùng chung một cây, nên nhánh chạy trước sinh `pub/static` cho nhánh sau,
và `env up` của Govard xoá `generated/code` cùng `var/cache` khiến lần deploy đầu
phải sinh lại toàn bộ.

**Nhiễu.** Khoảng min-max được in cạnh mỗi trung vị. Thời gian start của DDEV dao
động 21,5 s đến 51,3 s qua ba lượt trên máy đang làm việc khác; một lượt đơn lẻ
là nhiễu.

## Đọc các con số này thế nào

Một lần đo trên máy đang làm việc khác chỉ là nhiễu: hai lần chạy cùng harness
trên cùng máy đã lệch nhau gấp đôi ở chỉ số biên dịch, và hơn năm lần ở static
deploy của govard. Hãy chạy ít nhất ba lần và công bố trung vị, đúng như bảng
bên dưới; khoảng trong ngoặc là nhỏ nhất và lớn nhất quan sát được, khoảng càng
rộng nghĩa là máy đang bận chứ không phải tool chậm. Coi các con số là bậc độ
lượng, không phải đồng hồ bấm giây.

## Kết quả

<!-- bench:start -->

| Metric | govard | ddev |
| --- | --- | --- |
| Start (up to first 200) | 14713 ms (14667-15316 ms) | 26061 ms (21469-51303 ms) |
| Stop | 11262 ms (5329-11828 ms) | 2641 ms (2341-4987 ms) |
| TTFB p50 (cached) | 16.7 ms (15.8-21.8 ms) | 173.9 ms (114.7-196.0 ms) |
| TTFB p95 (cached) | 21.5 ms (17.5-31.7 ms) | 266.2 ms (147.6-289.5 ms) |
| TTFB p50 (cache bypassed) | 341.6 ms (244.3-405.6 ms) | 184.9 ms (107.3-221.0 ms) |
| TTFB p95 (cache bypassed) | 343.7 ms (255.3-407.9 ms) | 251.1 ms (109.9-286.2 ms) |
| Static deploy | 4875 ms (4608-6156 ms) | 5055 ms (4381-10160 ms) |
| DI compile | 31837 ms (31816-33068 ms) | 46028 ms (35882-73548 ms) |
| Peak memory | 1696 MB (1666-1788 MB) | 446 MB (444-463 MB) |
| CLI wrapper overhead | 40 ms (34-95 ms) | 331 ms (152-812 ms) |

- **govard**: 3 run(s), images warm (pulls excluded); stack: nginx, php, php-debug, varnish, redis, opensearch, mariadb. 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-10-gea2e9648-dirty at commit ea2e9648, measured 2026-11-03T09:52:08Z.
- **ddev**: 3 run(s), images warm (pulls excluded); stack: web (nginx + php-fpm), database; ddev magento default, no varnish, redis or search. 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-10-gea2e9648-dirty at commit ea2e9648, measured 2026-11-03T10:12:53Z.

<!-- bench:end -->

## Chạy lại

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # một lần, khoảng mười phút
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` ghi lại bảng phía trên từ các file JSON trong `OUT`. Không ai phải gõ tay con số nào, và file JSON giữ cả máy, phiên bản lẫn commit đi kèm số đo.
