---
title: Govard và Warden
description: Govard và Warden khác nhau ở đâu với một dự án Magento; mặc định mỗi bên khởi động gì, bộ cài đụng vào máy host những gì, và chuyển dự án sang bằng cách nào.
---

# Govard và Warden

Cả hai đều dựng một stack Magento trong container và trỏ nó tới một tên miền
local. Warden ra đời trước Govard: nó ghép một stack cố định do `env-init` sinh
ra với router traefik, và giữ cấu hình trong các tệp biến môi trường. Govard đi
theo hướng khác: một binary duy nhất, proxy Caddy, và một profile khai báo trong
`.govard.yml`. Trang này nêu các khác biệt; con số nằm ở
[trang benchmark](/vi/workflows/benchmark) để chúng không tách rời khỏi phương
pháp đã tạo ra chúng.

## Mặc định khởi động những gì

| | Govard | Warden |
| :--- | :--- | :--- |
| Web + database | có | có |
| Varnish, Redis, OpenSearch, RabbitMQ | nằm trong profile mặc định | nằm trong stack mà `env-init` ghi ra |
| Router | Caddy trên 80 và 443, dùng chung cho mọi dự án Govard | traefik trên 80 và 443, không đổi cổng được |
| Cấu hình | `.govard.yml` | `.env` và `.warden/warden-env.yml` |
| Cài CLI | một binary vào `~/.local/bin`, không cần quyền | cần root một lần: đổi chủ một khoá SSH tunnel, sửa `/etc/ssh/ssh_config`, và cài CA vào trust store của hệ thống |

Dòng cuối là khác biệt mà một đội cảm nhận đầu tiên, và cũng là lý do nhánh
benchmark phải chạy CLI của Warden trong container thay vì xin sudo trên máy đo.

## Hai bên đo ra sao

Cùng một fixture, cùng một máy, cùng một khung giờ, mỗi bên ba lần sau một lần
warm-up bị loại bỏ. Bảng đầy đủ và các khoảng min-max nằm ở
[trang benchmark](/vi/workflows/benchmark).

| Chỉ số | Govard | Warden | Đọc thế nào |
| :--- | ---: | ---: | :--- |
| Start, tới khi storefront trả lời | 11,0 s | 11,6 s | gần nhau |
| Stop | 11,2 s | 3,7 s | Warden nhanh hơn khoảng ba lần |
| TTFB p50, cache như mặc định | 14,2 ms | 13,5 ms | gần nhau |
| TTFB p50, bỏ qua cache | 250,6 ms | 495,7 ms | Govard nhanh hơn khoảng hai lần ở lượt render nguội |
| Static deploy | 4,3 s | 7,7 s | Govard nhanh hơn |
| DI compile | 21,0 s | 25,5 s | Govard nhanh hơn |
| Bộ nhớ đỉnh, trạng thái ổn định | 1.601 MB | 1.020 MB | Warden dùng ít hơn |
| Chi phí wrapper CLI | 46 ms | 213 ms | xem lưu ý bên dưới |

Hai dòng cần lưu ý trước khi trích dẫn:

- **Stop.** `env down` của Govard mất 11,2 s so với 3,7 s của Warden. Nó dọn
  nhiều thứ hơn, và trang này không tô hồng con số đó.
- **Chi phí CLI.** 213 ms của Warden bao gồm việc chạy CLI trong container, cách
  duy nhất để nhánh này hoạt động mà không cần root trên máy đo. Cài native sẽ
  không phải trả khoản đó.

Phần không bằng nhau: traefik của Warden giữ cổng 80 và 443, nên proxy của Govard
được dừng trong lúc đo nhánh Warden, và container của nó được gọi trực tiếp. Cả
hai nhánh đều phục vụ qua Varnish, nên dòng bỏ qua cache so sánh cùng một dạng
request ở hai bên.

## Chuyển dự án từ Warden sang Govard

```bash
govard init --migrate-from warden
```

Bộ nhập đọc `.env` và `.warden/warden-env.yml`, map tên dự án, framework, phiên
bản PHP, Node, Composer, MySQL, Varnish, web root và table prefix, rồi hỏi có
clone volume database của Warden sang volume của Govard hay không. Sau đó
`govard env up`. Hướng dẫn đầy đủ, gồm cả bước xoá cache để tránh Redis và file
cache cũ trong môi trường mới, nằm ở
[Chuyển từ DDEV hoặc Warden](/vi/getting-started/migration).

## Khi nào nên chọn Warden

- Đội đã vận hành một môi trường Warden ổn định, và chuyển đổi tốn hơn phần thu
  lại.
- Bạn muốn stack và router đến từ một môi trường cố định, giữ bằng shell và tệp
  env, và việc cài bằng root không phải vấn đề.
- Bạn phụ thuộc vào các env template sẵn có của Warden cho những stack mà Govard
  chưa đóng gói.

## Khi nào nên chọn Govard

- Bạn muốn cài đặt không cần quyền và không phải sửa tệp hệ thống để có môi
  trường local.
- Deploy, remote với `prod` chặn ghi, audit, snapshot và desktop app nằm trong
  cùng công cụ dùng để phát triển.
- Bạn cũng từng dùng DDEV, nên một bộ nhập phủ cả hai, và nó báo từng khoá là đã
  map, dùng mặc định hay bị bỏ qua thay vì bỏ im lặng.
- Thời gian render nguội quan trọng hơn bộ nhớ ở trạng thái ổn định, theo các số
  đo phía trên.
