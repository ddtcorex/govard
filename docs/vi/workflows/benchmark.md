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

Hai nhánh không chạy cùng một stack, và hai dòng trong bảng đo lớp bọc lệnh chứ
không đo công việc thật. Đọc phần này trước khi so hai cột; không câu nào ở đây
nói tool nào nhanh hơn.

**Bộ container.** Govard dựng bảy container (nginx, php, php-debug, Varnish,
Redis, OpenSearch, MariaDB). DDEV dựng hai (web, database) cộng router dùng
chung. Bộ nhớ đỉnh gần như bằng đúng phần chênh đó, và riêng search engine đã
chiếm phần lớn.

**Page cache.** Govard đặt Varnish phía trước, nên request vào storefront thường
là cache hit: response mang `age` và `x-magento-cache-debug: HIT`. DDEV không có
Varnish nên cùng request đó phải render PHP. Vì vậy dòng TTFB so một lần trúng
proxy với một lần render PHP, không phải hai lần render PHP. Muốn ép miss trên
Govard phải ban cache hoặc dùng session cookie, vì VCL ở đây bỏ qua query string
khi tính cache key.

**Chi phí lớp bọc CLI.** Static deploy và DI compile chạy qua lớp exec của từng
tool. Đo trực tiếp trên máy này: `docker exec` thô 1,1 s, `ddev exec` 2,4 đến 3,2
s, `govard tool` 5,1 đến 5,4 s. Phần lớn dòng static deploy là chi phí wrapper,
không phải Magento.

**Dùng chung một cây mã nguồn.** Hai nhánh đo trên cùng một thư mục, nên
`pub/static` đã được bên chạy trước sinh sẵn, và `env up` của Govard xoá
`generated/code` cùng `var/cache`. Đó là lý do lần deploy đầu của Govard mất 58 s
còn các lần sau 5 s, trong khi DDEV bắt đầu ở trạng thái ấm. Harness phải tách
mỗi nhánh một bản sao trước khi dòng này so sánh được.

**Nhiễu.** Cùng một trang, mẫu TTFB trên máy này dao động 16 đến 111 ms trong lúc
khảo sát. Hai lượt hai mẫu không đủ tách một khác biệt 1,5 lần khỏi nhiễu.

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
| Start (up to first 200) | 8958 ms (7209-10707 ms) | 22113 ms (17880-26346 ms) |
| Stop | 11348.5 ms (11188-11509 ms) | 2519.5 ms (1604-3435 ms) |
| TTFB p50 | 17.5 ms (16.4-18.6 ms) | 108.6 ms (97.2-120.0 ms) |
| TTFB p95 | 21.4 ms (20.6-22.2 ms) | 144.5 ms (133.9-155.0 ms) |
| Static deploy | 16433.5 ms (5015-27852 ms) | 2771.5 ms (2441-3102 ms) |
| DI compile | 24425.5 ms (18888-29963 ms) | 23280.5 ms (20697-25864 ms) |
| Peak memory | 1695.5 MB (1644-1747 MB) | 430 MB (416-444 MB) |

- **govard**: 2 run(s), images warm (pulls excluded). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-6-g85a08e53 at commit 85a08e53, measured 2026-11-03T07:22:37Z.
- **ddev**: 2 run(s), images warm (pulls excluded); stack: web+db (ddev magento default; no varnish, redis or search). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-6-g85a08e53 at commit 85a08e53, measured 2026-11-03T07:30:39Z.

<!-- bench:end -->

## Chạy lại

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # một lần, khoảng mười phút
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` ghi lại bảng phía trên từ các file JSON trong `OUT`. Không ai phải gõ tay con số nào, và file JSON giữ cả máy, phiên bản lẫn commit đi kèm số đo.
