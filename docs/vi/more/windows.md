---
title: Windows và WSL2
description: Cái gì chạy được khi dùng Govard trên Windows, cái gì không, và vì sao WSL2 là đường được hỗ trợ.
---

# Windows và WSL2

Govard được phát triển và kiểm thử trên Linux, và chạy được trên macOS. Trên
Windows, đường đi được hỗ trợ là WSL2: cài Govard bên trong bản phân phối, không
cài trên máy Windows gốc.

Trang này nói rõ cái gì được hỗ trợ, cái gì không, và vì sao, để bạn quyết định
trước khi mất cả buổi chiều.

## Phần chạy được trên Windows gốc

- **Binary có build cho Windows.** GoReleaser phát hành archive
  `windows/amd64` và `windows/arm64` ở mỗi bản release.
- **npm cài được.** `npm i -g @ddtcorex/govard` tải archive đúng nền tảng và
  giải nén bằng PowerShell; lệnh `govard` sau đó chạy được.
- **Các lệnh chỉ làm việc trên máy, không đụng container** chạy được:
  `govard version`, `govard capabilities`, `govard doctor`, `govard help`.

## Phần không chạy được

- **CI không kiểm thử Windows.** Mọi workflow đều chạy trên `ubuntu-latest`, nên
  bản Windows chỉ được biên dịch chứ chưa được chạy thử. Hãy coi nó là chưa được
  hỗ trợ cho tới khi điều đó thay đổi.
- **Desktop app chỉ có cho Linux.** Nó cần GTK 4 và WebKitGTK 6.0, và pipeline
  release chỉ build cho `linux/amd64`.
- **Domain `.test` và HTTPS không được tự động hoá.** Govard ghi vào hosts file
  của Unix và tin cậy CA cục bộ trong kho chứng chỉ của Unix. Trên Windows bạn
  phải tự thêm dòng vào `C:\Windows\System32\drivers\etc\hosts` và import CA vào
  Trusted Root store.
- **`install.sh` không chạy.** Đó là script bash gọi `uname`; hãy dùng npm hoặc
  archive trong trang release.

## WSL2, đường đi được hỗ trợ

Chạy mọi thứ bên trong bản phân phối:

```bash
# trong bản phân phối WSL2
npm i -g @ddtcorex/govard
govard version
govard svc up        # proxy, giao diện database, mail catcher
govard init
govard env up
```

Hai điều cần biết:

- **Docker:** cài Docker Engine bên trong bản phân phối, hoặc bật tích hợp WSL
  của Docker Desktop cho bản phân phối đó. Cách nào cũng được, miễn là từ trong
  WSL2 gọi được Docker, vì Govard nói chuyện với Docker qua đó.
- **Mở từ Windows:** các domain `.test` phân giải bên trong bản phân phối. Muốn
  mở bằng trình duyệt trên Windows, thêm đúng dòng đó vào hosts file của Windows,
  hoặc mở trực tiếp qua IP của bản phân phối.

## Báo lỗi trên Windows

Mở một bug report kèm output của `govard doctor`. Vì Windows không nằm trong CI,
một báo lỗi có đúng lệnh và đúng phiên bản là thứ giúp sửa được.
