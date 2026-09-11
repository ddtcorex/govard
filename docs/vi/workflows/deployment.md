---
title: Triển khai
description: Triển khai một revision git lên remote bằng govard — pipeline task trung tính, recipe theo framework, hook, mô hình CI hai job với artifact, rollback, và sandbox để diễn tập ngay trên máy bạn.
---

# Triển khai

`govard deploy` đưa đúng một revision git lên một môi trường remote. Lệnh chỉ cần
SSH và rsync — không Docker, không PHP ở máy local, không cần công cụ deploy
khác — nên cùng một lệnh chạy được trên laptop và trong job CI.

```bash
govard deploy staging --revision <sha>   # triển khai đúng một commit
govard deploy staging                    # ... hoặc HEAD ở máy local
govard deploy plan staging               # in kế hoạch, không kết nối
govard deploy check staging              # kiểm tra trước và báo target ngụ ý gì
```

Đích là một remote trong `.govard.yml`. Branch, repository, deploy path và chiến
lược publish lấy từ block `deploy:` của dự án; mỗi remote có thể ghi đè.

## Pipeline

Triển khai là một chuỗi task trung tính cố định, do engine sắp thứ tự chứ không
phải do recipe:

| Stage | Task |
| --- | --- |
| `prepare` | preflight, lock, thư mục release, code, shared, quyền ghi |
| `build` | dependencies, patches, sinh code, frontend, static — hoặc artifact |
| `publish` | maintenance, workers, backup DB, cấu hình, migration, kích hoạt, cache, ghi record |
| `verify` | kiểm tra sau publish |
| `cleanup` | dọn release cũ, nhả lock |

Mỗi framework đóng góp một **recipe** điền những task nó hỗ trợ; task bỏ trống
được báo là skipped, không phải lỗi. Dự án tuỳ biến pipeline bằng **hook** neo vào
task id, alias stage hoặc hook khác:

```yaml
deploy:
  hooks:
    - { name: varnish-purge, on: "publish:activate", position: after, order: 10, run: "varnishadm ban req.url ~ /" }
```

`govard deploy plan` in ra cây thực thi kèm nguồn và cách hiện thực của từng bước,
nên có thể soi vị trí hook mà không cần kết nối tới đâu.

## Build mode

`--build=auto` (mặc định) quyết định theo **sự hiện diện**, không dò đoán môi
trường: có thư mục artifact nghĩa là đã build xong, ngược lại target tự build.

| Mode | Build ở đâu | Dùng khi |
| --- | --- | --- |
| `server` | trên target | hotfix từ laptop, hoặc dự án chưa có CI |
| `artifact` | trên máy chạy `govard deploy build` | CI, để job deploy không cần toolchain |

### Mô hình CI hai job

```yaml
build:
  stage: build
  script:
    - govard deploy build production --output artifacts --revision $CI_COMMIT_SHA
  artifacts:
    paths: [artifacts/]

deploy:
  stage: deploy
  script:
    - govard deploy production --artifact-dir artifacts --revision $CI_COMMIT_SHA --yes
```

Image của job `deploy` chỉ cần govard, SSH và rsync: không PHP, không Composer,
không Node, không container runtime. Artifact được upload vào release và manifest
của nó được đối chiếu với revision đang triển khai cùng phiên bản PHP của target —
image CI không khớp server bị từ chối trước khi publish. `govard deploy plan
--artifact-dir artifacts` cho biết đang chạy nhánh nào của stage build.

Thư mục output không rỗng sẽ bị từ chối để một file còn sót từ lần build trước
không thể lọt ra production. Dùng `--force` nếu muốn thay nội dung.

## Publish

`--publish=auto` đọc target thay vì đoán:

- current path không tồn tại hoặc là symlink → **symlink**: release nằm trong
  `releases/<n>` và cú swap là `mv -T` nguyên tử, nên không khách nào thấy một
  cây file publish dở;
- current path là thư mục thật → **in_place**: docroot được reset về đúng
  revision, các path cấu hình được copy vào, và file static content version được
  ghi cuối cùng.

`deploy check` báo layout ngụ ý chiến lược nào và vì sao.

## Kiểm chứng, backup và rollback

`deploy:verify` chạy sau publish và bật mặc định: revision đang live, dấu artifact,
các shared file recipe yêu cầu, một kiểm tra ứng dụng do recipe cung cấp có chạm
vào dependency thật, và kiểm tra HTTP khi đã đặt `deploy.verify.url`. Một lần
deploy có migrate database mà không có kiểm tra HTTP sẽ in cảnh báo rõ ràng thay
vì giả vờ rằng kiểm chứng chỉ bằng SSH là đủ.

`--db-backup` dump database vào `shared/backups/deploy/<n>/` ngay trước task đầu
tiên thay đổi database và ghi lại đường dẫn trong release.

```bash
govard deploy releases staging                  # target đang có gì
govard deploy status                            # mỗi môi trường đang chạy gì
govard deploy rollback staging                  # đưa release trước đó trở lại
govard deploy rollback staging --to 12          # ... hoặc một release chỉ định
govard deploy rollback staging --with-db --yes  # ... kèm cả dump database
```

Rollback không bao giờ build lại: layout symlink được trỏ lại, còn layout
in-place chạy lại phần publish từ thư mục release đã có trên server.

Một lần deploy lỗi vẫn giữ thư mục release và record của nó. `--resume` tiếp tục
release mới nhất chưa xong thay vì tạo release mới, và `--from <task>` bắt đầu từ
một task hoặc hook chỉ định. `govard deploy unlock` giải phóng lock do lần lỗi để
lại.

## Sandbox

`govard deploy sandbox` cho dự án một đích triển khai thật ngay trên máy bạn — một
container đóng vai remote — để diễn tập trước khi chạm vào server. Không phần nào
trong pipeline biết sự khác biệt, nên đây là diễn tập thật chứ không phải mô
phỏng.

```bash
govard deploy sandbox up                      # tạo (mặc định profile php)
govard deploy sandbox up --profile basic      # chỉ sshd, rsync, git
govard deploy sandbox status
govard deploy sandbox reset --layout deployer # seed target mà công cụ kia đang giữ
govard deploy sandbox ssh
govard deploy sandbox down [--purge]
```

`up` publish SSH trên một cổng loopback còn trống, sinh khoá riêng dưới
`.govard/sandbox/` (đã gitignore), mount read-only một mirror của repository local,
và ghi remote `sandbox` vào `.govard.local.yml`. Mirror được refresh trước mỗi lần
deploy, nên một commit bạn chưa từng push vẫn triển khai được. Vì `sandbox` cũng
là một subcommand, hãy deploy bằng dạng flag:

```bash
govard deploy --remote sandbox --yes
```

`--docroot` định hình target để chiến lược publish resolve theo đúng thứ bạn muốn
kiểm chứng: `absent` hoặc `symlink` chọn cú swap nguyên tử, `real` chọn in-place.
`down` xoá container và remote mà nó đã ghi; `--purge` xoá thêm image, khoá và
mirror.

## Capability và exit code

Mọi lệnh deploy khai báo thứ nó cần, và thiếu yêu cầu là exit `3` kèm thông báo
hành động được, trước khi làm bất cứ việc gì:

| Lệnh | Yêu cầu |
| --- | --- |
| `govard deploy` / `rollback` | `ssh,rsync` |
| `govard deploy check` / `releases` / `status` / `unlock` | `ssh` |
| `govard deploy plan` / `build` | `none` |
| `govard deploy sandbox *` | `docker` |

Exit code: `0` thành công, `1` lỗi thực thi, `2` sai cách dùng, `3` thiếu
capability, `4` lỗi cấu hình. Nhờ vậy job deploy trong CI chạy được trên host chỉ
có govard, SSH và rsync.
