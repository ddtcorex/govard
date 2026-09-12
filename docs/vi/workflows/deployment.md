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
`deploy_path` không có mặc định, nên remote nào bỏ trống sẽ dùng layout mà target
đã có sẵn — `releases/`, `shared/`, `.dep/` hoặc symlink `current` — và govard chỉ
nhận khi đúng một ứng viên khớp, đồng thời nói rõ là cái nào. Không có layout nào,
hoặc có nhiều cái, đều là lỗi cấu hình kèm danh sách đã dò.

## Pipeline

Triển khai là một chuỗi task trung tính cố định, do engine sắp thứ tự chứ không
phải do recipe:

| Stage | Task |
| --- | --- |
| `prepare` | preflight, lock, thư mục release, code, shared, quyền ghi |
| `build` | dependencies, patches, sinh code, frontend, static — hoặc artifact |
| `publish` | maintenance, workers, backup DB, cấu hình, migration, kích hoạt, cache, ghi record |

Maintenance window chỉ được mở khi nó thật sự cần: kích hoạt bằng symlink là một
cú rename nguyên tử nên không có gì đang phục vụ bị ghi đè, và window chỉ xuất
hiện nếu cùng plan đó còn migrate hoặc import cấu hình. Kích hoạt in-place luôn mở
window, vì chính docroot bị ghi đè trong lúc đang phục vụ.
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

Mọi hành vi nâng cao đều nằm sau một flag và mặc định là bản tối ưu:
`--no-verify`, `--no-db-backup` và `--lock=false` tắt bớt việc, `--force` deploy
lại revision mà target đã chạy, còn `--build`, `--artifact-dir` và `--publish`
đổi cách release được tạo và publish.

`deploy.settings` được đối chiếu với recipe trước khi chạy bất cứ thứ gì: key mà
framework không biết, hoặc giá trị sai dạng, là lỗi cấu hình (exit 4) có nêu tên
key và gợi ý key gần đúng. Trước đây gõ sai là im lặng bỏ qua —
`static_content_locale: en_US` deploy mọi locale mà recipe mặc định. Giá trị chuỗi
phải được quote nếu trông giống số (`php_version: "8.2"`): engine đọc các setting
này dưới dạng chuỗi, nên `8.2` không quote sẽ đọc thành rỗng.

### Tách static content

`split_static_deployment` deploy static content của adminhtml và frontend thành
hai lượt thay vì một, với `--area=adminhtml` rồi `--area=frontend`. Lượt admin
dùng `magento_themes_backend` (mặc định là theme admin) và
`static_content_locales_backend`, mặc định lấy theo ngôn ngữ frontend để hai lượt
khớp nhau trừ khi dự án nói khác. Lượt frontend dùng `magento_themes` và
`static_content_locales`.

Nó dành cho hai trường hợp mà một lượt xử lý kém: danh sách theme chỉ có theme
frontend (theme admin sẽ bị bỏ sót) và deployment lớn nơi một tiến trình ôm mọi
area sẽ hết bộ nhớ. Lượt thứ hai được nối bằng `&&`, nên lượt admin lỗi thì deploy
dừng lại thay vì publish một nửa static content.

### Build frontend (Hyvä)

Theme Hyvä được build bằng Node ngay trong thư mục của nó, dự án khai báo qua
`settings.frontend_dir`:

```yaml
deploy:
  settings:
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind
    frontend_command: npm ci && npm run build   # mặc định
```

`frontend_command` chạy bên trong `frontend_dir`, trong release, như một command
shell: mặc định nối hai command, nên giá trị một command như
`npx tailwindcss -i input.css -o output.css` cũng viết y hệt. Để `frontend_dir`
trống thì bước này bị bỏ qua — đúng ý cho dự án Luma hoặc theme mặc định.

Node cần ở nơi *build*, không phải nơi deploy: với `--artifact-dir`, theme được
build ở job build của CI và `build:frontend` bị bỏ qua trên target, nên image của
job deploy vẫn chỉ cần govard + ssh + rsync.

### Composer repository riêng

Release build trên target thì cài dependency ngay trên đó, nên cần credential cho
mọi repository không phải packagist. `COMPOSER_AUTH` từ môi trường được chuyển
tiếp tới bước cài dependency — qua standard input, không nằm trong command, nên
không lộ trong process list của target và không lọt vào log deploy — và
`shared/auth.json` trên target cũng dùng được như với công cụ deploy kia. Govard
không lưu cái nào.

`govard deploy check` cho biết đang dùng nguồn nào, và cảnh báo khi dự án khai báo
repository riêng mà không có nguồn nào. Nó cảnh báo chứ không từ chối: thứ govard
đọc được chỉ là URL, và URL không cho biết repository có cần credential hay không.
Hãy đặt `COMPOSER_AUTH` ở cả job build lẫn job deploy khi artifact được build ở đó.

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
không Node, không container runtime. Ở mode này năm task build bị bỏ qua trên
target — artifact đã mang sẵn phần chúng tạo ra — nên không có gì trên server phải
chạy `composer install`, `setup:di:compile` hay `setup:static-content:deploy`.
Artifact được upload vào release và manifest của nó được đối chiếu với chính các
file, với revision đang triển khai và với phiên bản PHP của target — image CI không
khớp server, hoặc artifact đã bị đổi byte sau khi build, đều bị từ chối trước khi
publish. `govard deploy plan --artifact-dir artifacts` cho biết đang chạy nhánh nào
của stage build.

Thư mục output không rỗng sẽ bị từ chối để một file còn sót từ lần build trước
không thể lọt ra production. Dùng `--force` nếu muốn thay nội dung.

## Publish

`--publish=auto` đọc target thay vì đoán:

- current path không tồn tại hoặc là symlink → **symlink**: release nằm trong
  `releases/<n>` và cú swap là `mv -T` nguyên tử, nên không khách nào thấy một
  cây file publish dở;
- current path là thư mục thật → **in_place**: object của docroot được fetch trong
  `prepare`, ngoài mọi maintenance window, rồi docroot được reset về đúng
  revision, các path cấu hình được copy vào, và file static content version được
  ghi cuối cùng.

`deploy check` báo layout ngụ ý chiến lược nào và vì sao.

## Kiểm chứng, backup và rollback

`deploy:verify` chạy sau publish và bật mặc định: revision đang live (symlink
`current` được resolve, hoặc `HEAD` của docroot với target in-place), các shared
file recipe yêu cầu, các check do recipe khai báo, và kiểm tra HTTP khi đã đặt
`deploy.verify.url`. Check của recipe là những thứ engine không thể tự biết — với
Magento là `bin/magento setup:db:status`, cần `app/etc/env.php` hoạt động *và*
database kết nối được, cộng thêm so sánh static content version của docroot với
release khi publish in-place.

Không cấu hình URL thì kiểm chứng chỉ bằng SSH: nó chứng minh đúng file đã nằm
đúng chỗ, không chứng minh ứng dụng phục vụ được. Một lần deploy có chạy
`db:migrate` mà không có verify URL sẽ nói rõ điều đó trước bước đầu tiên, thay vì
để người vận hành tưởng ngược lại.

`--db-backup` dump database vào `shared/backups/deploy/<n>/` ngay trước task đầu
tiên thay đổi database và ghi lại đường dẫn trong release. `deploy:cleanup` dọn
các dump đó theo đúng window `keep_releases` như các release mà chúng thuộc về,
nên backup không thể phình mãi trên máy production.

```bash
govard deploy releases staging                  # target đang có gì
govard deploy status                            # mỗi môi trường đang chạy gì
govard deploy rollback staging                  # đưa release trước đó trở lại
govard deploy rollback staging --to 12          # ... hoặc một release chỉ định
govard deploy rollback staging --with-db --yes  # ... kèm cả dump database
```

Rollback không bao giờ build lại: layout symlink được trỏ lại, còn layout
in-place chạy lại phần publish từ thư mục release đã có trên server.

`deploy.lock_stale_after` (mặc định 2h) là ngưỡng để `govard deploy unlock` nhả
lock mà không cần `--force`; thông báo từ chối khi lock đang bị giữ có nêu người
giữ, revision và đã giữ bao lâu.

`deploy.maintenance_timeout` (mặc định 15m) chặn mỗi bước chạy trong lúc site đang
maintenance, thấp hơn hẳn `deploy.command_timeout` là có chủ đích: bước chậm trong
window là đang giữ site down. Hãy nâng lên với dự án có dump database vốn mất
nhiều thời gian.

Một lần deploy lỗi vẫn giữ thư mục release và record của nó. Lỗi ở đâu quyết định
số phận của lock: lỗi trong `prepare` hoặc `build` sẽ nhả lock vì chưa có gì live
thay đổi, nên chỉ cần sửa lỗi rồi deploy lại — còn lỗi từ `publish` trở đi thì giữ
lock, vì target có thể đang dở dang, và đường đi tiếp là `govard deploy <remote>
--resume` để tiếp tục release mới nhất chưa xong thay vì tạo release mới.
`--from <task>` bắt đầu từ một task hoặc hook chỉ định, và `govard deploy unlock`
giải phóng lock do lần lỗi để lại.

## Output cho máy đọc

`--json` ghi đúng một JSON document ra stdout, còn mọi thứ cho người đọc ra stderr,
nên job CI có thể pipe stdout vào parser và giữ stderr trong log:

```json
{"schema_version":1,"remote":"production","branch":"main","revision":"0123abc…",
 "release":"42","build":{"mode":"server"},"publish":{"strategy":"symlink"},
 "verify":"ok","result":"ok","duration_ms":94300,
 "tasks":[{"id":"deploy:check","stage":"prepare","status":"ok","duration_ms":19}]}
```

Deploy lỗi cho ra cùng document đó với `"result":"failed"` và `"error"` nêu task,
host, command; tiến trình thoát 1. Release record mang `ci.pipeline`/`ci.job` khi
lần chạy đó là CI, nhờ vậy câu "pipeline nào đã deploy cái này" trả lời được sau
đó từ `govard deploy status`.

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
