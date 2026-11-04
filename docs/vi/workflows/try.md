---
title: Thử Govard không cần dự án
description: Một lệnh tạo storefront Mage-OS dùng một lần, chạy sau Govard proxy và tự xoá khi bạn thoát, kèm cờ, chi phí và giới hạn.
---

# Thử Govard không cần dự án

```bash
govard try
```

Chỉ vậy là đủ. Lệnh tạo một dự án Mage-OS tạm trong thư mục riêng, khởi động toàn
bộ stack mặc định, cài dữ liệu mẫu, in ra địa chỉ storefront và admin, rồi xoá sạch
khi bạn thoát. Thư mục hiện tại của bạn không bị đụng tới, và không cần tài khoản
Magento nào.

![govard try chạy đến khi storefront phục vụ](/try-demo.gif)

Ghi ngày 2026-11-04 bằng asciinema trên chính máy dựng dự án này. Bản ghi là thật,
chỉ cắt phần cài dependency ở giữa, chỗ có dòng
`... 7 minutes of dependency install and Magento setup ...`; thời gian chờ được nén
còn tối đa một giây. Lượt chạy đầy đủ mất 7 phút 45 giây.

![Storefront Luma của Mage-OS do govard try phục vụ](/try-storefront.png)

Storefront mà lượt chạy đó tạo ra, chụp bằng trình duyệt: Mage-OS kèm dữ liệu mẫu,
phục vụ qua HTTPS trên tên miền `*.test` bởi Govard proxy.

## Một lượt chạy làm gì

1. Tạo workspace thử và viết `.govard.yml` cho Mage-OS.
2. Khởi động stack mặc định: nginx, PHP-FPM, MariaDB, Redis, OpenSearch và Varnish,
   cùng container PHP debug mà dự án luôn có.
3. Cài Magento kèm dữ liệu mẫu, rồi đặt base URL cho storefront.
4. Đăng ký tên miền với Govard proxy và chờ storefront trả lời bằng chứng chỉ do CA
   cục bộ ký.
5. In địa chỉ và tài khoản admin, chờ bạn, rồi xoá workspace nếu bạn không yêu cầu giữ.

## Chi phí

Đo trên máy đã có sẵn image, ngày 2026-11-04:

| | Giá trị |
| :--- | :--- |
| Từ lúc chạy đến khi storefront phục vụ | 8 phút 6 giây |
| Dung lượng workspace | 1,1 GB |
| Cổng | 80 và 443, qua Govard proxy |
| Độ trễ sau lần truy cập đầu | vài chục mili giây, mức trúng Varnish đo ở [trang benchmark](/vi/workflows/benchmark) |

Lần chạy đầu còn phải tải image về nên sẽ lâu hơn. Các lần sau dùng lại image đã có.

## Các cờ

| Cờ | Tác dụng |
| :--- | :--- |
| `--keep` | Giữ workspace khi kết thúc và in đường dẫn để mở lại sau |
| `--no-sample` | Bỏ bước cài dữ liệu mẫu, vốn chiếm phần lớn tám phút |
| `--dir <path>` | Dùng thư mục này làm workspace thay vì thư mục tự sinh |
| `--setup` | Chạy thiết lập một lần đầu tiên: proxy, CA cục bộ và mục hosts |
| `--timeout <duration>` | Thời gian chờ storefront trả lời, mặc định 20 phút |
| `--no-open` | Không mở storefront trên trình duyệt |

## Yêu cầu

- Docker đang chạy trên máy.
- Cổng 80 và 443 còn trống. Govard proxy giữ hai cổng này và phục vụ mọi dự án, nên
  phải dừng thứ khác đang chiếm chúng.
- CA cục bộ đã được tin cậy và tên miền phân giải được, `govard try --setup` làm việc
  này trong một lần chạy có quyền. Thiếu những thứ đó thì lệnh dừng trước khi tải gì
  và nói rõ phần nào còn thiếu.

## Khi lỗi

- **Cổng 80 hoặc 443 bị chiếm.** Lệnh báo proxy không khởi động được; dừng tiến trình
  đang giữ cổng, hoặc xem `govard proxy status`.
- **Storefront không trả lời.** Lệnh in phần cuối log bootstrap và giữ workspace, nên
  có thể vào đó chạy `govard env up` để xem container.
- **Chứng chỉ không được tin cậy.** Chạy `govard try --setup` một lần; nó cài CA và
  mục hosts.

## Sau khi thử xong

Không có `--keep` thì workspace và container bị xoá. Có `--keep` thì đường dẫn được
in ra; vào đó dùng `govard env up` và `govard env down` như mọi dự án khác, hoặc xoá
thư mục là xong.

Nếu thấy đáng giữ, hãy tạo dự án đúng cách:

```bash
govard init          # nhận diện framework, viết .govard.yml
govard env up        # khởi động
```

## Ghi lại demo

Bản ghi trên được làm bằng [asciinema](https://asciinema.org/) và
[agg](https://github.com/asciinema/agg):

```bash
asciinema rec run.cast --idle-time-limit 1 --cols 100 --rows 28 \
  --command "govard try --keep --no-open"
agg --speed 2 --font-size 14 --theme asciinema --cols 100 --rows 28 \
  run.cast try-demo.gif
```

Tệp cast là nguồn gốc: giữ nó cùng bản ghi để người đọc phát lại đúng lượt chạy đó.
