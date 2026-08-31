---
title: Tài liệu tham khảo lệnh CLI Govard
description: Tài liệu đầy đủ về các lệnh CLI, alias và shortcut của Govard để quản lý môi trường phát triển Docker cục bộ.
---

# Lệnh CLI (CLI Commands)

Đây là tài liệu tham khảo chính thức cho các lệnh CLI của Govard.

---

## Các phím tắt và Tên viết tắt (Aliases and Shortcuts)

### Phím tắt quản lý Lifecycle gốc

| Phím tắt | Lệnh tương đương |
| :--- | :--- |
| `govard up` | `govard env up` |
| `govard down` | `govard env down` |
| `govard restart` | `govard env restart` |
| `govard ps` | `govard env ps` |
| `govard logs` | `govard env logs` |

### Tên viết tắt của các lệnh

| Tên viết tắt | Lệnh đầy đủ |
| :--- | :--- |
| `govard boot` | `govard bootstrap` |
| `govard cfg` | `govard config` |
| `govard dbg` | `govard debug` |
| `govard gui` | `govard desktop` |
| `govard diag` | `govard doctor` |
| `govard ext` | `govard extensions` |
| `govard prj` | `govard project` |
| `govard rmt` | `govard remote` |
| `govard sh` | `govard shell` |
| `govard snap` | `govard snapshot` |

### Viết tắt của lệnh `govard tool`

| Tên viết tắt | Lệnh đầy đủ |
| :--- | :--- |
| `govard tool mr` | `govard tool magerun` |

### Viết tắt của lệnh `govard sync`

- `--from` là viết tắt của `--source`
- `--to` là viết tắt của `--destination`
- `-e, --environment` là tùy chọn môi trường nguồn được tiếp tục hỗ trợ

---

## 🌿 Các lệnh môi trường (Environment Commands)

### `govard audit`

Chạy audit dự án có session được lưu bền vững. `lint` chạy phân tích tĩnh;
Magento 2 và Mage-OS còn khai báo thêm check `profiler` CSV stock do Govard tự
điều phối. Các giai đoạn sau sẽ bổ sung browser job mà không đổi semantics
session.

```bash
govard audit run
govard audit run --checks profiler --url 'https://shop.test/category.html?product_list_limit=48'
govard audit run --checks lint,profiler --url 'https://shop.test/'
govard audit diff --base origin/master
govard audit rerun --session 20260816T010203Z-a1b2c3d4
govard audit status --session 20260816T010203Z-a1b2c3d4
govard audit result --session 20260816T010203Z-a1b2c3d4 --run run-0001
govard audit cleanup --older-than 168h
```

`run` mặc định dùng `--scope project`, `--checks lint`,
`--lint-provider govard`, `--mode auto` và `--lint-jobs min(nproc,4)` (mặc định `4` trên host thường, kẹp `2–8`). Format mặc định
`text` stream tiến trình trực tiếp như `vendor/bin/phpcs`/`vendor/bin/phpstan` —
giai đoạn `validate` → `prepare` → `phpcs`/`phpstan`, trạng thái cache
(`cold`/`warm`/`bypassed`) và `glint: php X.Y analyzed` hiện ngay khi chạy —
rồi mới in bản tóm tắt: kết luận trước (PASSED/FAILED/CANCELLED), scope, thời
gian, môi trường, kết quả từng PHP kèm findings và gợi ý `What next` trỏ tới
report đã lưu cùng lệnh rerun chính xác. Trên TTY tương tác (và khi chưa đặt
`NO_COLOR`) findings được tô màu (tool xanh nhạt đậm, rule vàng,
`path:line:col` cyan) và hiện toàn bộ như CLI gốc; khi pipe/redirect thì giữ
plain và chỉ hiện tối đa mười finding (`... and N more`) để log CI gọn. Một run
hoàn tất nhưng check không pass (failed/cancelled) vẫn in tóm tắt rồi mới thoát
khác 0 để script/CI nhận biết được. `--format json` chỉ ghi một JSON object
không decoration ra stdout cho AI Agents; diagnostic, log backend và dòng
`ERROR audit run … reported failed checks` nằm ở stderr hoặc `govard-lint.log`
đã lưu. Chỉ chấp nhận `text`/`json`; `--lint-jobs` phải từ 1 đến số PHP version
framework khai báo.

`profiler` yêu cầu `--url` tuyệt đối HTTP(S) tường minh ở run đầu tiên và target
phải là toàn bộ Govard project (standalone module lẫn module-only target đều bị
từ chối trước khi có bất kỳ thay đổi runtime nào). URL chính xác được lưu trong
run và được `audit rerun` tái sử dụng, nên các run before/after bắt cùng một
trang. Govard dùng `MAGE_PROFILER=csvfile` stock của Magento; không cài Magento
module, không phụ thuộc repository/image bên thứ ba, và không sửa
`app/etc/env.php`.

Chạy `govard env up` với phiên bản Govard hiện tại trước lần capture đầu tiên
để Compose stack đã render mount thư mục cấu hình custom thuộc sở hữu project.
Trong quá trình capture có lease bảo vệ, Govard tạo nguyên tử một include với
tên duy nhất, reload server đang active, thực hiện một HTTP GET có giới hạn,
thu `var/log/profiler.csv` qua PHP container, rồi khôi phục lại cả include lẫn
CSV runtime. Nginx nhận tham số FastCGI tạm thời bên trong PHP location của
Magento. Chế độ Apache dùng dịch vụ `web`; hybrid cấu hình và reload dịch vụ
`apache` của nó thay vì nginx. CSV thu được được lưu tại
`runs/<run-id>/artifacts/profiler/profile.csv` kèm digest SHA-256.

Ví dụ bản tóm tắt của `govard audit run`:

```
== Audit run run-0001 / session 20260822T005406Z-14bd9570 ==
  Status:      FAILED
  Scope:       project
  Duration:    4.5s
  Environment: magento2 | nginx | Govard 1.63.0

  Checks
    - lint - failed - 4.5s - provider govard
      PHP 8.5 | failed | 3.9s | cache cold | 12 findings
      - phpcs Squiz.Classes.ClassFileName app/code/Acme/Catalog/Model/Item.php:12: Class name is not camel case

  What next
  Full findings: ~/.govard/audit/<project-id>/sessions/<session-id>/runs/run-0001/report.json
  Re-run:        govard audit rerun --session <session-id>
```

Mỗi lần chạy tạo
`~/.govard/audit/<project-id>/sessions/<session-id>/manifest.json` và ghi result
atomically tại
`~/.govard/audit/<project-id>/sessions/<session-id>/runs/<run-id>/audit-result.json`,
kèm `report.json` của chính provider. `rerun`, `status`, `result` luôn cần đúng
`--session` (và `result` cần thêm `--run`); Govard không tự chọn session mới
nhất.

`rerun` không kèm `--checks` sẽ lặp lại đúng bộ check của lần chạy gần nhất
trong session đó — kể cả URL profiler đã lưu — thay vì rơi về mặc định chỉ lint.
Truyền `--checks` thì chạy lại đúng những gì được yêu cầu; cả hai dạng chỉ dựng
lại những backend mà bộ chọn đó cần.

#### Target mode

`--mode` quyết định phạm vi được phân tích. `auto` (mặc định) tự phân loại thư
mục hiện tại:

| Mode | Được chọn khi | Phạm vi phân tích |
|------|---------------|-------------------|
| `project` | Thư mục nằm trong một Magento project root (có `bin/magento` và một Composer requirement của Magento) và không nằm trong module nào | Toàn bộ project |
| `module_in_project` | Thư mục là một module — qua `etc/module.xml` (cách khai báo module `app/code`) hoặc Composer package type `magento2-module` — nằm bên trong một Magento project | Chỉ module đó, toàn bộ project được mount read-only để autoloader phân giải được |
| `standalone` | Thư mục là một module và không có Magento project nào ở cấp trên | Chỉ module đó; dependency được cài vào worktree tạm và chỉ được scan lấy symbol |

`--mode project`, `--mode module_in_project` và `--mode standalone` buộc một
phân loại cụ thể và sẽ lỗi khi thư mục không thỏa điều kiện.

```bash
# project: chạy từ project root của Magento
cd ~/projects/storefront
govard audit run

# module_in_project: chạy từ trong một module ở app/code
cd ~/projects/storefront/app/code/Acme/Catalog
govard audit run

# module_in_project: chạy từ trong một package ở vendor (Composer type magento2-module)
cd ~/projects/storefront/vendor/acme/module-catalog
govard audit run

# standalone: chạy từ một module không có Magento project nào ở cấp trên
cd ~/work/module-catalog
govard audit run --php 8.1,8.5
```

Mỗi lệnh trên tự nhận diện mode từ thư mục hiện tại — `--mode` chỉ cần dùng khi
muốn buộc hoặc từ chối một phân loại cụ thể (ví dụ `--mode project` sẽ lỗi khi
chạy ngoài project root thay vì tự phân loại lại).

#### Phiên bản PHP

Lint image cung cấp `7.4`, `8.0`, `8.1`, `8.2`, `8.3`, `8.4` và `8.5`.

- Target `project` và `module_in_project` chỉ phân tích đúng một phiên bản:
  `stack.php_version` đang active của project. Cả bảy phiên bản đều hợp lệ ở đây.
  `--php` chỉ được chấp nhận khi trùng với phiên bản active đó, và run sẽ bị từ
  chối nếu container ứng dụng đang chạy báo PHP khác với cấu hình.
- Target `standalone` chấp nhận `8.1` đến `8.5` và mặc định chạy cả năm. `7.4` và
  `8.0` **không** dùng được cho standalone module và bị từ chối trước khi bất kỳ
  image nào được pull, build hay chạy.

Phiên bản bị từ chối sẽ lỗi với thông báo `unsupported_php:` và không thực hiện
bất kỳ công việc container nào.

#### Đường dẫn được quét

Cả hai analyzer native đều bỏ qua các cây không bao giờ chứa mã shipped:
`vendor/`, `generated/`, `var/`, `pub/static/` và `pub/media/`. Bỏ qua các
cây nội dung người dùng giúp run toàn dự án nhanh hơn trên các store nhiều
nội dung.

Vì `pub/media` bị analyzer bỏ qua nhưng lại chính là nơi webshell được upload,
mỗi phiên bản PHP được phân tích còn chạy thêm một phase **media guard**: quét
theo tên trong `pub/media` để tìm file `.php`, `.phtml` và `.pht`. Mỗi file
tìm thấy được báo thành finding `M2-LINT-MEDIA` với đường dẫn relative từ target
root, và phase đó làm run thất bại. Phép quét này chỉ mất mili giây dù media lớn
hàng GB; nó chỉ đọc tên file, không bao giờ đọc nội dung file vào report.

#### Provider

`--lint-provider` chọn lint backend. `govard` (mặc định) là backend native do
Govard sở hữu: nó chạy lint image riêng của Govard, với build context được nhúng
trong chính binary Govard. Bản release ghim image đó theo digest bất biến; khi
image đã ghim không pull được hoặc label không khớp context nhúng, Govard sẽ
build context nhúng ngay tại máy và run vẫn tiếp tục. Đường đi mặc định này không
liên quan tới registry private hay credential bên ngoài nào.

Mọi giá trị khác phải trùng tên một entry trong `audit.lint.external_providers`
của cấu hình dự án (xem
[Cấu hình](./configuration.md#audit-lint-providers)). External provider không bao
giờ là fallback cho backend native và không bao giờ được suy diễn: tên lạ là lỗi,
và lỗi của native vẫn là lỗi của native. Target standalone không có cấu hình dự
án nên chỉ dùng được `govard`.

#### Cache

State lint tái sử dụng nằm ở `~/.govard/cache/audit/lint/<target-id>/` và có chủ
đích **không** bị `audit cleanup` xóa — lệnh đó chỉ dọn session đã lưu. Mỗi target
giữ một cache generation cho mỗi toolchain identity (image, runner, PHP matrix,
analyzer policy). Trong một generation:

- Thay đổi `composer.json`, `composer.lock` hoặc một analyzer ruleset
  (`phpcs.xml`, `phpstan.neon` và các biến thể `.dist`) sẽ loại bỏ analyzer state
  đã cache nhưng giữ Composer download cache còn nóng, nên đổi lock không buộc
  tải lại toàn bộ dependency.
- `--no-lint-result-cache` bỏ qua analyzer state cho một run và được báo lại với
  cache state `bypassed`. Composer download cache vẫn được giữ.

Evidence của mỗi run lưu cache state (`cold`, `warm` hoặc `bypassed`) kèm lý do
theo từng phiên bản PHP, cùng image digest bất biến, toolchain digest và timing
của từng phase.

#### Credential và cancellation

Composer credentials tại `~/.composer/auth.json` được mount read-only khi file
tồn tại, và được link vào một Composer home riêng bên trong container — không bao
giờ bị copy, log hay ghi vào report. Forward SSH agent là opt-in tuyệt đối qua
`--allow-lint-ssh-agent`; không có flag đó thì `SSH_AUTH_SOCK` không bao giờ được
forward dù host có set. Source tree luôn được mount read-only.

Hủy một run sẽ stop lint container rồi remove nó, và run được báo là cancelled
chứ không phải lỗi hạ tầng.

`diff` lưu base ref trong manifest, nhưng lint hiện vẫn phân tích toàn bộ target;
evidence vì thế báo `effective_scope: project`.

### `govard audit toolchain`

Quản lý lint image dùng chung cho cả máy. Các lệnh này không cần chạy bên trong
một Govard project và không bao giờ gọi external lint provider.

```bash
govard audit toolchain status
govard audit toolchain pull
govard audit toolchain build
```

- `status` chỉ kiểm tra image local — không pull, không build — và báo context
  digest nhúng của bản build này, reference official đã ghim, official image đó
  có sẵn và đã verify label hay chưa, và local build đã tồn tại hay chưa. Khi
  chưa có gì dùng được, nó cũng in ra nên chạy lệnh nào tiếp theo.
- `pull` chỉ resolve official image đã ghim. Nó không build, nên khi đường đi
  official không dùng được thì lệnh báo lỗi thay vì âm thầm tạo image local. Bản
  build không ghim digest nào thì không có gì để pull và sẽ nói rõ điều đó.
- `build` chỉ build context nhúng và không pull. Image kết quả là content
  addressed, nên image đã có cho cùng context digest sẽ được dùng lại nguyên
  trạng.

### `govard init`

Phát hiện framework của dự án và tạo cấu hình `.govard.yml`.

```bash
govard init
govard init --framework magento2
govard init --framework custom
govard init --migrate-from warden
```

Khi di chuyển từ Warden, lệnh `govard init --migrate-from warden` tự động ánh xạ `WARDEN_TABLE_PREFIX` sang cấu hình `table_prefix` của Govard cho các dự án Magento 2, Magento 1 và OpenMage.

### `govard bootstrap`

Chạy các quy trình khởi tạo dự án khi clone hoặc cài đặt mới hoàn toàn.

```bash
govard bootstrap
govard bootstrap --clone --environment staging --yes
govard bootstrap --framework magento2 --fresh --framework-version 2.4.9
govard bootstrap -e staging --no-pii --no-noise
```

**Lựa chọn chế độ (Mode selection):**
- `--fresh` + `--framework` + `--framework-version` — cài đặt mới hoàn toàn qua scaffolder của framework.
- `--clone` + `--environment` — rsync toàn bộ mã nguồn từ một remote server.

**Lựa chọn nguồn (Source selection):**
- `-e, --environment` — tên của remote nguồn; chấp nhận các tên chuẩn (`staging`, `production`, `dev`) cũng như các định danh tùy chỉnh (`qa`, `preprod`, `demo`, `client-uat`).
- `--remote` — tên viết tắt của `--environment`.
- `--db-dump` — import database trực tiếp từ một đường dẫn file SQL local.

**Các bộ lọc hiệu năng & bảo mật dữ liệu:**

| Cờ (Flag) | Tác dụng |
| :--- | :--- |
| `-N, --no-noise` | Loại bỏ các dữ liệu rác/tạm thời (logs, sessions, cache tags, lịch sử cron) |
| `-S, --no-pii` | Loại bỏ các dữ liệu cá nhân nhạy cảm (thông tin khách hàng, đơn hàng, tài khoản admin, password) |
| `--delete` | Xóa các file ở đích nếu không tồn tại ở nguồn |
| `--no-compress` | Tắt nén khi chạy rsync |
| `-X, --exclude` | Các pattern loại trừ rsync tùy chỉnh (có thể lặp lại nhiều lần) |
| `--no-db` | Bỏ qua bước import database |
| `--no-media` | Bỏ qua bước đồng bộ file media |
| `--media [mode]` | Chế độ đồng bộ media (`none`, `minimal`, `optimized`, `catalog` (Magento), `all`) |
| `--no-composer` | Bỏ qua việc chạy `composer install` |
| `--no-admin` | Bỏ qua bước tạo tài khoản admin (chỉ áp dụng cho Magento 2) |
| `--no-stream-db` | Sử dụng một file tạm local để truyền DB thay vì stream trực tiếp |
| `--no-up` | Bỏ qua bước khởi động container local trước khi chạy bootstrap |
| `--code-only` | Chỉ clone code (bỏ qua DB/media, kèm `--clone`) |
| `--fix-deps` | Chạy hook `fix-deps` trước khi bootstrap |
| `--framework-version` | Phiên bản framework cho cài mới (vd. `2.4.9` cho Magento) |

Đối với các dự án Magento 2/Mage-OS, Magento 1/OpenMage hoặc PrestaShop có thiết lập `table_prefix`, các bộ lọc bảo mật DB sẽ tự động áp dụng chính xác cho các bảng có tiền tố tương ứng.

**Các cờ đặc thù của Magento:**

| Cờ | Tác dụng |
| :--- | :--- |
| `--include-sample` | Cài đặt dữ liệu mẫu (cho cài đặt mới) |
| `--hyva-install` | Tự động cài đặt theme Hyva |

**Xem trước kế hoạch & Xác nhận:**
- `--plan` — hiển thị kế hoạch thực thi rồi thoát, không chạy thực tế.
- `-y, --yes` — bỏ qua bước xác nhận tương tác (tiện lợi cho CI/non-interactive).

### `govard env`

Quản lý lifecycle của dự án và là wrapper của Docker Compose.

```bash
govard env up
govard env start
govard env stop
govard env restart
govard env down
govard env ps
govard env logs php -f
govard env pull
govard env build
govard env cleanup
```

**Các cờ của `govard env up`:**

| Cờ | Tác dụng |
| :--- | :--- |
| `--pull` | Tải về các image mới nhất trước khi chạy |
| `--fallback-local-build` | Build các image bị thiếu ở local (mặc định `true`) |
| `--remove-orphans` | Xóa các container không còn trong cấu hình (orphaned) |
| `--quickstart` | Đường dẫn khởi động nhanh nhất (chỉ dịch vụ tối thiểu) |
| `--update-lock` | Tự động cập nhật `govard.lock` nếu phát hiện sai lệch |
| `--no-tuning` | Bỏ qua các prompt cấu hình tự động cho framework |
| `--profile <name>` | Dùng profile cụ thể (`.govard.<name>.yml`) cho lần chạy này |
| `--force-recreate` | Tạo lại container dù config không đổi |

`--profile` là cách inline thay cho `govard config profile switch`; vẫn cần `govard env up` để áp dụng.

**Hành vi của `govard env pull`:**

Image được pull từng cái một. Nếu một image không thể pull (bị xóa khỏi
registry, tag không được hỗ trợ, lỗi mạng), Govard vẫn pull tiếp các image
còn lại và build locally các image do Govard quản lý thay vì dừng toàn bộ
quá trình. Dùng `--no-fallback` để tắt cơ chế build local thay thế.

Image search: tag `elasticsearch`/`opensearch` do Govard quản lý theo
phiên bản minor (vd. `7.17`), trong khi `search_version` trong `.govard.yml`
chấp nhận cả minor (`7.17`) lẫn patch đầy đủ (`7.17.28`). Version patch
được pull nguyên trạng khi có trên registry; nếu không thì dùng image minor
và bản build local fallback sẽ nhắm đúng bản upstream thật gần nhất.

**Các file được render lại khi chạy `env up`:**
- `~/.govard/compose/<project-hash>.yml`
- `~/.govard/nginx/<project>/default.conf`
- `~/.govard/apache/<project>/httpd.conf`
- `~/.govard/nginx/<project>/mage-run-map.conf`

**Các cờ của `govard env down`:**
- `-v, --volumes` — xóa các docker volume dữ liệu đi kèm.
- `--rmi local` — xóa các image local được dựng cho dự án.

### `govard frontend`

Quản lý runtime frontend development được dự án sở hữu cho các dự án hỗ trợ
`stack.features.frontend_sync: true`. Hãy khởi động ứng dụng bằng
`govard env up` trước. `env up` không cấp phát container BrowserSync,
LiveReload, Grunt, Tailwind watcher hay HTML injection; tài nguyên frontend chỉ
tồn tại từ lúc chạy `frontend start` đến khi chạy `frontend stop`.

```bash
govard frontend start
govard frontend logs -f
govard frontend logs watch-vendor-theme -f
govard frontend stop
```

`start` chỉ render và khởi động các dịch vụ Compose frontend riêng biệt
(`sync`, mọi `watch-<theme>` được phát hiện, và `inject`), sau đó đợi
BrowserSync/Luma và mọi watcher được phát hiện ở trạng thái healthy. Sau khi
health check thành công, lệnh đăng ký runtime đang hoạt động qua Caddy Admin
API. Cả hai chế độ đều expose 1 path hẹp cho client asset trên đúng domain của
ứng dụng, đồng thời chạy 1 proxy HTML-injection "che" route ứng dụng (khớp mọi
path) để script client xuất hiện trên trang thật mà không cần sửa file dự án
hay theme: Hyva expose `/browser-sync/*` ở cổng 3000 và inject
`<script src="/browser-sync/browser-sync-client.js"></script>`; Luma expose
`/livereload/*` ở cổng 35729 và inject
`<script src="/livereload/livereload.js?snipver=1&port=443&path=livereload/livereload"></script>`.
Cả hai injector chỉ buffer response HTML, mọi thứ khác đi qua nguyên vẹn.

`stop` xóa các route Caddy (kể cả proxy injection) trước khi chỉ xóa các dịch
vụ frontend; dependency volume vẫn được giữ lại. Sau khi xóa, route gốc của
ứng dụng tự động nhận lại toàn bộ traffic. Nếu đăng ký Caddy thất bại trong
`start`, Govard xóa các dịch vụ frontend vừa khởi động để không còn runtime ẩn
tiếp tục tiêu tốn tài nguyên. `logs` chỉ nhận service frontend đã được phát
hiện; bỏ qua service để dùng `sync`.

### `govard svc`

Quản lý các dịch vụ toàn cục dùng chung (proxy, Mailpit, PHPMyAdmin, Portainer).

```bash
govard svc up
govard svc restart --no-trust
govard svc logs --tail 50
govard svc sleep
govard svc wake
```

> **Portainer** có thể truy cập tại `https://portainer.govard.test`
> Đăng nhập mặc định: `admin` / `AdminGovard123$`

### `govard domain`

Quản lý các domain phụ local cho dự án hiện tại.

```bash
govard domain add brand-b.test
govard domain remove brand-b.test
govard domain list
```

### `govard status`

Liệt kê tất cả các môi trường Govard đang chạy trong toàn bộ workspace của bạn.

```bash
govard status
```

### `govard desktop`

Khởi chạy ứng dụng Wails desktop.

```bash
govard desktop
govard desktop --dev
govard desktop --background
```

Xem tài liệu [Ứng dụng Desktop](/vi/workflows/desktop-app) để biết thêm chi tiết.

---

## 🛠️ Các lệnh phát triển (Development Commands)

### `govard shell`

Mở terminal kết nối trực tiếp vào bên trong container ứng dụng.

```bash
govard shell
govard shell --no-tty
```

- PHP frameworks → Vào container `php` tại thư mục `/var/www/html`
- Node-first frameworks (Next.js, Emdash) → Vào container `web` tại thư mục `/app`

### `govard debug`

Quản lý trạng thái hoạt động và các session của Xdebug.

```bash
govard debug status
govard debug on
govard debug off
govard debug shell
```

Các request chỉ được định tuyến tới `php-debug` khi cookie `XDEBUG_SESSION` trùng khớp với giá trị của `stack.xdebug_session`.

### `govard test`

Khởi chạy các công cụ test bên trong container ứng dụng.

```bash
govard test phpunit
govard test phpstan
govard test mftf
govard test unit
govard test integration
```

`unit` là alias chạy `phpunit` nhanh; `phpstan` fallback `--level=0` với `app/code`+`app/design` (Magento 2) hoặc `app`+`src` khi chưa có `phpstan.neon` riêng.

### `govard custom`

Chạy các lệnh tùy chỉnh được cấu hình trong `.govard/commands` hoặc `~/.govard/commands`.

```bash
govard custom list
govard custom hello
govard custom deploy -- --dry-run
```

### `govard project`

Xem và quản lý các dự án Govard đã đăng ký trên hệ thống.

```bash
govard project list
govard project list --orphans
govard project open billing
govard project delete demo
govard project delete --yes demo
```

::: warning CẢNH BÁO
Lệnh `govard project delete` mặc định sẽ xóa hoàn toàn các volume database của dự án đó. Mã nguồn của dự án **không bao giờ** bị xóa.
:::

**Quy trình xóa dự án:**
1. Chạy các lifecycle hook `pre-delete`.
2. Thực thi lệnh `docker compose down -v` (xóa container + volume).
3. Hủy đăng ký các domain trên proxy.
4. Xóa thông tin dự án khỏi registry (`projects.json`).
5. Chạy các hook `post-delete`.

---

## 🔗 Các lệnh Remote, Đồng bộ và Dữ liệu (Remote, Sync, & Data)

### `govard remote`

Quản lý các môi trường remote được định danh để sử dụng cho đồng bộ, deploy, shell và truy cập cơ sở dữ liệu.

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/app
govard remote copy-id staging
govard remote test staging
govard remote exec staging -- ls -la
govard remote audit tail --status failure --lines 50
```

Đối với các đường dẫn remote tương đối với thư mục home, hãy đóng dấu nháy đơn cho giá trị đường dẫn:

```bash
govard remote add staging --host staging.example.com --user deploy --path '~/public_html'
```

Các tính năng chính:
- Capabilities (Quyền hạn): `files`, `media`, `db`, `deploy`.
- Các phương thức đăng nhập: `keychain`, `ssh-agent`, `keyfile`.
- Tự động bảo vệ chống ghi đè cho môi trường production.
- Ghi nhật ký lịch sử thao tác: `~/.govard/remote.log`.

→ Hướng dẫn đầy đủ: [Remote & Đồng bộ](/vi/workflows/remotes-and-sync)

### `govard sync`

Đồng bộ các file, media, hoặc database giữa môi trường local và các remote server.

```bash
govard sync --source staging --destination local --full --plan
govard sync --from staging --to local --media
govard sync -s prod --file --path app/etc/config.php
govard sync --db --no-noise --no-pii
```

Tự động chọn remote `staging` nếu không truyền cờ `--source`, và fallback về `dev`.
Khi cờ `--media` được gọi mà không truyền mode cụ thể, Govard sẽ mặc định chạy ở chế độ `optimized`.

**Các cờ chính:**

| Cờ | Tác dụng |
| :--- | :--- |
| `-s, --source` / `--from` | Môi trường nguồn |
| `-d, --destination` / `--to` | Môi trường đích |
| `--file`, `--media`, `--db`, `--full` | Phạm vi đồng bộ dữ liệu |
| `--plan` | Chỉ hiển thị kế hoạch thực thi rồi thoát |
| `-I, --include` | Pattern bao gồm của rsync (có thể khai báo nhiều lần) |
| `-X, --exclude` | Pattern loại trừ của rsync (có thể khai báo nhiều lần) |
| `-m, --media [mode]` | Phạm vi đồng bộ media (`none`, `minimal`, `optimized`, `catalog` (Magento), `all`); cờ `--media` đơn lẻ mặc định là `optimized` |
| `-N, --no-noise` | Loại bỏ các dữ liệu rác khi đồng bộ |
| `-P, --no-pii` | Loại bỏ thông tin cá nhân nhạy cảm khi đồng bộ |

### `govard db`

Các tiện ích quản lý và truy vấn database local và remote.

```bash
govard db connect
govard db dump
govard db dump -e staging --local
govard db query "SELECT COUNT(*) FROM sales_order"
govard db info
govard db top
govard db import --file backup.sql --drop
govard db import --stream-db -e staging --drop
govard db clone-volume warden_magento2_dbdata
```

### `govard deploy`

Chạy các deploy lifecycle hook được cấu hình cho dự án hiện tại.

```bash
govard deploy
```

### `govard snapshot`

Quản lý các bản snapshot dữ liệu DB và media local/remote nhanh chóng.

```bash
govard snapshot create
govard snapshot create -e staging
govard snapshot list
govard snapshot list -e staging
govard snapshot restore latest
govard snapshot delete before-deploy
govard snapshot export latest ./backup.tar.gz
govard snapshot pull latest -e staging
govard snapshot push before-deploy -e prod
```

Các lệnh con: `create`, `list`, `restore`, `delete`, `export`, `pull`, `push`. `export` ghi ra file `tar.gz` local; `delete` xóa snapshot theo tên. `pull`/`push` chuyển snapshot giữa local và remote có tên (`-e`).

### `govard open`

Mở nhanh các đường dẫn dịch vụ/ứng dụng trên trình duyệt.

```bash
govard open app
govard open admin
govard open mail
govard open db
govard open db --pma
govard open db --client
govard open db -e staging
```

### `govard tunnel`

Quản lý các đường link public tunnel (yêu cầu cài đặt `cloudflared`). Govard đăng ký domain tunnel trong Caddy như alias, giữ nguyên `Host` header, và tự động rewrite base URL của framework (Magento, Laravel, …) — khôi phục khi `tunnel stop` hoặc `Ctrl+C`.

```bash
govard tunnel start
govard tunnel start https://my-tunnel.trycloudflare.com
govard tunnel start --provider cloudflare --no-tls-verify --plan
govard tunnel status
govard tunnel stop
```

| Cờ | Tác dụng |
| :--- | :--- |
| `[url]` | URL tunnel tùy chọn (nếu không truyền, tự dò từ output `cloudflared`) |
| `--provider <name>` | Provider tunnel (`cloudflare` là provider duy nhất hiện tại) |
| `--no-tls-verify` | Bỏ qua xác thực TLS cho endpoint tunnel |
| `--plan` | In kế hoạch khởi động rồi thoát, không chạy |

::: important QUAN TRỌNG
Binary `cloudflared` phải được bạn tự cài đặt riêng trên hệ thống.
Cài đặt thông qua [kho lưu trữ chính thức của Cloudflare](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/install-run/install-threads/) hoặc tải từ [releases trên GitHub](https://github.com/cloudflare/cloudflared/releases).
:::

→ Hướng dẫn đầy đủ: [Tunnel](/vi/workflows/tunnel)

---

## 🔧 Các lệnh gọi công cụ framework (Tool Commands)

Khởi chạy các CLI của framework bên trong container ứng dụng:

```bash
govard tool magento [command]    # Magento 2
govard tool magerun [command]    # Magento 1 / Magento 2 (Viết tắt: mr)
govard tool artisan [command]    # Laravel
govard tool drush [command]      # Drupal
govard tool symfony [command]    # Symfony
govard tool shopware [command]   # Shopware
govard tool cake [command]       # CakePHP
govard tool wp [command]         # WordPress
govard tool prestashop [command] # PrestaShop
govard tool dagster [command]    # Dagster
govard tool manage [command]     # Django (python manage.py)

# Các công cụ quản lý package & build dùng chung
govard tool composer [command]
govard tool php [command]        # Chạy trực tiếp CLI PHP (vd: tích hợp editor/IDE)
govard tool npm [command]
govard tool yarn [command]
govard tool npx [command]
govard tool pnpm [command]
govard tool grunt [command]
```

Các lệnh package Node (`npm`, `npx`, `yarn`, `pnpm`, và `grunt`) chạy trong
container one-shot `node:<stack.node_version>-alpine`, mount project tại
`/var/www/html`. Nhờ vậy bản build dùng cùng Node version với frontend watcher
và BrowserSync. Với PHP framework, `govard shell` không có Node.js.

`govard tool php` yêu cầu thư mục hiện tại phải đúng là project root. Với các tích hợp editor/IDE (xem phần dưới), dùng `govard vscode` thay thế.

---

## 🧩 Các lệnh tích hợp Editor (Editor Integration Commands)

### `govard vscode setup`

Ghi (hoặc merge vào) các cấu hình VSCode cần thiết để chạy công cụ PHP bên trong container thay vì trên host:

```bash
# Chạy từ trong project (hoặc bất kỳ thư mục con nào của project)
govard vscode setup
#   -> .vscode/settings.json: intelephense.environment.phpVersion, phpstan.paths,
#                             phpunit.paths (nếu có vendor/bin/phpunit),
#                             và (nếu có vendor/bin/phpcs) phpcs.standard + phpcs.autoConfigSearch=false
#   -> .vscode/launch.json:   cấu hình "Listen for Xdebug (Govard)" (port 9003)

# Chỉ chạy 1 lần, áp dụng cho mọi project Govard
govard vscode setup --global
#   -> tạo wrapper script ~/.govard/bin/govard-php, govard-php-cs-fixer, và govard-phpcs
#   -> user settings.json: php.validate.executablePath, phpstan.binCommand,
#                          php-cs-fixer.executablePath, phpcs.executablePath, phpunit.command
```

Settings phản ánh đúng profile đang dùng gần nhất của project (vd. profile upgrade ghim version PHP mới hơn) nếu có đăng ký, thay vì luôn đọc `.govard.yml` gốc — nên `intelephense.environment.phpVersion` khớp với thực tế đang chạy.

Coding standard cho PHPCS được tự nhận diện từ `composer.json` (`magento/magento-coding-standard` -> `Magento2`, `wp-coding-standards/wpcs` -> `WordPress`, `drupal/coder` -> `Drupal`), fallback về `PSR12` nếu không khớp gói nào. `phpcs.autoConfigSearch` bị tắt vì nếu không, extension sẽ tự dò file `phpcs.xml`/`.dist` và truyền path tuyệt đối *trên host* làm `--standard` — container không đọc được path đó.

Nếu có `vendor/bin/phpstan` nhưng project **chưa có** config `phpstan.neon`/`.dist`/`dist.neon` riêng, `setup` sẽ set `phpstan.options` với mặc định `--level=0` (`--autoload-file=vendor/autoload.php` cộng `app/code`+`app/design` cho Magento 2 hoặc `app`+`src` cho framework khác — đúng convention `govard test phpstan` đã dùng khi fallback) để PHPStan có gì đó mà phân tích. Cái này cố tình nằm trong `.vscode/settings.json`, không phải tạo file `phpstan.neon` ở root project — file đó thường bị git track và không phải của mình để tạo ra. Ngay khi project có config riêng, chạy lại `setup` sẽ tự xoá `phpstan.options` để không bao giờ đè lên rule thật của project — config của project luôn được ưu tiên.

`phpunit.command` (recca0120.vscode-phpunit) không cần wrapper script — đây là template mà extension tự tokenize, nên được set thẳng thành `govard vscode phpunit ${phpunitargs}`. Bạn có panel Testing (chạy/rerun từng test) mà không cần cài PHPUnit trên host. Debug từng test qua extension này chưa được wire — cần forward biến môi trường Xdebug vào lệnh `docker exec`.

Mỗi nhóm cấu hình cần đúng 1 extension VSCode tương ứng (Intelephense, PHPStan, PHP CS Fixer, PHPCS, PHPUnit, PHP Debug). Nếu chưa cài, `setup` sẽ cảnh báo và hỏi có muốn cài ngay qua `code --install-extension` không — đồng ý thì setting tương ứng vẫn được wire luôn trong lần chạy đó. Truyền `--yes` để tự cài hết những gì thiếu mà không hỏi (hữu ích khi chạy script); nếu không có TTY và không có `--yes`, các extension còn thiếu sẽ tự bị bỏ qua, không hỏi.

Các key hiện có và các configuration khác trong `launch.json` được giữ nguyên — chỉ những key do Govard quản lý mới bị thêm/ghi đè. Lưu ý: settings.json được parse như JSON thuần, nên comment (nếu có) sẽ bị mất khi ghi lại.

### `govard vscode <tool>`

Các lệnh chạy tool thực tế mà cấu hình do `setup` ghi ra sẽ trỏ vào:

```bash
govard vscode php [args]
govard vscode composer [args]
govard vscode phpstan [args]       # vendor/bin/phpstan
govard vscode php-cs-fixer [args]  # vendor/bin/php-cs-fixer
govard vscode phpcs [args]         # vendor/bin/phpcs
govard vscode phpunit [args]       # vendor/bin/phpunit, kèm memory_limit=-1
```

Khác với `govard tool`, các lệnh này tự tìm project bằng cách đi ngược thư mục từ vị trí hiện tại lên để tìm `.govard.yml` gần nhất — vì các editor thường gọi tool với thư mục làm việc không phải workspace root (vd: thư mục chứa file đang mở), nên không thể chắc chắn cwd khớp chính xác.

---

## ⚙️ Các lệnh cấu hình (Configuration Commands)

```bash
govard config get stack.php_version
govard config set stack.php_version 8.4
govard config set table_prefix demo_
govard config profile              # Hiển thị cấu hình profile đề xuất cho framework
govard config profile --json      # Output thông tin profile dạng JSON
govard config profile apply       # Áp dụng profile đề xuất vào .govard.yml
govard config auto                # Magento 2: inject các thiết lập kết nối vào env.php
```

### `govard config profile`

Hiển thị profile môi trường được khuyến nghị cho framework đã phát hiện.

```bash
govard config profile
govard config profile --json
```

Output bao gồm framework được phát hiện, phiên bản PHP đề xuất, cấu hình database, cache, search và các dịch vụ stack đi kèm khác.

### `govard config profile switch`

Chuyển đổi sang một profile môi trường cấu hình khác. Tính năng này cho phép bạn chạy cùng một dự án với các cấu hình runtime khác nhau (ví dụ: chạy PHP 8.2 để test production, chạy PHP 8.3 để code phát triển).

```bash
govard config profile switch upgrade
govard config profile switch staging
govard config profile switch          # Lựa chọn dạng tương tác trực quan
```

Các file profile được lưu trữ dưới dạng `.govard.<name>.yml` trong thư mục gốc dự án. Profile đang được chọn sẽ được ghi nhớ trên từng dự án tại đường dẫn `~/.govard/projects.json`.

Sau khi chuyển đổi, chạy `govard env up` để áp dụng môi trường mới. Bạn sẽ được nhắc xác nhận khi profile thay đổi yêu cầu khởi động lại container.

### `govard config profile clear`

Reset môi trường về lại profile mặc định (không sử dụng profile phụ).

```bash
govard config profile clear
```

### `govard extensions`

Khởi tạo các khung template mở rộng tại thư mục `.govard/*`.

```bash
govard extensions init
govard extensions init --force
```

### `govard blueprint cache`

Quản lý bộ nhớ cache của các registry blueprint tải từ xa.

```bash
govard blueprint cache list
govard blueprint cache clear
```

---

## 🩺 Diagnostics (Chẩn đoán lỗi)

### `govard doctor`

Khởi chạy hệ thống chẩn đoán lỗi môi trường kèm giải pháp sửa đổi cụ thể.

```bash
govard doctor
govard doctor --fix
govard doctor --json
govard doctor --pack
govard doctor trust
```

Các thành phần được kiểm tra bao gồm: Docker, Compose, các port kết nối, dung lượng ổ đĩa, tình trạng thư mục Govard home, sức khỏe thư mục compose, SSH agent và kết nối mạng ra ngoài.

- **`--fix`** — Tự động phát hiện và sửa các lỗi phổ biến được tìm thấy.
- **`trust`** — Cài đặt Root CA vào keychain hệ thống + browser NSS store.

---

## 🔁 Các lệnh tiện ích (Utility Commands)

### `govard lock`

Tạo hoặc kiểm định file `govard.lock` phục vụ cho việc phát hiện sai lệch cấu hình môi trường giữa các máy.

```bash
govard lock generate
govard lock check
govard lock diff
govard lock generate --file .govard/govard.lock
```

### `govard self-update`

Tải về phiên bản Govard mới nhất, kiểm định mã checksum và thay thế các binary đã cài đặt một cách an toàn.

```bash
govard self-update                    # cập nhật theo channel hiện tại
govard self-update --channel beta     # chuyển sang nhận bản beta (được lưu lại)
govard self-update --channel stable   # quay lại bản stable (được lưu lại)
govard self-update --version v1.60.0-beta.1  # cài đúng 1 phiên bản cụ thể
```

Channel cập nhật được lưu lại qua các lần chạy (CLI và Govard Desktop dùng
chung cấu hình này), nên `govard self-update` không kèm flag sẽ tiếp tục
theo channel bạn đã chọn lần gần nhất.

### `govard upgrade`

Pipeline hỗ trợ nâng cấp framework native.

```bash
govard upgrade --version 2.4.8-p4     # Magento 2
govard upgrade --version 11            # Laravel
```

**Các cờ:**

| Cờ | Tác dụng |
| :--- | :--- |
| `--version` | Phiên bản đích nâng cấp (bắt buộc) |
| `--dry-run` | Xem trước các bước thực thi không chạy thực tế |
| `--no-db-upgrade` | Bỏ qua chạy các câu lệnh migration database |
| `--no-env-update` | Bỏ qua cập nhật profile và restart container |
| `-y, --yes` | Tự động đồng ý qua các câu hỏi xác nhận |

### `govard version`

```bash
govard version
```

### `govard redis`

Tiện ích thao tác nhanh với Redis/Valkey.

```bash
govard redis cli
govard redis flush
govard redis info
```

### `govard varnish`

Tiện ích thao tác nhanh với Varnish.

```bash
govard varnish purge
govard varnish status
```

### `govard rabbitmq`

Tiện ích thao tác nhanh với RabbitMQ.

```bash
govard rabbitmq status
govard rabbitmq queues
govard rabbitmq cli list_exchanges
```

### `govard valkey` / `govard elasticsearch` / `govard opensearch`

Shortcut trực tiếp tới service compose cùng tên. `govard env` proxy thông minh mọi lệnh compose khác, nhưng ba shortcut top-level này được đăng ký tường minh:

```bash
govard valkey cli
govard elasticsearch info          # hoặc: govard elasticsearch _cluster/health
govard opensearch info
# Mọi tham số sau tên service đều được forward:
govard elasticsearch curl -s http://elasticsearch:9200/_cat/indices
```

Host truy cập search cũng được route tự động qua `http://<domain>:9200` (xem [Cấu hình](/vi/reference/configuration#connecting-to-elasticsearchopensearch-from-the-host)) — shortcut trên exec trong container, còn route `:9200` dành cho `curl`/browser trên host.

---

## 🌐 Các cờ toàn cục (Global Flags)

Tất cả các lệnh của Govard đều hỗ trợ:

- `-h, --help` — Hiển thị trợ giúp của lệnh

---

[← Bắt đầu](/vi/getting-started/getting-started) | [Cấu hình →](/vi/reference/configuration)
