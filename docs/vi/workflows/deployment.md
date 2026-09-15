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

Trang này mô tả engine và mọi thứ có thể điều chỉnh trong đó. Về những cấu hình đã làm sẵn —
một store Luma, một storefront Hyvä, nhiều theme và store view, chế độ developer so
với production, và docroot symlink so với docroot thật — xem
[Case study triển khai](/vi/workflows/deploy-case-studies).

## Cài đặt cho một dự án Magento

Bốn thứ phải tồn tại trước lần deploy đầu tiên: một target đã đang phục vụ dự án,
một remote mà govard kết nối được, credential cho những gì release sẽ cài, và một
quyết định về cách một release trở thành live. Mỗi thứ hỏng theo một thông báo
riêng, và không bước nào suy ra bước khác.

### 1. Target đã chạy ứng dụng

`govard deploy` publish *vào* một installation đang chạy; nó không tạo ra
installation đó. Với dự án Magento, target cần:

- **một ứng dụng đã cài** — `shared/app/etc/env.php`, file mà `deploy:shared` link
  vào mọi release, trỏ tới database, cache backend và session handler mà target
  kết nối được;
- **database phía sau nó**, có cấu hình store trong đó.
  `setup:static-content:deploy` dừng với `The default website isn't defined` khi
  thiếu bảng store, và migration thì không chạy được;
- **một search engine được hỗ trợ** nếu dự án dùng. `setup:upgrade` từ chối thẳng
  fallback MySQL của Magento — `Your current search engine, 'MySQL', is not
  supported` — nên dự án ElasticSuite/OpenSearch phải có cluster kết nối được
  *trước* lần deploy đầu, không phải sau;
- **SSH** cho user deploy kèm `rsync` trên target, và quyền ghi vào thư mục layout;
- **credential** cho các nguồn private, đặt ở target (mục sau).

Target chưa từng chạy ứng dụng là target không recipe nào publish được. Sandbox là
chỗ để phát hiện điều đó trước khi dính tới server.

### 2. Khai báo remote

```yaml
remotes:
  staging:
    host: m2-staging.example.com
    user: m2-staging
    port: 22
    path: /home/m2-staging/public_html      # docroot đang được phục vụ
    auth:
      method: keyfile
      key_path: ~/.ssh/staging
    branch: main                            # tuỳ chọn; mặc định là HEAD local
    deploy:
      path: /home/m2-staging/.deployer      # releases/, shared/, .dep/ nằm ở đây
      settings:
        php_bin: php8.3
        composer_bin: composer
        php_version: "8.3"                  # target chạy gì, không phải mong muốn
        owner: m2-staging:m2-staging
        writable_mode: chmod+chown
      verify:
        url: https://staging.example.com/
```

`php_version` là một cổng chặn: `deploy:check` chạy `<php_bin> -r 'echo
PHP_VERSION;'` trên target và từ chối deploy khi series không khớp, vì release
build bằng interpreter sai sẽ hỏng muộn hơn và nói ít hơn về lý do. `deploy:check`
cũng báo layout nó tìm thấy, chiến lược publish mà layout đó ngụ ý, dung lượng
trống ở deploy path, và repository có tới được từ target hay không — hãy chạy nó
trước lần deploy đầu và đọc như câu trả lời cho "remote này đã sẵn sàng chưa".

### 3. Recipe Magento đã làm sẵn những gì

Recipe điền các task theo framework và có mặc định cho layout shared, nên dự án
chuẩn không cần `deploy.settings` nào cả:

| Setting | Mặc định |
| --- | --- |
| `shared_files` | `app/etc/env.php`, `var/.maintenance.ip` |
| `shared_dirs` | `var/log`, `var/report`, `var/session`, `var/backups`, `var/tmp`, `pub/media`, `pub/sitemap`, `pub/static/_cache` |
| `writable_dirs` | `var`, `pub/static`, `pub/media`, `generated`, `app/etc` |
| `sync_paths` | `vendor`, `generated`, `pub/static/adminhtml`, `pub/static/frontend` — chỉ khi publish in-place |

Chỉ ghi đè những gì dự án của bạn khác:

- **frontend**: `frontend_dir` (một hoặc nhiều đường dẫn theme Hyvä) và
  `frontend_command` (mặc định `npm ci && npm run build`) — xem *Build frontend*;
- **static content**: `static_jobs`, `static_content_locales`,
  `static_deploy_options`, việc tách adminhtml/frontend, và các danh sách theme —
  xem *Tách static content* và *Nhiều store, website và theme*;
- **mode**: `mage_mode` (`developer` bỏ qua việc deploy static content) — xem
  *Chế độ developer và production*;
- **workers**: `worker_control: true` chạy `cron:remove`/`queue:consumers:stop`
  quanh lúc deploy rồi khôi phục lại;
- **opcache**: `runtime_reload_command` sau bước flush cache, cho target mà opcache
  truy cập được từ user deploy — xem *Cache, opcache và cú swap symlink*;
- **ownership**: `owner`, `writable_mode` (`chmod`, `chown`, `chmod+chown`, `acl`,
  `skip`) và `writable_permissions` — xem *Quyền ghi và ownership*.

Mọi key mà cả hai recipe khai, kèm mặc định và hình dạng của nó, được bảng hoá trong
[Case study triển khai — phần Tham chiếu](/vi/workflows/deploy-case-studies#reference-every-setting-the-framework-recipes-read).

### 4. Credential Composer đến từ đâu

Ba đường, theo đúng thứ tự Composer resolve, và thứ tự này quan trọng:

| Đường | Cách nó tới bước build |
| --- | --- |
| `COMPOSER_AUTH` trong môi trường deploy | được chuyển tiếp vào `build:vendors` qua **standard input** (không bao giờ vào argv, không vào log) và **đè lên mọi file trên target** |
| `auth.json` trong dự án | được `deploy:code` materialise vào release, nên Composer đọc nó từ gốc release — đây là đường mà dự án commit sẵn file này dựa vào |
| `shared/auth.json` trên target | chỉ được đọc nếu release có `auth.json` trỏ tới nó, nghĩa là phải liệt kê `auth.json` trong `deploy.settings.shared_files` |

Vì `COMPOSER_AUTH` thắng, một **token dùng chung cho cả máy nhưng sai với dự án
còn tệ hơn không có gì**: nó đè lên `auth.json` đang chạy tốt của dự án và build
hỏng với lỗi authentication của nguồn, trông y hệt một dự án không có credential.
Hãy export đúng credential của dự án cho lần deploy, hoặc không export gì và để
file sẵn có trên target trả lời.

Package kiểu `git` là vấn đề khác: Composer clone qua SSH, nên *target* cần khoá và
một dòng `known_hosts` cho host đó. Không biến môi trường nào mang được hai thứ
ấy.

`govard deploy check` nói nó tìm thấy đường nào — `COMPOSER_AUTH is set for this
run`, `shared/auth.json exists on the target`, hay cảnh báo rằng dự án khai báo
repository private mà không có credential nào.

### 5. Release trở thành live thế nào

`auto` (mặc định) resolve từ target: docroot không tồn tại hoặc là symlink thì
publish bằng cú rename nguyên tử của symlink `current`; docroot đang là checkout
thật thì cập nhật in-place. Chọn tường minh bằng `publish: symlink` hoặc
`publish: in_place` trên remote khi target mơ hồ — ví dụ docroot là thư mục nhưng
không phải checkout.

Với in-place, release được reset vào docroot và chỉ `sync_paths` được copy, nên
danh sách đó phải nêu đúng những gì release *build ra*; xem *Publish in-place cần
`sync_paths`*. Ở cả hai chiến lược, maintenance window mở trên release đang **được
phục vụ**, và với symlink nó đóng trước cú swap.

### 6. Lần deploy đầu tiên

```bash
govard deploy plan staging     # toàn bộ danh sách task, không kết nối đi đâu
govard deploy check staging    # preflight: kết nối, layout, quyền, php, dung lượng, lock
govard deploy staging --yes    # ... hoặc --remote staging
```

Theo dõi bằng `--verbose`, nó stream output của từng command dưới đúng task của nó
và không gom lại. Khi một bước hỏng, lần chạy nói rõ bước nào và làm gì tiếp: hỏng
sau khi maintenance window đã mở thì giữ lock và chỉ tới `govard deploy --remote
staging --resume`, còn hỏng trước đó thì nhả lock và chỉ tới một lần retry
thường. Exit code là hợp đồng của CLI (`0` thành công, `1` lỗi thực thi, `2` dùng
sai, `3` thiếu capability, `4` lỗi cấu hình), và `--json` phát ra một document thay
cho timeline.

### 7. Diễn tập ngay trên máy này trước

```bash
govard deploy sandbox up --profile full --php 8.4   # một target thật, trên loopback
# provision nó: credential, shared/app/etc/env.php, database, search engine
govard deploy --remote sandbox --yes
govard deploy sandbox down --purge
```

Sandbox là một lần deploy production trỏ vào container — cùng SSH, cùng mirror,
cùng recipe — nên hỏng ở đó chính là hỏng bạn sẽ gặp trên server, mà không cần
server. Nó cần đúng những prerequisite ứng dụng như mọi target; mục *Sandbox* bên
dưới nói từng lỗi nghĩa là gì, và
[Case study triển khai](/vi/workflows/deploy-case-studies#rehearsing-any-case-in-the-sandbox)
đưa ra profile cùng hình dạng `--docroot` mà mỗi kiểu dự án cần.

## Laravel, Symfony và WordPress

Engine trung lập với framework; recipe là thứ điền command cụ thể của một ứng dụng
vào các task trung tính của pipeline. Mọi thứ ở trên — release, chiến lược publish,
rollback, `deploy check`, sandbox — áp dụng nguyên vẹn cho ba framework này. Phần
dưới chỉ nói cái mà mỗi recipe thêm vào, và ba chỗ nó **không** hành xử như recipe
Magento.

| Task | Laravel | Symfony | WordPress |
| --- | --- | --- | --- |
| `build:vendors` | `composer install --no-dev --optimize-autoloader` | như trên, thêm `--no-scripts` | chỉ khi có `composer.json` |
| `build:assets` | — | `assets:install public --symlink --relative` (chạy ở target) | — |
| `build:frontend` | `frontend_dir` × `frontend_command` | như trên | như trên |
| `app:configure` | `artisan storage:link` | — | — |
| `db:migrate` | `artisan migrate --force` | `doctrine:migrations:migrate --allow-no-migration` | `wp core update-db` |
| `maintenance:enable` / `disable` | `artisan down` / `up` | **để trống** | hai file trong đường dẫn được serve |
| `app:cache:flush` | `artisan optimize:clear` rồi `optimize` | `cache:clear --no-warmup` rồi `cache:warmup` | `wp cache flush` + `wp rewrite flush --hard` |
| `app:workers:pause` | `artisan queue:restart` (+ `horizon:terminate`) | `messenger:stop-workers` | — |
| `db:backup` / restore | — | — | `wp db export` / `wp db import` |
| check của `deploy:verify` | `artisan db:show` | `dbal:run-sql "SELECT 1"` | `wp core is-installed` |

State dùng chung, theo từng framework:

| Framework | `shared_files` | `shared_dirs` | `sync_paths` (chỉ in-place) |
| --- | --- | --- | --- |
| Laravel | `.env` | `storage` | `vendor`, `public/build` |
| Symfony | `.env.local` | `var/log` | `vendor`, `public/bundles` |
| WordPress | `wp-config.php` | `wp-content/uploads` | `vendor` |

### Các bước build chạy ở đâu, và artifact mang theo gì

Ở `--build=server` mọi bước chạy trên target. Ở `--build=artifact`, job build chạy
`govard deploy build` trên máy của nó, và target bỏ qua năm task build mà artifact
thay thế — **trừ** những task được recipe đánh dấu *cần ứng dụng*
(`NeedsApplication`), thứ mà không máy build nào làm được vì chúng đọc chính cấu
hình của ứng dụng đã cài:

| Recipe | Bước build vẫn chạy trên target ở artifact mode | Artifact phải mang theo |
| --- | --- | --- |
| Magento 2 | `build:assets` (`setup:static-content:deploy`) | `vendor/`, `generated/`, output build frontend |
| Laravel | không có | `vendor/`, output build frontend (`public/build`) |
| Symfony | `build:assets` (`assets:install public --symlink --relative`) | `vendor/`, output build frontend |
| WordPress | không có | `vendor/` nếu dự án có, output build frontend |

Không cache của framework nào thuộc về artifact, và không recipe nào ở đây đặt được
nó vào đó: `app:cache:flush` là bước thuộc giai đoạn publish ở cả bốn recipe, nên nó
luôn chạy trên target — nơi môi trường mà cache "nướng" vào thực sự tồn tại.

### Những bước các recipe này để trống

Bước trống được báo là **skipped**, không bao giờ là fail, và mỗi bước là một quyết
định chứ không phải thiếu sót. `build:compile` và `build:patches` trống ở cả ba:
không framework nào sinh code trước, và không framework nào có bước patch.
`app:workers:resume` trống với Laravel và Symfony — `queue:restart` và
`messenger:stop-workers` đã là toàn bộ tín hiệu, còn restart worker là việc của
process manager — trong khi WordPress không có task worker nào. Symfony để trống cả
hai bước maintenance và `app:configure`; Laravel và Symfony còn để trống `db:backup`,
đó là lý do bật `--db-backup` trên chúng bị từ chối thay vì bị bỏ qua im lặng.

### Mỗi recipe yêu cầu sandbox cấp những gì {#sandbox-recipe-defaults}

Đây là danh sách recipe khai — mặc định của ứng dụng, không phải chính sách. Dự án
override bất kỳ danh sách nào bằng `deploy.settings.sandbox_*`, và mỗi danh sách
**thay thế** danh sách của recipe chứ không nối thêm:

| Recipe | `sandbox_packages` | `sandbox_extensions` | `sandbox_services` | `sandbox_tools` |
| --- | --- | --- | --- | --- |
| Magento 2 | `libxslt1-dev`, `libzip-dev`, `libpng-dev`, `libjpeg-dev`, `libfreetype6-dev`, `default-mysql-client` | `bcmath`, `curl`, `gd`, `intl`, `mysql`, `soap`, `sockets`, `xsl`, `zip` | `mariadb`, `redis-server` | — |
| Laravel | `default-mysql-client` | `bcmath`, `curl`, `gd`, `intl`, `mbstring`, `mysql`, `sqlite3`, `xml`, `zip` | `mariadb`, `redis-server` | — |
| Symfony | `default-mysql-client` | `intl`, `mysql`, `mbstring`, `xml`, `curl`, `zip` | `mariadb`, `redis-server` | — |
| WordPress | `default-mysql-client` | `mysqli`, `curl`, `gd`, `intl`, `mbstring`, `xml`, `zip` | `mariadb`, `redis-server` | `wp-cli` |

Một tên service thuộc bảng của engine mang theo package cung cấp nó, nên chỉ cần
khai `postgresql` là đủ để cài và start; service mà image không start được sẽ được
nêu tên lúc container khởi động thay vì bị bỏ qua im lặng. `sandbox_tools` chỉ nhận
những binary mà engine có công thức cài — hiện tại là `wp-cli` — và tên lạ bị từ chối
ngay khi render image, không phải khi image build fail.

### Ba chỗ ba recipe này khác Magento

**Symfony không có task maintenance.** Symfony không có cơ chế gốc cho việc đó, nên
`maintenance:enable` và `maintenance:disable` để trống và engine báo chúng là
skipped. Vì vậy `db:migrate` chạy thẳng trên site đang sống. Nếu dự án cần một
window, đó là việc của hook:

```yaml
deploy:
  hooks:
    - { name: down, on: "maintenance:enable", position: after, order: 10, run: "touch {{current_path}}/maintenance.lock" }
    - { name: up,   on: "maintenance:disable", position: after, order: 10, run: "rm -f {{current_path}}/maintenance.lock" }
```

**Maintenance của WordPress là ghi file, không phải gọi command.** Cơ chế của
WordPress là hai file: `.maintenance` trong docroot mà `wp_is_maintenance_mode()`
tìm, và drop-in `wp-content/maintenance.php` mà `wp_maintenance()` serve kèm 503.
`wp_is_maintenance_mode()` coi cờ cũ hơn **mười phút** là hết hạn, nên recipe ghi
`$upgrading = time() + 86400`: ghi đúng giá trị WordPress tự ghi sẽ mở lại site
giữa một window dài hơn mười phút, và đưa traffic trở lại trên một database mới
migrate một nửa nếu deploy fail ở phút thứ chín. Drop-in mang một marker và chỉ file
có marker mới bị xoá, nên dự án tự ship trang maintenance của mình thì vẫn giữ được.

Với chiến lược `symlink`, hai file này được ghi vào release đang sống lúc mở window,
nên sau cú swap chúng thuộc release **trước** — release mới được serve bình thường,
điều đó đúng, nhưng rollback về release trước đó sẽ thấy nó vẫn ở maintenance.
`maintenance:disable` xoá chúng ở đường dẫn nó với tới được, và đây là cách xử lý
thủ công:

```bash
rm -f {{current_path}}/.maintenance {{current_path}}/wp-content/maintenance.php
```

**`db:backup` là opt-in theo từng framework.** Mặc định tắt ở mọi nơi. Magento có
qua `setup:backup`, WordPress có qua `wp db export/import`; Laravel và Symfony
**không có dump command** trong recipe. Bật `--db-backup` cho framework không có nó
bị từ chối ngay trước khi lần chạy bắt đầu (exit 4), kèm thông báo nêu tên recipe
và cách xử lý — cố ý, vì bỏ qua im lặng sẽ khiến operator tin là đã có backup ngay
trước một `db:migrate` phá hoại. Dự án muốn có thì thêm hook:

```yaml
deploy:
  hooks:
    - name: dump
      on: "db:backup"
      position: after
      order: 10
      run: "mysqldump --single-transaction \"$DATABASE_URL\" > {{shared_path}}/backups/manual.sql"
```

### Ghi chú riêng của từng framework

**Laravel.** `storage` được share chứ không chỉ ghi được, vì cờ maintenance nằm ở
`storage/framework/down`: thư mục share là thứ mang nó qua cú swap release. Cache
của framework được dựng trong `app:cache:flush`, ở target, không bao giờ lúc build —
`artisan optimize` ghi `bootstrap/cache/config.php`, và khi file đó tồn tại thì biến
môi trường không còn override `.env` nữa, nên cache dựng ở máy khác sẽ mang cấu
hình của máy đó lên production. `worker_control` gửi `queue:restart`, command này
exit 0 với mọi cache store, nên dự án có cache không mang được tín hiệu cần một
cache store bền thì bước này mới có nghĩa. Check `app` ưu tiên `artisan db:show` và
lùi về `migrate:status` trên Laravel 10 trở xuống, nơi `db:show` chưa có; nhánh lùi
exit 1 với dự án chưa có bảng migrations, mà đó là dự án khoẻ mạnh không migration.

**Symfony.** `auto-scripts` của Composer chạy `cache:clear` và `assets:install`,
đúng cho lập trình viên và sai cho deploy: cả hai phải xảy ra ở nơi ứng dụng sống,
nên `build:vendors` truyền `--no-scripts` và `build:assets` chạy `assets:install` ở
target với `--relative` (link nó ghi sau đó resolve được từ bất kỳ độ sâu nào của
docroot, đúng thứ deploy in-place cần). `doctrine:migrations:migrate` nhận
`--allow-no-migration`, vì dự án có thư mục migrations rỗng là dự án khoẻ mạnh.
`var/cache` **không** được share: container đã compile thuộc về một release và một
environment. `symfony_env` (mặc định `prod`) quyết định environment mà deploy chạy
dưới, vì `.env` được commit ghi `APP_ENV=dev` là mặc định phát triển chứ không phải
chỉ thị cho production; giá trị này không được validate, nên gõ sai sẽ dựng sai thư
mục cache. Check `app` ưu tiên `dbal:run-sql` và lùi về `doctrine:query:sql`, chọn
bằng cách hỏi `bin/console` chứ không đoán phiên bản DoctrineBundle.

**WordPress.** Chỉ hỗ trợ layout classic — file core và `wp-content/` ở gốc repo,
không có `composer.json`; layout Bedrock (core trong `vendor/`, docroot `web/`) và
checkout chỉ có content không được hỗ trợ. `wp-config.php` là shared file, và ở lần
deploy đầu `shared/` còn rỗng, nên **`wp-config.php` của target phải được seed trước
khi deploy chạy**, nếu không release sẽ giữ bản của repo — bản trỏ vào database phát
triển. `wp db export` gọi `mysqldump`, nên target cần cả wp-cli lẫn MySQL client.

### Sandbox cấp gì cho các recipe này

Dự án dùng database không phải mặc định của framework thì khai ở tầng project — đó
chính là thứ khiến nó diễn tập được. Ví dụ một dự án Symfony dùng PostgreSQL thay
thế danh sách của recipe chứ không nối thêm:

```yaml
deploy:
  settings:
    sandbox_packages: [postgresql]
    sandbox_extensions: [intl, pgsql, mbstring, xml, curl, zip]
    sandbox_services: [postgresql, redis-server]
    sandbox_tools: [wp-cli]
```

Bốn danh sách này **thay thế** danh sách của recipe chứ không nối thêm: dự án dùng
PostgreSQL thay `[mariadb]`, không chạy cả hai.

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

Maintenance window chỉ được mở khi nó thật sự cần: kích hoạt bằng symlink là một
cú rename nguyên tử nên không có gì đang phục vụ bị ghi đè, và window chỉ xuất
hiện nếu cùng plan đó còn migrate hoặc import cấu hình. Kích hoạt in-place luôn mở
window, vì chính docroot bị ghi đè trong lúc đang phục vụ.

Với dự án Magento, deploy symlink cũng mở window: recipe import cấu hình và chạy
schema upgrade ở mọi lần deploy, nên plan luôn có migrate. Cách còn lại là hỏi
target xem có gì đang chờ không rồi tin câu trả lời — một phép dò mà chính recipe
của công cụ deploy tham chiếu ghi nhận là bỏ sót trường hợp — và một window không
cần thiết chỉ tốn thời gian downtime bằng một lần migration, còn việc đổi schema
mà release đang phục vụ vẫn chạy thì tốn cả site. Thứ giới hạn chi phí là
`deploy.maintenance_timeout` (mặc định 15m) và việc dump database trong window là
tuỳ chọn (`--no-db-backup`).

Window được mở và đóng **trên release đang được phục vụ** (`current`), không phải
trên release đang build. Maintenance mode được đọc từ docroot mà request rơi vào,
nên cờ viết vào release mới sẽ không bảo vệ gì trong lúc `setup:upgrade` đổi schema
mà code đang chạy phụ thuộc vào, và sẽ tắt site ngay khi release đó lên live. Với
symlink, điều đó cũng quyết định window **kết thúc ở đâu**: nó đóng trước cú swap,
vì swap đổi release đang được phục vụ — đóng sau đó sẽ để lại cờ trong release vừa
bị thay, và rollback về release đó là serve maintenance mode cho mọi khách. Với
in-place chỉ có một thư mục duy nhất, nên window vẫn mở xuyên qua lúc ghi đè.
Điều kiện chặn là *ứng dụng* đang được phục vụ (`bin/magento` trong thư mục đang
phục vụ), không chỉ là thư mục: lần deploy đầu chưa có release nào đang phục vụ, và
docroot của target in-place là một git checkout có thể chưa từng được deploy tới —
cả hai đều không có gì để bảo vệ, nên hai bước đó là no-op.

Mỗi framework đóng góp một **recipe** điền những task nó hỗ trợ; task bỏ trống
được báo là skipped, không phải lỗi. Dự án tuỳ biến pipeline bằng **hook** neo vào
task id, alias stage hoặc hook khác:

```yaml
deploy:
  hooks:
    - { name: varnish-purge, on: "publish:activate", position: after, order: 10, run: "varnishadm ban req.url ~ /" }
```

`govard deploy plan` in ra cây thực thi kèm cách hiện thực của từng bước, và nguồn
của từng hook, nên có thể soi vị trí hook mà không cần kết nối tới đâu.

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

### Theo dõi một lần deploy đang chạy

Mặc định, output là một dòng cho mỗi task khi task đó xong: đủ yên cho log CI, và đủ
để thấy bước nào đang chạy. Hai thứ lấp khoảng trống trong lúc một bước đang chạy:

- **heartbeat** mỗi mười giây — `… build:compile still running (42s)` — cho mọi bước,
  bất kể command của nó có in gì hay không. Một `composer install` im lặng và một
  transfer bị treo trông giống hệt nhau nếu thiếu nó, và phân biệt được hai thứ đó quan
  trọng từ rất lâu trước khi timeout nổ;
- **`--verbose`**, stream output của từng command ngay khi nó chạy, thụt vào dưới task
  mà nó thuộc về. Không gom nhóm, không đổi thứ tự: command viết gì bạn thấy đúng cái
  đó, ngay lúc nó viết.

`--json` thắng `--verbose`: khi bật cả hai, stream trở thành no-op và stdout vẫn đúng
một document parse được (`--json` đã chuyển timeline sang stderr). Các transfer bằng
rsync — copy `sync_paths` của in-place và upload artifact — chỉ thêm
`--info=progress2` khi output là terminal *và* `--verbose` đang bật: không có terminal
thì rsync không vẽ lại được dòng tiến độ, nên mỗi cập nhật sẽ thành một dòng log mới
thay vì một dòng đang chạy.

### Dừng một lần deploy

`Ctrl-C` (hoặc `SIGTERM`) huỷ lần chạy thay vì giết tiến trình tại chỗ. Govard báo bước
đó là *the run was interrupted*, rồi áp đúng luật mà một lần thất bại áp: trước
maintenance window thì lock được nhả, vì chưa có gì live thay đổi và lần thử lại không
được phép bị từ chối; khi window đã mở thì lock, thư mục release và record của nó ở lại,
vì release là thứ duy nhất nói target đang dở dang ở đâu. `--resume` sẽ hoàn tất nó.

Huỷ cũng dừng **công việc**, không chỉ sổ sách của govard. Mỗi bước là một chuỗi shell —
<span v-pre>`cd {{release_path}} && composer install …`</span> — nên tiến trình govard khởi động là shell,
còn compile, install hay transfer là con của nó. Bước chạy local nằm trong process group
riêng: group nhận `SIGTERM` khi lần chạy bị huỷ, và thứ gì bỏ qua nó sẽ bị kill ngay khi
command đã dừng trả về. Qua SSH, giết client local không dừng được gì trên máy kia, nên
bước đó ghi lại pid của shell remote — `sshd` cấp cho nó session và process group riêng —
rồi đường huỷ signal đúng group đó qua một kết nối thứ hai, ngắn. Bản ghi được xoá ngay khi bước kết thúc — kể cả bước thay shell của nó bằng
`exec` hay tự đặt `EXIT` trap — và bởi đường teardown khi bước bị huỷ, nên lần chạy
bình thường không để lại gì.

Trên Windows không có process group để signal và govard không tạo job object, nên một
bước bị huỷ chỉ giết shell mà govard khởi động, con của shell đó có thể sống lâu hơn lần
chạy. Huỷ là yêu cầu dừng, không phải bảo đảm rằng bước đã bắt đầu thì đã dừng — hãy
kiểm tra bằng `govard deploy releases` và `status` trước khi chạy lần nữa.

### Chế độ developer và production

`mage_mode` là setting duy nhất đổi *việc deploy làm gì* chứ không phải cách nó làm,
nên đáng để ghi tường minh trong mọi dự án:

| `mage_mode` | `build:assets` có chạy | Dùng khi |
| --- | --- | --- |
| không đặt (mặc định) | có | hành vi production; tương đương `production` |
| `production` | có | target mà static content được compile lúc deploy |
| `developer` | không | target mà Magento sinh static file theo nhu cầu |

Điều kiện chặn của recipe chỉ là so sánh chuỗi, nên giá trị rỗng hành xử y hệt
`production`; chỉ đúng chuỗi `developer` mới bỏ qua bước này. Vì vậy deploy ở
developer mode nhanh hơn vài phút trên một storefront lớn — không hề có
`setup:static-content:deploy` — và ứng dụng compile những gì một trang cần ở request
đầu tiên.

```yaml
deploy:
  settings:
    mage_mode: developer      # staging mà team duyệt; bỏ qua static content
```

Hai hệ quả cần nói thẳng:

- **`developer` là sai trên target production** trừ khi `env.php` của nó bật
  `static_content_on_demand_in_production`. `env.php` của production thường tắt cờ
  này, nên theme mà static file chưa từng được deploy sẽ trả `404` cho chúng. Target
  staging **dùng chung** mà người ta đo hiệu năng cũng nên dùng `production`, vì
  việc sinh theo nhu cầu làm đổi các con số.
- **Split không áp dụng ở developer mode.** `split_static_deployment` chỉ mô tả *cách*
  sắp xếp các lượt static content, mà ở developer mode thì không có lượt nào. Phần
  kiểm chứng của chính recipe cũng theo đó: check static content version cho in-place
  được chặn theo điều kiện file version tồn tại, nên nó pass một cách vô nghĩa khi
  không có gì được deploy.

`govard deploy plan <remote>` cho thấy điều kiện chặn sẽ so sánh với mode nào, trước
khi kết nối tới đâu: bước static content có mặt trong cả hai trường hợp, và command mà
plan in ra mang sẵn phép so sánh, ví dụ `[ developer != developer ]` trên target ở
developer mode.

Mode là một deploy setting, không phải setting của môi trường local: môi trường local
trong `.govard.yml` tự chọn Magento mode cho việc phát triển, và hai bên không nhất
thiết phải khớp nhau. Nó còn mang tính **mô tả**: govard không bao giờ chạy
`bin/magento deploy:mode:set`, nên đặt `developer` ở đây là nói cho deploy biết target
đang chạy gì, chứ không làm target chạy như vậy. Hãy kiểm tra target bằng
`bin/magento deploy:mode:show` rồi đặt setting cho khớp.

### Tách static content

`split_static_deployment` deploy static content của adminhtml và frontend thành
hai lượt thay vì một, với `--area=adminhtml` rồi `--area=frontend`. Lượt admin
dùng `magento_themes_backend` (mặc định là theme admin) và
`static_content_locales_backend`, mặc định lấy theo ngôn ngữ frontend để hai lượt
khớp nhau trừ khi dự án nói khác. Lượt frontend dùng `magento_themes` và
`static_content_locales`.

`static_deploy_options` truyền thêm cờ cho mọi lượt — `--no-parent` cho theme có
theme cha được deploy riêng, `-s standard`, hay `--exclude-theme`. Chuỗi được truyền
nguyên văn, list thì mỗi entry thành một từ:

```yaml
deploy:
  settings:
    static_deploy_options: --no-parent
```

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

`frontend_command` chạy bên trong từng thư mục, trong release, như một command
shell: mặc định nối hai command, nên giá trị một command như
`npx tailwindcss -i input.css -o output.css` cũng viết y hệt. Để `frontend_dir`
trống thì bước này bị bỏ qua — đúng ý cho dự án Luma hoặc theme mặc định.

Dự án có nhiều hơn một theme build bằng Node thì khai báo hết, mỗi thư mục được
build tại chỗ:

```yaml
deploy:
  settings:
    frontend_dir:
      - app/design/frontend/Acme/hyva/web/tailwind
      - app/design/frontend/Acme/other/web/tailwind
```

Các thư mục được build theo thứ tự, mỗi thư mục trong subshell riêng, và lỗi đầu
tiên dừng deploy thay vì để build của theme sau che mất. Mọi entry đều được quote,
nên một entry là đúng một thư mục: path có dấu cách phải viết dưới dạng list, vì
dạng một path được đọc như danh sách tách theo khoảng trắng.

Node cần ở nơi *build*, không phải nơi deploy: với `--artifact-dir`, theme được
build ở job build của CI và `build:frontend` bị bỏ qua trên target, nên image của
job deploy vẫn chỉ cần govard + ssh + rsync.

[Case study triển khai](/vi/workflows/deploy-case-studies) đi qua một theme Hyvä
(case 3), chính theme đó ở developer mode (case 4) và hai theme build bằng Node
(case 5), kèm cấu hình và lệnh sandbox cho từng ca.

### Nhiều store, website và theme

Dự án multi-store deploy một release phục vụ mọi website, nên tập deploy là
**theme × locale**: theme của mọi store view phải có trong `magento_themes` và
locale của mọi store view phải có trong `static_content_locales`. Storefront mà
theme hoặc locale không được build thì không có static file, và production mode
trả `404` cho chúng: Magento chỉ publish lại static resource thiếu khi `env.php`
bật `static_content_on_demand_in_production`, tức là tốn một lần chạy PHP mỗi
request chứ không thay thế được việc deploy.

```yaml
deploy:
  settings:
    # Mọi theme mà store view dùng, kèm locale mà theme đó phục vụ.
    magento_themes:
      Acme/hyva: [en_US, fr_CA]
      Acme/other: [de_DE]
      Magento/luma: [en_US]
    # Không bắt buộc, và mang tính cộng thêm: list này áp cho mọi theme ở trên.
    static_content_locales: [en_US]
```

Locale trong map theme được **cộng vào** `static_content_locales`, và kết quả được
deploy cho mọi theme trong map. Đây là chủ ý và là đặc tính của ứng dụng, không
phải đơn giản hoá: một lần gọi `setup:static-content:deploy` chỉ resolve
`--language` một lần cho cả lượt chạy, nên một lần gọi không thể compile theme A
với bộ locale này và theme B với bộ locale khác. Muốn thu hẹp theo từng theme thì
phải gọi một lần cho mỗi nhóm locale; union là thứ command diễn đạt được, và là
hướng an toàn — thừa locale chỉ tốn thời gian build, thiếu locale thì mất
storefront.

Split vẫn áp dụng: `split_static_deployment` đẩy `magento_themes_backend` (và
`static_content_locales_backend`, mặc định lấy theo locale frontend) sang lượt
adminhtml, còn `magento_themes` sang lượt frontend.

Phần còn lại của pipeline đã lo sẵn cho nhiều website:

- `app/etc/env.php` và `pub/media` được **share** giữa các release, nên cấu hình
  theo scope và media không mất khi swap;
- `app:config:import` áp cấu hình nằm trong `config.php` và `env.php` — nơi cấu
  hình theo scope thuộc về khi nó được version hoá;
- `setup:upgrade`, flush cache và worker control là toàn cục, đúng với việc mọi
  website dùng chung một database.

Điều govard chủ động **không** làm: không bao giờ ghi cấu hình theo store vào
database. Base URL, cấu hình scope và mọi thứ operator đổi trong admin là dữ liệu
của ứng dụng, không phải của release; một deploy ghi đè chúng là một deploy có thể
xoá cấu hình của storefront đang chạy. `deploy.verify.url` kiểm một URL; dự án
multi-store muốn kiểm mọi storefront thì gắn hook vào `verify` và chạy kiểm tra
mình cần.

[Case study triển khai — Ca 6](/vi/workflows/deploy-case-studies#case-6-multi-store-hyva-storefront-with-a-luma-admin) chính là mục này
áp vào một dự án thật: một storefront Hyvä với admin Luma, split được bật, quy tắc
union-of-locales, và hook verify kiểm mọi storefront.

### Quyền ghi và ownership

`writable_dirs` liệt kê những path ứng dụng cần ghi được, và `writable_mode` quyết
định cách cấp quyền:

| Mode | Việc nó làm |
|---|---|
| `chmod` (mặc định) | `chmod -R` với `writable_permissions` (`0775`) |
| `chown` | `chown -R` cho `owner` |
| `chmod+chown` | cả hai |
| `acl` | `setfacl` entry cho cả access *và* default của `owner` |
| `skip` | không làm gì — cho target đã được image hoặc bước provisioning cấp quyền |

`owner` là `user` hoặc `user:group`; hai mode chown và `acl` bắt buộc phải có, vì
owner sai sẽ tạo ra release mà web server không đọc được — tệ hơn là từ chối. Mode
ngoài bảng là lỗi cấu hình (exit 4), bị từ chối ngay lúc validate settings chứ không
phải fail giữa deploy.

`acl` là mode bao luôn những file ứng dụng tạo *về sau*: default ACL được kế thừa,
nên `var/`, `pub/static/` và `generated/` vẫn ghi được sau deploy mà không cần
`chown -R` cả release. Mode này cần `setfacl` trên target, và `deploy check` từ chối
deploy ngay trước khi thư mục release tồn tại nếu thiếu nó.

### Cache, opcache và cú swap symlink

Release flush cache của ứng dụng ngay trong pipeline, nên release mới không bao giờ
phục vụ cache do code cũ dựng. Thứ mà flush cache không chạm tới là trạng thái của
chính PHP: sau cú swap, một worker đã resolve `current` có thể vẫn giữ release cũ
trong `realpath_cache` và file đã compile trong opcache tới `realpath_cache_ttl`.
Đó là cách một deploy trông thành công mà vẫn phục vụ code của release trước.

`settings.runtime_reload_command` là cách được hỗ trợ để xoá trạng thái đó. Nó chạy
như phần cuối của bước `app:cache:flush` — trong window với in-place, sau cú swap với
symlink:

```yaml
deploy:
  settings:
    runtime_reload_command: cachetool opcache:reset && cachetool stat:clear
```

Reset opcache và realpath cache được ưu tiên hơn reload PHP-FPM: reload có thể làm
rớt những request đang bay, đó là lý do recipe PHP-FPM của công cụ deploy tham chiếu
tự cảnh báo đừng reload và chỉ đúng vào cách reset cache này. Setting là một command
shell thô, nên mọi cách tương đương đều dùng được khi user deploy không với tới được
opcache — reload PHP-FPM, hook restart container, hay cùng command đó qua công cụ
khác.

### Composer repository riêng

Release build trên target thì cài dependency ngay trên đó, nên cần credential cho
mọi repository không phải packagist. Ba đường và thứ tự ưu tiên nằm ở *Credential
Composer đến từ đâu*: `COMPOSER_AUTH` từ môi trường deploy được chuyển tiếp tới
bước cài dependency qua standard input — không nằm trong command, nên không lộ
trong process list của target và không lọt vào log deploy — và nó đè lên mọi file
trên target. `shared/auth.json` chỉ được đọc khi dự án liệt kê `auth.json` trong
`shared_files`, tức là có link nó vào release. Govard không lưu credential nào của
riêng nó.

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

### Artifact mang được gì và không mang được gì

Job build chạy đúng recipe mà server build chạy, trừ những bước hỏi chính ứng dụng.
Đáng chú ý nhất là deploy static content: `setup:static-content:deploy` đọc cấu
hình store, website và locale từ database, nên một máy không có ứng dụng, không có
`app/etc/env.php` và không có database thì không chạy được — khai báo theme và
locale tường minh cũng không đổi được điều đó, vì store nó hỏi vẫn nằm trong
database.

Recipe đánh dấu những bước đó, và artifact mode để chúng **ở lại trong deploy**:
target chạy chúng sau khi `deploy:artifact` đã bung artifact — đúng chỗ mà server
build chạy chúng. Phân chia thành ra là:

| Chạy trên máy build | Chạy trên target |
| --- | --- |
| Composer install, patches, DI compile, build frontend (node) | static content, `setup:upgrade`, import cấu hình, flush cache, verify |

Job deploy vẫn không cần toolchain riêng — chỉ cần govard, ssh và rsync; target
chạy các bước ứng dụng qua SSH như trước giờ.

Artifact cũng không bao giờ mang những path mà recipe khai là **shared**
(`shared_files`, `shared_dirs`): chúng thuộc về target, và `govard deploy build`
in ra từng path bị loại. Nếu không, một artifact dựng trên máy tình cờ có
`app/etc/env.php` riêng sẽ đè lên cái của target — file thường thay thế symlink mà
`deploy:shared` vừa tạo — và release sẽ chết bằng đúng lời của ứng dụng:
`Connection "default" is not defined`.

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

Khi dự án đã mô tả môi trường trong `.govard.yml`, cặp job viết tay trên có thể
sinh tự động: [`govard ci generate`](/vi/workflows/ci-generate) render cả
pipeline với ship một job cho mỗi remote (build và deploy chung một workspace
nên artifact không chạy qua lại giữa các job) kèm mode `--check` cho CI.

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

### Publish in-place cần `sync_paths`

Kích hoạt in-place reset docroot về đúng revision, copy các `sync_paths` đã cấu hình
từ release đã build vào đó (kèm `--delete`), và ghi `pub/static/deployed_version.txt`
cuối cùng. `git reset --hard` để nguyên những thư mục gitignored của lần deploy trước,
nên **path không được copy là path site giữ lại từ release cũ** — code mới nằm trên
`vendor/`, `generated/`, `pub/static/` cũ mà vẫn báo deploy thành công.

Recipe Magento đã ship sẵn danh sách mà chiến lược này cần, và nên giữ nguyên:

```yaml
deploy:
  settings:
    sync_paths: [vendor, generated, pub/static/adminhtml, pub/static/frontend]
```

Ba quy tắc engine áp dụng cho bất cứ danh sách nào dự án cấu hình:

- **path mà release không build ra thì bị bỏ qua, không làm fail** — `generated/` chỉ
  có sau `setup:di:compile`, `pub/static/adminhtml` chỉ có khi area admin được deploy;
  bước kích hoạt in ra việc bỏ qua đó;
- **path mà release link từ `shared/` thì không được copy** — `deploy:shared` link nó
  bằng symlink tương đối theo release, sang docroot ở độ sâu khác là trỏ sai chỗ, nên
  docroot giữ bản của chính nó và bước đó nói rõ. Đây là lý do `pub/static` được ghi
  bằng hai con đã build thay vì ghi cả thư mục: `pub/static/_cache` là shared;
- **path shared nằm trong một entry được sync thì bị loại khỏi bản copy** thay vì bị
  xoá, nên dự án ghi `pub/static` vẫn giữ `_cache` của docroot.

`sync_paths: []` vẫn hợp lệ và nghĩa là "không copy gì"; preflight sẽ cảnh báo, vì đó
phải là quyết định có chủ ý chứ không phải thiếu sót.

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

`deploy.command_timeout` (mặc định 30m) chặn mọi bước còn lại, và là thứ cần nâng
đầu tiên khi deploy timeout: một lần `composer install` **nguội** của dự án lớn —
vài trăm package, repository private clone qua mạng — có thể lâu hơn mức đó trên
target mới, và lỗi hiện ra là `command timed out` ở đúng bước đó. Bước bị timeout
không phải trường hợp đặc biệt: lock được trả lại nếu lần chạy chưa tới maintenance
window, record ghi rõ bước nào dừng, và lần chạy lại tiếp tục với Composer cache đã
ấm trên target.

```yaml
deploy:
  command_timeout: 90m
```

Một lần deploy lỗi vẫn giữ thư mục release và record của nó. Lỗi ở đâu quyết định
số phận của lock: lỗi trong `prepare` hoặc `build` sẽ nhả lock vì chưa có gì live
thay đổi, nên chỉ cần sửa lỗi rồi deploy lại — còn lỗi từ `publish` trở đi thì giữ
lock, vì target có thể đang dở dang, và đường đi tiếp là `govard deploy <remote>
--resume` để tiếp tục release mới nhất chưa xong thay vì tạo release mới.
`--from <task>` bắt đầu từ một task hoặc hook chỉ định, và `govard deploy unlock`
giải phóng lock do lần lỗi để lại.

Có thể `--resume` bao nhiêu lần cũng được, và lần nào cũng tiếp tục đúng release
đó. Bước mà lần chạy trước đã thành công sẽ không chạy lại, và record giữ nguyên
trạng thái `ok` mà lần đó ghi, nên lần resume hiện bước đó là `already done in an
earlier run` và không có gì bị build hai lần. Thư mục release chỉ bị từ chối khi nó
không mang record của chính govard cho release đó: thư mục do công cụ khác tạo được
bảo vệ, còn release dở dang của govard thì được đi tiếp chứ không bị chặn.

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
govard deploy sandbox up --profile full --php 8.4   # database, cache, web server, PHP 8.4
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

Profile `php` và `full` còn có **web tier**: nginx phục vụ served path cộng
`stack.web_root` của dự án (`/pub` với Magento), và PHP-FPM chạy bằng chính user
deploy — nên ứng dụng ghi được những thư mục mà `deploy:writable` giao cho user đó.
`up` publish luôn cổng đó trên loopback và trỏ `deploy.verify.url` của remote
sandbox vào nó, nghĩa là deploy vào sandbox diễn tập **toàn bộ** pipeline, kể cả
bước kiểm tra HTTP — bước mà một target không có web server không bao giờ chạy được.

Kiểm tra đó là thật: target chưa phục vụ được sẽ fail ở bước cuối với đúng mã HTTP
mà nó trả về (`verify http: http://127.0.0.1:PORT/ returned HTTP 403`) — đó là
check đang làm việc, không phải lỗi. Có ứng dụng (env.php, database, search engine)
thì nó pass; `--no-verify` để tắt cho lần diễn tập chỉ cần dừng ở mức file.

Profile `full` start sẵn database và cache, và recipe Magento khai cả hai, nên
`env.php` của target có thể trỏ `127.0.0.1` cho MariaDB và Redis/Valkey — đúng hình
dạng server thật — thay vì phải sửa tay sang file cache. Profile `basic` không có
gì trong số đó và không quảng cáo verify URL.

`--php` chọn series PHP mà image cung cấp, ví dụ `--php 8.4`; không có thì image
giữ version của distribution gốc. Series lấy từ repository sury và kéo theo `php`
binary, các extension, `php_bin` và `php_version` mà remote khai — nên dự án có
`composer.lock` đòi PHP mới hơn image gốc vẫn diễn tập được đúng interpreter mà
target thật chạy. Series nằm trong image tag, nên đổi series là build image khác
chứ không tái dùng image cũ.

`--docroot` định hình target để chiến lược publish resolve theo đúng thứ bạn muốn
kiểm chứng: `absent` hoặc `symlink` chọn cú swap nguyên tử, `real` chọn in-place.
Việc định hình chỉ xảy ra khi target được tạo và khi bạn nói rõ hình dạng muốn có —
vì `up` còn là cách khởi động lại sandbox đang dừng và cách refresh mirror trước khi
deploy revision kế tiếp, và cả hai đều không được phép làm mất ứng dụng đang phục
vụ. `reset` thì luôn định hình: xoá thư mục deploy rồi dựng lại là việc của nó.
`down` xoá container và remote mà nó đã ghi; `--purge` xoá thêm image, khoá và
mirror.

Một sandbox đã tồn tại được mô tả bằng chính nó, không bằng flag của lệnh vừa gọi
tới: `up` báo đúng profile và series PHP mà container được build, cùng image thật
của nó — nên lần `up` sau không có `--php` không xoá mất series, và không mô tả một
sandbox `full` thành profile mặc định. Đòi một profile hoặc series khác sẽ bị từ
chối kèm đúng flag thay đổi được nó (`--recreate`), thay vì dán nhãn mới cho một
container mà image vẫn là image cũ. `up` cũng chờ một lần đăng nhập thật trước khi
báo sandbox sẵn sàng — cổng đã publish và chấp nhận kết nối chưa phải là một target
deploy được. Riêng block remote `sandbox` được ghi lại từ trạng thái
container ở mọi lần `up` (host, port, path, branch, mirror, verify URL và các setting
mà profile hàm ý), nên sửa tay trong đó không giữ được — hãy đặt những gì cần giữ
vào cấu hình của chính dự án.

Một lần diễn tập chỉ đầy đủ bằng credential và ứng dụng mà target có. Package
`git` cần khoá và `known_hosts` *bên trong container*, và credential chỉ nằm trong
shell profile của bạn chính là loại có thể đè lên `auth.json` đang chạy tốt của dự
án rồi làm hỏng build (xem *Credential Composer đến từ đâu*). Ứng dụng thì cần
`shared/app/etc/env.php` trỏ tới database, cache và session mà target kết nối
được, cộng thêm search engine được hỗ trợ nếu dự án dùng: thiếu chúng thì
`build:assets` dừng ở `The default website isn't defined` và `db:migrate` dừng ở
`Your current search engine, 'MySQL', is not supported`. Đó là prerequisite của
target, không phải của engine — một server chưa từng chạy ứng dụng thì không
publish release lên được, và sandbox từ chối giả vờ ngược lại. Đặt credential và
`env.php` vào container (`docker exec`, hoặc mount file) rồi chạy lại `govard
deploy --remote sandbox --yes`; bước hỏng sẽ đi tiếp từ release directory sạch và
Composer cache được giữ nguyên.

## Một kết nối cho mỗi target

Mọi command trong một lần chạy dùng chung một kết nối SSH cho mỗi target
(`ControlMaster` với socket trong `~/.govard/ssh/`, giữ 60 giây sau command cuối).
Pipeline Magento chạy khoảng hai mươi command, nên handshake và authentication
trả một lần thay vì hai mươi lần — và khoảng một nửa số đó chạy bên trong
maintenance window, nơi tiết kiệm một giây là bớt một giây downtime. Socket nằm
ngay trên máy chạy deploy và tự hết hạn; trên target không để lại gì.

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
