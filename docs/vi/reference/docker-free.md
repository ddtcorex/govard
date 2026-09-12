---
title: Các lệnh chạy không cần Docker — Govard
description: Toàn bộ lệnh Govard có yêu cầu trong manifest không bao gồm Docker — phân tích audit không cần container, chẩn đoán trên host, remote và tunnel — kèm capability gate và mã thoát.
---

# Chạy không cần Docker

Govard cài và chạy được trên máy không có container runtime. Mỗi lệnh khai báo
yêu cầu của nó trong manifest của binary, và `govard capabilities` in ra tập đã
phân giải cho đúng máy bạn đang dùng — **output đó là nguồn sự thật, trang này
chỉ phản chiếu lại.**

```bash
govard capabilities          # lệnh, yêu cầu, trạng thái trên máy này
govard capabilities --json   # machine-readable (schema_version 1)
```

## Các lệnh không cần Docker

| Lệnh | Yêu cầu |
| :--- | :--- |
| `govard audit cleanup` | `none` |
| `govard audit diff` | `none` |
| `govard audit rerun` | `none` |
| `govard audit result` | `none` |
| `govard audit run` | `none` |
| `govard audit status` | `none` |
| `govard blueprint` | `none` |
| `govard blueprint cache` | `none` |
| `govard blueprint cache clear` | `none` |
| `govard blueprint cache list` | `none` |
| `govard capabilities` | `none` |
| `govard completion bash` | `none` |
| `govard completion fish` | `none` |
| `govard completion powershell` | `none` |
| `govard completion zsh` | `none` |
| `govard config` | `none` |
| `govard config get` | `none` |
| `govard config profile` | `none` |
| `govard config profile clear` | `none` |
| `govard config set` | `none` |
| `govard custom` | `none` |
| `govard custom list` | `none` |
| `govard deploy` | `ssh,rsync` |
| `govard deploy build` | `none` |
| `govard deploy check` | `ssh` |
| `govard deploy plan` | `none` |
| `govard deploy releases` | `ssh` |
| `govard deploy rollback` | `ssh,rsync` |
| `govard deploy status` | `ssh` |
| `govard deploy unlock` | `ssh` |
| `govard desktop doctor` | `none` |
| `govard doctor` | `none` |
| `govard doctor trust` | `none` |
| `govard domain list` | `none` |
| `govard help` | `none` |
| `govard init` | `none` |
| `govard project list` | `none` |
| `govard project open` | `none` |
| `govard remote add` | `ssh,rsync` |
| `govard remote audit stats` | `ssh,rsync` |
| `govard remote audit tail` | `ssh,rsync` |
| `govard remote copy-id` | `ssh,rsync` |
| `govard remote exec` | `ssh,rsync` |
| `govard remote test` | `ssh,rsync` |
| `govard self-update` | `net` |
| `govard sync` | `ssh,rsync` |
| `govard trust` | `none` |
| `govard tunnel` | `cloudflared` |
| `govard tunnel start` | `cloudflared` |
| `govard tunnel status` | `cloudflared` |
| `govard tunnel stop` | `cloudflared` |
| `govard version` | `none` |
| `govard vscode setup` | `none` |

Cột `Yêu cầu` là đúng token mà `govard capabilities` báo:

| Token | Ý nghĩa |
| :--- | :--- |
| `none` | Không cần gì ngoài binary Govard. |
| `ssh,rsync` | Cần SSH client và rsync. Vẫn không cần container runtime. |
| `cloudflared` | Cần binary `cloudflared`, cho tunnel. Vẫn không cần container runtime. |
| `net` | Cần kết nối mạng ra ngoài, cho `self-update`. Vẫn không cần container runtime. |

Những lệnh có yêu cầu bao gồm `docker` thì đi qua gate: vòng đời stack (`env`,
`restart`, `down`, `ps`, `logs`, `svc`, `db`, `shell`, `tool`, `test`,
`frontend`), phần quản lý project/domain có chạm container (`project delete`,
`project orphans`, `domain add`, `domain remove`), các wrapper `vscode <tool>`,
`deploy`, `bootstrap`, `debug`, mở `desktop`, và các nhánh audit cần container (`audit toolchain`,
`audit run --checks lint`, `audit run --checks profiler`).

## Tính năng không cần Docker

- **Phân tích không cần container.** `govard audit run --checks integrity`
  (Magento 2 / Mage-OS) đọc trực tiếp checkout bằng analyzer Go — Composer
  manifest/lock có khớp nhau, tính nhất quán module/DI/sequence — không Docker,
  không PHP, không toolchain image. Phần còn lại của vòng đời audit cũng chạy
  trên host: `status`, `result`, `cleanup`, `diff`. `rerun` chạy lại đúng bộ
  check đã ghi trong session, nên rerun một session `lint`/`profiler` vẫn cần
  Docker.
- **Chẩn đoán.** `govard doctor` chạy ở mọi nơi; Docker chỉ là một check tùy
  chọn và thiếu nó không làm lệnh fail (`--strict` khôi phục hard gate cho
  script bootstrap). `govard trust` cài CA nội bộ vào trust store của máy.
- **Cấu hình.** `govard config get|set` và `govard config profile` đọc/ghi
  cấu hình project; `govard config profile clear` và
  `govard blueprint cache list|clear` làm việc trên file và cache local. Áp
  profile vào stack đang chạy (`config profile apply|switch`), `govard config
  auto` (nó cấu hình framework bên trong container) và mọi lệnh `govard lock` là
  việc của container: lock file ghi lại version docker/compose đã phân giải và
  digest image của từng service.
- **Khởi tạo project.** `govard init` và `govard custom list`.
- **Registry và domain.** `project list` và `project open` đọc registry;
  `domain list` in ra domain của project; `vscode setup` suy ra cấu hình editor từ
  chính file của project. `project orphans` quét tài nguyên Docker nên vẫn cần runtime.
- **Triển khai.** `govard deploy` và `govard deploy rollback` cần SSH và rsync;
  `govard deploy build` cũng không cần gì: đây là nửa CI của artifact mode, chạy
  trên runner có toolchain của dự án và không hề kết nối tới server. Chính sự
  tách đôi này cho phép job deploy chỉ cần govard, SSH và rsync — không PHP,
  không Composer, không container runtime. Ngoại lệ duy nhất trong nhóm deploy là
  `govard deploy sandbox *`: nó tạo một container đóng vai target, tức là việc của
  container runtime theo đúng định nghĩa, và đó là cách duy nhất để một buổi diễn
  tập dùng chung code path với production. `govard deploy check`,
  `govard deploy releases`, `govard deploy status` và
  `govard deploy unlock` chỉ cần SSH. `govard deploy plan` không cần gì cả: nó
  đọc `.govard.yml` và in kế hoạch thực thi mà không kết nối đi đâu. Rollback chỉ
  trỏ lại symlink hoặc chạy lại phần publish từ thư mục release đã có trên
  server, nên không cần toolchain build ở máy local.

- **Remote và đồng bộ.** `govard remote add|test|copy-id|exec`,
  `govard remote audit stats|tail`, và `govard sync` cần SSH và rsync, không cần
  Docker.
- **Tunnel.** `govard tunnel start|stop|status` điều khiển `cloudflared` trên
  host.
- **Self-update, help, completion.** `govard self-update` chỉ cần mạng;
  `govard version`, `govard help`, và `govard completion bash|zsh|fish|powershell`
  không cần gì cả.

## Khi thiếu capability

Gate từ chối trước khi lệnh làm bất cứ việc gì, và nói rõ thiếu gì:

| Mã thoát | Ý nghĩa |
| :--- | :--- |
| `0` | Thành công. |
| `1` | Lệnh đã chạy và thất bại. |
| `2` | Lỗi usage: flag lạ hoặc tham số không hợp lệ. |
| `3` | `CAPABILITY_MISSING` — một yêu cầu đã khai báo không có sẵn. |
| `4` | Lỗi cấu hình. |

`--error-json` in lỗi ra stdout dưới dạng envelope machine-readable
(`schema_version`, `code`, `capability`, `command`, `message`, `hint`), nên
script không phải parse dạng text:

```bash
govard tunnel status --error-json
# {"schema_version":1,...,"error":{"code":"CAPABILITY_MISSING","capability":"cloudflared",...}}
```

Một check audit cần container sẽ chỉ thẳng sang lựa chọn không cần container:

```bash
govard audit run --checks lint
# Hint: run `govard audit run --checks integrity` for container-free analysis on this host
```

## Liên quan

- [Cài đặt](/vi/getting-started/installation) — cài và chạy CLI không cần Docker.
- [Audit](/vi/workflows/audit) — check `integrity` và nhánh lint cần container.
- [Lệnh CLI](/vi/reference/cli-commands) — tham chiếu đầy đủ.
