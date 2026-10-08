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

`--path <dir>` (dùng cho cả `audit run` và `audit diff`) phân giải target từ
`<dir>` thay vì thư mục làm việc hiện tại, để một lệnh có thể audit một thư mục
mà nó không chạy trong đó — ví dụ một dòng checklist audit một module nằm dưới
project root. Đường dẫn tương đối được tính theo thư mục làm việc hiện tại, và
`<dir>` phải tồn tại: phân giải sẽ lỗi nếu không, còn việc phân loại mode được
áp dụng trên `<dir>` đúng như khi áp dụng trên thư mục làm việc.

```bash
# buộc target module trong khi đang chạy từ project root
govard audit run --mode module_in_project --path app/code/Acme/Catalog
```

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
root, và phase đó làm run thất bại. Tiền tố id do framework quyết định
(`M2-LINT-*` cho Magento, `LARAVEL-LINT-*`, `SYMFONY-LINT-*`, `WP-LINT-*`). Phép quét này chỉ mất mili giây dù media lớn
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

#### Phạm vi (`--scope`, `--base`)

`--scope project` (mặc định) audit toàn bộ target. `--scope diff --base <ref>` ghi
base được yêu cầu vào manifest của session và chỉ lint những file đã đổi
(`git diff --name-only --diff-filter=ACMRT <base>...HEAD` cộng với staged/unstaged
so với `HEAD`, lọc còn `php/phtml` trong target, loại trừ
`vendor/generated/var/pub/media` qua `diff-files.txt` được mount thành
`GOVARD_LINT_DIFF_FILE`); diff rỗng thì short-circuit thành `passed` với cache
`diff-empty` mà không khởi động container. Dùng `--base auto` để tự dò base qua
`git merge-base HEAD origin/HEAD` (cùng fallback `origin/master`/`origin/main`, rồi
`gh pr view --json baseRefName`). `git merge-base` trả về commit SHA (dùng thẳng làm
base); `gh pr view` chuẩn hoá thành `origin/<branch>` khi thiếu tiền tố.
Ví dụ cho skill review:

```bash
govard audit run --checks lint --mode project --scope diff --base auto --format json
```

#### Chạy đồng thời

Nhiều lệnh `govard audit run` đồng thời trên cùng một dự án sẽ xếp hàng qua
`~/.govard/audit/<projectId>/lock` (qua `GovardHomeDir`, tôn trọng
`GOVARD_HOME_DIR`). Lần chạy thứ hai chờ tối đa 30s để lần trước nhả lock
(`audit run waiting for prior run`) rồi tiếp tục; các run được xếp hàng chứ không
bị huỷ (sửa hành vi `cancelled` trước đó). Nếu lock vẫn bị giữ sau 30s, run fail
kèm gợi ý xoá lock cũ hoặc chạy `govard audit cleanup`.

#### Guard Xdebug

Khi `stack.features.xdebug: true`, lint audit thoát với thông báo
`Xdebug enabled, ~10-20% tax; disable with govard debug off or pass --allow-xdebug`
trừ khi truyền `--allow-xdebug`. Guard được thực thi trong command và trong backend
lint của Govard.

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
một Govard project và không bao giờ gọi external lint provider. Chúng cần
container runtime — trên host không có Docker, chúng thoát với mã `3` kèm
`CAPABILITY_MISSING` trước khi chạm vào Docker, giống mọi lệnh khác có yêu cầu
trong manifest là `docker`. Phân tích không cần container vẫn dùng được:
`govard audit run --checks integrity`.

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
- `--fresh` + `--framework` + `--framework-version` — cài đặt mới hoàn toàn qua scaffolder của framework. Không có `--framework-version` thì fresh install dùng `framework_version` đã có trong `.govard.yml` (ví dụ từ `govard init --framework-version 2.4.6`), và plan cùng log nói rõ dùng version nào và lấy từ đâu; không có cả hai thì cài bản mới nhất. Flag tường minh luôn thắng.
- `--clone` + `--environment` — rsync toàn bộ mã nguồn từ một remote server.

**Lựa chọn nguồn (Source selection):**
- `-e, --environment` — tên của remote nguồn; chấp nhận các tên chuẩn (`staging`, `production`, `dev`) cũng như các định danh tùy chỉnh (`qa`, `preprod`, `demo`, `client-uat`).
- `--remote` — tên viết tắt của `--environment`.
- `--db-dump` — import database trực tiếp từ một đường dẫn file SQL local.

Tên `sandbox` cũng được chấp nhận, và được resolve từ container đang chạy giống
hệt `govard deploy --remote sandbox`. Một lần chạy không phải `--plan` mà chưa có
container sẽ dừng với thông báo riêng của sandbox (`no sandbox exists for this
project; run 'govard sandbox up' to create it`) thay vì báo tên này chưa được cấu
hình, và lệnh sẽ không bao giờ đề nghị chạy `remote add sandbox` — chỉ
`govard sandbox up` mới tạo được container. Việc dừng này giả định không có prompt
tương tác để rơi về: trong terminal mà không có `--yes`, cùng thông báo đó được in
ra và trình chọn tên remote mở ra, và lần thử lại mà nó chạy sẽ lại thất bại y
hệt. Như với mọi tên không resolve được,
`--plan` vẫn không coi đây là lỗi và không in ra plan nào.

**Các bộ lọc hiệu năng & bảo mật dữ liệu:**

| Cờ (Flag) | Tác dụng |
| :--- | :--- |
| `-N, --no-noise` | Loại bỏ các dữ liệu rác/tạm thời (logs, sessions, cache tags, lịch sử cron) |
| `-P, --no-pii` | Loại bỏ các dữ liệu cá nhân nhạy cảm (thông tin khách hàng, đơn hàng, tài khoản admin, password) |
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

`env up` chờ readiness của runtime cho từng service — tối đa ~90 giây mỗi
service. Với thông tin database sai, service không bao giờ healthy nên lỗi tới
muộn: tệ nhất là một lần chờ đủ cho mỗi service. Khi `up` treo, hãy kiểm tra
credential trước, đừng nghi ngờ orchestration.

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

### `govard verify`

Chạy bộ kiểm tra QA thực thi 5 pha (thay cho tick thủ công). Registry là nguồn duy nhất — 74 mục tĩnh P1 15 · P2 15 · P3 15 · P4 21 · P5 8, cộng với các mục mà từng framework tự khai báo. 10 mục tĩnh mang `When isMagento2` và được báo cáo thành dòng `skipped` (không bị loại bỏ) với Laravel/Symfony/WordPress; các framework này lấy lại dev loop nhờ mục do framework khai báo (xem bên dưới).

```bash
govard verify --plan --json                 # dry-run tất cả pha, JSON máy
govard verify --phase 1 --json              # một pha (1..5, 0=all)
govard verify --phase 5 --allow-destructive --json # destructive sau snapshot
govard verify --phase 3 --project /path/to/project --json # chọn dự án khác (chạy trơn bị từ chối nếu thiếu --allow-destructive)
```

Flags: `--phase 0..5`, `--json`, `--plan`, `--allow-destructive` (`--yes`), `--allow-remote-write`, `--allow-xdebug`, `--lint-jobs 4`, `--timeout auto|0|<dur>`, `--checks`, `--base`, `--remote`, `--project`. `--base`, `--remote`, `--allow-xdebug`, `--allow-destructive` và `--allow-remote-write` thay đổi thứ được chạy. `--checks lint,profiler` thu hẹp run xuống các mục dùng một trong các check đó: mục không khai báo check nào không thuộc riêng check nào nên luôn chạy, còn mục bị lựa chọn loại ra vẫn nằm trong báo cáo dưới dạng dòng `skipped` có nêu tên lựa chọn. `P3-15` chạy `audit run --checks integrity` và khai báo check đó, nên lựa chọn `lint` hoặc `profiler` loại nó ra. Chỉ những tên mà `audit run --checks` chấp nhận mới hợp lệ — `lint`, `profiler`, `integrity` — nên `--checks lints` là lỗi usage (exit 2) thay vì một flag bị bỏ qua, chọn không mục nào mà vẫn báo `passed`. `--lint-jobs N` và `--timeout <value>` được truyền vào argv của các mục chạy check lint — `P3-10`, `P3-11`, `P3-13`, `P3-14` và `P5-04` — và mỗi mục trong số đó cũng ghi lại argv đã giải quyết vào evidence excerpt. `P3-12` chạy check profiler nên không nhận flag nào. Giá trị `--checks` rỗng được coi là chưa đặt chứ không phải một lựa chọn: `verify --checks ""` chạy toàn bộ, còn `audit run --checks ""` nghĩa là `lint`. Ở giá trị mặc định của flag (`4`, `auto`) verify coi như chưa được đặt: `--lint-jobs` không được truyền đi, để `audit run` giữ số worker `engine.AuditRunJobs()` của chính nó thay vì số 4 của verify đè lên cơ chế tự điều chỉnh của máy — vì vậy `--lint-jobs 4` là yêu cầu duy nhất không thể diễn đạt, và giá trị nhỏ hơn 1 cũng được coi như chưa đặt thay vì truyền xuống cho lệnh con từ chối — còn `--timeout` rơi về giá trị riêng của mục: `auto` cho các mục lint ở pha 3, `0` (không deadline) cho lần re-lint `--no-lint-result-cache` ở pha 5. Số worker vượt quá khả năng của framework dự án là việc của lệnh con: `audit run` từ chối nó nên mục đó đỏ kèm thông báo của con, chứ không bị bỏ qua trong im lặng. `--base <ref>` là base cho phạm vi diff của `P3-11` và luôn được ưu tiên; nếu không có, mục dùng ref đầu tiên tồn tại trong thư mục gốc dự án trong `origin/master`, `origin/main`, `master`, `main`, và skip với lý do nêu `--base` khi không có ref nào, vì `audit run --scope diff` âm thầm mở rộng sang toàn bộ dự án khi thiếu base ref.

Mục do framework khai báo: mỗi framework khai báo mục checklist của riêng nó ngay trong package của nó — `VerifyToolItems` trên định nghĩa framework nêu id, pha, tiêu đề và đúng một lệnh `govard tool <binary> <args>` — rồi `RegistryFor` ghép chúng với registry tĩnh, nên `internal/verify` không hề nêu tên framework nào. Magento 2 khai báo `P5-MAG-01` (`setup:db:status` sau restore, đúng phần phát hiện mà `P5-07` còn thiếu); Laravel `P3-LAR-01..03` + `P5-LAR-01`; Symfony `P3-SYM-01..03` + `P5-SYM-01`; WordPress `P3-WP-01..03` + `P5-WP-01`. Chỉ những lệnh mà bộ khung của framework đảm bảo mới được khai báo, nên một mục không bao giờ đỏ vĩnh viễn vì thiếu bundle hay plugin tuỳ chọn.

Mục remote: mọi mục có nêu remote — `P2-04`..`P2-08`, `P2-15`, `P4-01`, `P4-03`..`P4-07`, `P4-13`..`P4-15`, `P4-17` và `P4-18` — đều lấy remote từ `--remote` và chỉ từ đó. Một lần chạy không có `--remote` sẽ đánh dấu skip các dòng đó với lý do `no --remote named: this item contacts a remote` thay vì đoán, nên checklist không bao giờ mở phiên tới bất cứ thứ gì dự án tình cờ gọi là `staging`/`stage`/`stg`; `P4-16` (`remote list`) không nêu remote nên vẫn chạy. `P4-13`..`P4-16` phủ nửa chỉ-đọc của bề mặt remote — `deploy plan`, `deploy status`, `deploy releases` và `remote list` — an toàn khi chạy vào một remote production. Checklist là một preflight, nên thứ ghi vào thì nằm ngoài nó một cách có chủ đích. `deploy check` được chạy tay thay vì làm một dòng: nó không để lại gì trên target, vì probe quyền ghi của nó không tạo ra path nào mà đi ngược lên parent tồn tại gần nhất ở đó rồi kiểm tra parent đó, còn probe `mv -T` mà nó chạy trên một target dạng symlink thì tạo một thư mục tạm `.dep` ở đó (và, trên một host mới, các tầng còn thiếu của deploy path phía trên nó) rồi xoá chúng đi. `deploy unlock`/`deploy rollback` thay đổi lock và release state trên target, `db`/`snapshot`/`open -e` thay đổi trạng thái trên remote đích (lệnh duy nhất vẫn còn *đề nghị* ghi vào `authorized_keys` để tới được đó là `govard remote test`, và lời đề nghị đó mặc định là **Không** trừ khi bạn nói có; ngoài ra việc thiết lập key là yêu cầu tường minh qua `govard remote copy-id <remote>`), và `tunnel stop` chỉ gửi tín hiệu tới đúng tunnel mà govard đã ghi nhận cho project, đồng thời từ chối mọi pid mà nó không xác nhận được là do chính mình khởi động.

Mục probe: `P2-13` và `P4-11` gọi tới chính site của project, nên một project chưa cấu hình `domain` thì không probe được — chúng skip với lý do `no configured domain` thay vì đoán `localhost` rồi báo cáo về một môi trường khác. `P2-13` ưu tiên `https://<domain>/` và chỉ lùi về HTTP thuần khi chính tầng TLS không dùng được (CA cục bộ không được tin, hoặc một cổng trả lời bằng clear text), và evidence nêu scheme nó đã dùng; `P4-11` cần một payload health có `status` thật, nên câu trả lời không có body là đỏ chứ không phải xanh. `P3-12` theo cùng quy tắc cho URL của profiler: không có `domain` thì nó skip với lý do `no domain configured` thay vì audit một host đoán mò.

Verdict của từng dòng: một dòng chỉ báo cáo điều nó đã chạy. `P1-06` (`lock check`, kèm `lock diff` làm evidence khi fail) skip với lý do nêu `govard lock generate` thay vì sinh ra một file được track từ một pha verify, còn lock đã tồn tại mà lệch với project vẫn đỏ; `P3-09` (`frontend start`) skip trên Magento 2 trừ khi `stack.features.frontend_sync` bật, và lý do skip nêu đúng công tắc đó. Mỗi mục còn mang một ghi chú `Requires` (nó giả định điều gì, ví dụ `P2-01 up`) cho người đọc registry: runner không bao giờ bắt buộc nó, và không lý do skip nào trích nó.

Gates: Pha 5 yêu cầu snapshot **của chính project này** — một lần chạy pha 4 thật (không phải `--plan`) cho cùng project, có `P4-08` exit `0` và snapshot được ghi lại vẫn còn dùng được trên đĩa — VÀ `--allow-destructive`. Thiếu → `need snapshot create (P4-08) first`; thiếu flag → `need --allow-destructive for phase 5`. Lần chạy `--plan` không thoả gate nào, không chạy gì và không đổi trạng thái project nào (nó vẫn ghi một artifact `mode: "plan"`, thứ không bao giờ thoả gate). Mọi gate được kiểm tra trước khi bất kỳ mục nào chạy, ở cả chế độ người đọc lẫn `--json`: `verify` không có `--phase` và không có `--allow-destructive` thoát `1` với lỗi gate và không chạy gì (không môi trường nào bị dừng, không snapshot nào được tạo), `--phase 0` bị gate như pha 5, và opt-in `--allow-destructive` được kiểm tra trước snapshot gate. Ở lần chạy tất cả các pha, snapshot gate được kiểm tra khi tới pha 5, vì chính pha 4 trong lần chạy đó tạo snapshot. Một lần từ chối xảy ra trước khi có gì chạy in `{"error": "<lý do>"}` ra stdout khi có `--json`. Khi pha 5 từ chối khởi động sau khi pha 1 đến 4 đã chạy, lần chạy tất cả các pha với `--json` vẫn in một document duy nhất: kết quả gộp của pha 1-4 kèm lý do trong chuỗi `error` (trường `error` không bao giờ xuất hiện trong run artifact), và exit code là `1`.

Run artifact được ghi dù có `--json` hay không, vì `--json` chỉ định hình stdout: `verify --phase 4` rồi `verify --phase 5 --allow-destructive` chạy được mà không cần `--json`. Thư mục runs không ghi được không làm lần chạy mất gì: verdict được giữ và một cảnh báo ra stderr. Chỗ duy nhất có ý nghĩa là lần chạy tất cả các pha mà bản ghi pha 4 không ghi được, vì khi đó pha 5 không có bản ghi snapshot nào để đọc; nó bị từ chối với `phase 4 result could not be recorded: <nguyên nhân>; phase 5 needs it` (`verify.ErrRunNotRecorded`) thay vì thông báo `need snapshot create` trần. `P4-08` ghi lại tên snapshot nó tạo và `P5-05` restore đúng tên đó, nên restore không thể lấy nhầm snapshot khác được tạo ở giữa. Một dòng bị đánh dấu `skipped` không bao giờ thoả gate: nó ghi nhận rằng `P4-08` tồn tại, không phải rằng nó đã chạy.

Guard: mỗi mục tĩnh mang một trong bốn nhãn (`Item.Guard`). Runner hành động theo `DESTRUCTIVE-LOCAL` và `REMOTE-WRITE`; `REMOTE-PROBE` là nhãn cho người đọc, không phải một lớp bảo vệ, và chế độ plan là gate duy nhất của nó.

| Guard | Argv tuyên bố điều gì | Runner làm gì | Số mục |
| :--- | :--- | :--- | :--- |
| `""` | việc cục bộ, không remote, không có gì không thể hoàn tác | chạy | **53** |
| `REMOTE-PROBE` | có nêu remote, argv của nó không ghi gì lên đó | chạy khi `--remote` nêu tên một remote; nếu lần chạy không có cờ đó, mục giữ nguyên dòng ở trạng thái skip và không dò một remote đoán mò (`--plan` vẫn thay bằng stub, và `P4-16` không nêu remote nên luôn chạy). Không có gate ở tầng guard — `--plan` vốn đã thay mọi mục bằng stub nên chặn chúng ở đó sẽ xoá coverage chứ không bảo vệ được gì. Ở lần chạy thật, mục này tới remote vô điều kiện: cái tên nói "probe" để không ai đọc nó như một lời đảm bảo | **15** |
| `DESTRUCTIVE-LOCAL` | phá huỷ cục bộ không thể hoàn tác | chỉ pha 5, cùng với snapshot gate và `--allow-destructive` | **3** |
| `REMOTE-WRITE` | ghi qua remote | **skip** trừ khi truyền `--allow-remote-write` | **3**: `P2-05`, `P2-08`, `P2-15` |

`P2-15` (`deploy --remote <remote> --yes --force`) là bài diễn tập đầu-cuối trên một remote sandbox: `--yes` bỏ prompt của nó, nên với `--allow-remote-write` nó chạy không cần người trả lời, còn `--force` khiến lần chạy lại deploy thật thay vì thoát `0` với `already runs <rev>; nothing to do`. `P4-17` (`deploy check`) và `P4-18` (`remote exec <remote> -- hostname`) là các probe không để lại gì trên target; `P4-19` (`remote audit stats`) đọc audit log cục bộ, còn `P4-20`/`P4-21` chỉ chạy phần help của `deploy rollback` và `deploy unlock`, những lệnh thay đổi target. `P1-08`..`P1-15` là các dòng chỉ-đọc về host và project (`capabilities --json`, mà output phải là JSON hợp lệ, `config profile`, `domain list`, `custom list`, `project orphans`, `gateway status`, `audit toolchain status`, `sandbox status`).

Các dòng không áp dụng được thì skip kèm lý do thay vì fail: `P4-10` và `P4-11` cần `stack.services.cache` / `stack.services.search` nêu tên một service, các dòng audit lint, profiler và integrity (`P3-10`..`P3-15`, `P5-04`) theo đúng những gì định nghĩa framework khai báo, `P2-12` (`tool composer validate --no-check-publish`, nên một project skeleton không có tên package chỉ bị đánh giá về tính nhất quán giữa manifest và lock) cần `composer.json` và `P3-07` cần `vendor/bin/phpstan` trong thư mục gốc project, còn `P1-03` (`doctor trust`, cần sudo và không có terminal để hỏi) cần sudo không mật khẩu. `P2-08` (clone bootstrap) sao chép từ release đang live của remote, nên nó skip kèm lý do nêu rõ khi `deploy releases --remote <remote> --json` báo remote chưa có release nào (chạy `P2-15` trước).

Hai mục `REMOTE-WRITE` gốc chính là các lần chạy `bootstrap … --no-noise`. Lý do skip nêu lệnh chạy thủ công, với `<remote>` thay cho remote bạn phải nêu bằng `--remote`, và chúng chạy `bootstrap -y`: các tiến trình con của verify không có tty, nên thiếu `-y` lệnh sẽ dừng ở bước xác nhận và thoát với mã 1. Với `--allow-remote-write` chúng chạy không cần người trả lời và thực sự ghi qua remote.

Exit codes: `0` mọi mục đều pass hoặc bị skip; `1` có mục fail **hoặc** một gate của pha chặn lần chạy (thông báo nêu rõ gate nào). `2`/`3`/`4` giữ nguyên nghĩa toàn cục (usage / capability / config) — checklist đỏ là lỗi thực thi, không bao giờ là lỗi dùng lệnh. Script nên rẽ nhánh theo mã này và đọc chi tiết từng mục từ `--json`.

Outputs: `<govard home>/verify-runs/<project-id>/<ISO>-phaseN.json`, với `project-id` là hash của đường dẫn chuẩn hoá của project, gồm `{govard_version, project_sha, project_id, phase, mode, status, items:[{id, command, duration_ms, exit_code, evidence_excerpt, json_valid, artifacts, skipped, skip_reason}]}`. Khoá `retries` theo từng mục của các bản trước đã bị bỏ: nó luôn là `0` và không có gì đọc nó, nên consumer từng đọc nó giờ thấy nó vắng mặt. `evidence_excerpt` bị giới hạn 500 byte: mục pass giữ phần đầu, mục fail giữ cả đầu và đuôi quanh dấu `[... output truncated ...]` để lỗi thật không bị mất, và `command` của `P3-11` nêu `--base` đã giải quyết (`<no base ref found>` khi không có ref nào). `mode` là `run` hoặc `plan`; `status` là `passed` hoặc `failed`; một lần chạy mà evidence không do tiến trình nào tạo ra mang `fake: true` trên document và trên các mục đó (hook hermetic: bất cứ khi nào binary sẽ chạy mục đó là một binary test của Go, nó trả lời bằng excerpt `fake(test-binary):` và không cần biến môi trường nào, còn `GOVARD_VERIFY_FAKE=1` chỉ đổi tiền tố đó thành `fake:`; với mọi binary khác cả hai đều bị bỏ qua, nên một `GOVARD_VERIFY_FAKE=1` lạc loài không đổi gì ở lần chạy thật), và `status` vẫn là `passed` hoặc `failed`; `artifacts` nêu thứ mà mục đó tạo ra (snapshot của `P4-08`, chính là thứ `P5-05` restore). `skipped` đánh dấu một mục tồn tại nhưng không áp dụng cho lần chạy này — ví dụ mục bị gate theo framework — và `skip_reason` nói rõ lý do; bảng cho người đọc in `SKIP` cho mục đó, nó không được tính vào cả hai cột pass/fail, không bao giờ làm pha đỏ và không đổi exit code, nên một pha chỉ toàn mục skip vẫn `passed` và exit `0`. `~/.govard/checklist-runs/` cũ được migrate lần đầu; artifact tạo trước khi có project-scoping (nằm phẳng ở gốc `verify-runs/`) không mang danh tính project nên không bao giờ thoả gate. `phase 0/all --json` xuất một JSON duy nhất `phase: "all"` với `status` được tính lại và `items` gộp.

`GOVARD_VERIFY_BIN` ghim binary mà các mục checklist thực thi. Không đặt thì binary đang chạy sẽ thực thi chúng, và `PATH` chỉ là phương án cuối. Để kiểm chứng một bản build từ source, hãy chạy trực tiếp bản build đó và đặt biến này nếu nó không phải là bản mà shell sẽ tìm thấy — nếu không, `govard` trần trên `PATH` có thể là bản cài cũ và checklist sẽ báo cáo về bản đó, không phải bản build của bạn.

Tự động phát hiện framework qua `engine.DetectFramework` khi thiếu `.govard.yml` (nên project mới có `artisan`/`composer.json` được nhận diện `laravel` v.v.).

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
govard remote copy-id staging                  # lệnh duy nhất copy public key
govard remote test staging
govard remote exec staging -- ls -la
govard remote list                        # các remote đã cấu hình, cộng dòng sandbox ẩn
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

`remote exec` chạy bất kỳ lệnh shell nào bạn đưa vào trên remote, và nó nằm **ngoài** gate
bảo vệ ghi: không có gì kiểm tra lệnh đó, kể cả với remote được bảo vệ, nên người vận hành
tự chịu trách nhiệm về thứ mình chạy. Ví dụ trong help cố ý là lệnh chỉ đọc
(`govard remote exec staging -- "df -h /var/www"`). `remote exec`, `remote test` và
`remote copy-id` nhận một alias của remote đã cấu hình (`stg`, `STAGE`) và resolve nó
về tên đã cấu hình trước, nên key và cấu hình auth lưu cho remote đó mới là thứ được dùng
và các sự kiện audit mang tên đã cấu hình.

`remote add` kiểm tra trước khi ghi bất cứ thứ gì: tên không hợp lệ hoặc `--port` ngoài
phạm vi (âm hoặc lớn hơn 65535) thoát `4`, file cấu hình và auth store nguyên vẹn, còn
file cấu hình không ghi được thì thoát khác 0 thay vì in dòng thành công. Điều tương tự
áp dụng cho `config set`, `domain add|remove`, `profile apply` và `debug on|off`: cấu hình
kết quả không qua validate thì thoát `4` và để `.govard.yml` như cũ, ghi hỏng là một lỗi,
và `debug on|off` không khởi động môi trường sau một lần lưu hỏng.

`remote list` in bảng NAME/HOST/CAPABILITIES/AUTH/KEY gồm các remote đã cấu
hình cộng dòng synthetic `sandbox (implicit)`, trong đó cột HOST mang trạng thái
(`running`, `dormant — …`, `absent — …`) và cột CAPABILITIES mang đúng những gì
block `remotes.sandbox` cấu hình. Khi có block `remotes.sandbox` được cấu
hình, sandbox vẫn chỉ có đúng một dòng, và stderr cho biết block đó làm gì:
`capabilities`, `protected` và cấu hình `deploy` của nó được lớp lên sandbox
synthetic, còn host, port, user và auth vẫn lấy từ container.

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

`--source` và `--destination` đều nhận `sandbox`, và tên này được resolve từ
container đang chạy — cùng một remote mà `govard deploy --remote sandbox` dùng —
nên không cần block `remotes.sandbox`; và nếu đã có block, block chỉ định hình
buổi diễn tập (capabilities, protection, cấu hình `deploy`), không bao giờ trỏ
lại luồng truyền sang máy khác. Trên máy không có Docker mà được yêu cầu `-e sandbox`
thì lệnh thoát với mã `3` và `CAPABILITY_MISSING`; còn khi có Docker nhưng chưa có
container thì thoát với mã `1` kèm lời nhắc chạy `govard sandbox up`.

`sync` chỉ cần container runtime cho phạm vi database: `--db` và `--full` chuyển database
qua container database local, nên khi thiếu nó thì thoát `3` với `CAPABILITY_MISSING` trước
khi liên lạc với bất kỳ remote nào, còn `--plan` được miễn và sync file hay media không cần
Docker. `--plan` và bản tóm tắt xác nhận hiện mật khẩu database dưới dạng
`export MYSQL_PWD=***;` (hoặc `export PGPASSWORD=***;` với PostgreSQL) thay cho giá trị thật;
các dòng đó chỉ là văn bản hiển thị, còn lệnh được chạy vẫn mang giá trị thật.

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
| `-P, --no-pii` | Loại bỏ thông tin cá nhân nhạy cảm khi đồng bộ; bị từ chối khi không biết table prefix, nên không bao giờ tạo dump chưa lọc |
| `--resolve-symlinks` | Khi pull, sao chép nội dung của symlink trỏ ra ngoài path đồng bộ (mặc định: không đi theo các link đó) |

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

File dump chỉ thuộc về chủ sở hữu của nó. `db dump` và `db import --stream-db` tạo file
local với mode `0600`, và một file đã tồn tại mà rộng quyền hơn thì bị siết về `0600` trước
khi ghi bất cứ thứ gì vào. `db dump -e <remote>` không kèm `--local` chạy dưới `umask 077`
trên remote, nên dump là `0600` và mọi thư mục nó tạo (như `~/backup`) là `0700`.

### `govard deploy`

Triển khai một revision git lên môi trường remote.

```bash
govard deploy <remote>                   # triển khai HEAD ở máy local
govard deploy staging --revision <sha>   # triển khai đúng một commit (CI)
govard deploy build staging --output artifacts --revision <sha>   # build artifact (CI)
govard deploy staging --artifact-dir artifacts --revision <sha>   # triển khai artifact đó
govard deploy plan staging               # in kế hoạch, không kết nối
govard deploy check staging              # kiểm tra trước và báo chiến lược publish
govard deploy releases staging           # liệt kê các release trên server
govard deploy status                     # mỗi môi trường đang chạy revision nào
govard deploy rollback staging           # đưa release trước đó trở lại
govard deploy rollback staging --to 12   # ... hoặc một release chỉ định
govard deploy rollback staging --with-db --yes   # ... kèm cả dump database
govard deploy unlock staging --force     # giải phóng lock do lần deploy lỗi
```

Quản lý sandbox ở máy local — một container đóng vai đích triển khai — là lệnh
top-level `govard sandbox`, được mô tả bên dưới:

```bash
govard sandbox up --profile full --php 8.4   # database, cache, PHP 8.4
govard sandbox up --profile full --db mariadb:10.6   # choose the database series
govard sandbox status
govard sandbox down [--purge] [--volumes]
```

→ Những cấu hình đã làm sẵn (Luma, Hyvä, nhiều theme và store view, chế độ developer
và production, docroot symlink và docroot thật): [Case study triển khai](/vi/workflows/deploy-case-studies).

Đích là một remote trong `.govard.yml`. Branch, repository, deploy path và chiến
lược publish lấy từ block `deploy:` của dự án; mỗi remote có thể ghi đè bằng các
field topology trên remote (`branch`, `repository`, `deploy_path`, `publish`,
`local`) hoặc bằng `remotes.<name>.deploy.<key>` cho các key của block `deploy:`.
Flag có độ ưu tiên cao nhất.

`deploy_path` không có mặc định và cũng không được tự thêm: remote bỏ trống sẽ
dùng layout mà target đã có (`releases/`, `shared/`, `.dep/` hoặc symlink
`current`), chỉ nhận khi đúng một ứng viên khớp, và govard nói rõ đã dùng cái nào.
Không có layout nào, hoặc nhiều cái, là lỗi cấu hình (exit 4) kèm danh sách đã dò.
`deploy_path` đã cấu hình thì không bao giờ bị dò.

`deploy:verify` chạy check revision đang live, các shared file recipe yêu cầu, các
check do recipe khai báo — với Magento là `setup:db:status`, chạy trong **served
path** nên target in-place kiểm tra docroot chứ không phải release — và request
HTTP khi đã đặt `deploy.verify.url`. Với target in-place nó còn dry-run chính lệnh
copy của activation cho từng entry trong `sync_paths`, đó là thứ chứng minh docroot
giữ đúng những gì build tạo ra. Deploy có chạy `db:migrate` mà thiếu verify URL
sẽ in cảnh báo trước bước đầu tiên. `maintenance:enable`/`disable` bị bỏ qua với kích hoạt
symlink, trừ khi plan đó còn migrate hoặc import cấu hình.

Pipeline là một chuỗi task trung tính cố định. Dự án tuỳ biến bằng cách neo hook
vào một task id, vào alias của stage (`stage:build`) hoặc vào một hook khác:

```yaml
deploy:
  hooks:
    - { name: apache-reload, on: "publish:activate", position: after, order: 10, run: "touch ~/apache-reload" }
```

Mỗi framework tự đóng góp recipe của mình thay vì govard rẽ nhánh theo tên
framework: `magento2` điền các task build/publish bằng lệnh Magento, còn framework
chưa có recipe vẫn triển khai code qua pipeline trung tính. Task nào recipe bỏ
trống sẽ được báo là skipped, không phải lỗi.

Flag: `--remote`, `--branch`, `--revision`, `--tag` (loại trừ lẫn nhau),
`--build=auto|server|artifact`, `--artifact-dir`, `--publish=auto|symlink|in_place`,
`--keep`, `--verify/--no-verify`, `--db-backup/--no-db-backup`,
`--lock/--no-lock`, `--ignore-deployer-lock`, `--command-timeout`, `--resume`,
`--from <task>`, `--force`, `--yes`, `--json`, `--verbose` (stream output của từng command ngay khi chạy, thụt dưới task tương ứng; no-op khi có `--json`).

**Build mode.** `--build=auto` (mặc định) quyết định theo sự hiện diện, không dò
đoán môi trường: có thư mục artifact — `--artifact-dir <dir>` hoặc
`deploy.artifact_dir` của dự án — nghĩa là đã build xong, nên mode là `artifact`;
ngược lại là `server`. `--build=artifact` mà không có thư mục artifact là lỗi
cách dùng, chứ không âm thầm quay về build trên server — đúng cái mà mode này
sinh ra để tránh.

**Mô hình CI hai job.** Điểm mấu chốt của artifact mode là job chạm vào
production không cần toolchain:

| Job | Image cần gì | Lệnh |
|---|---|---|
| `build` | PHP, Composer, Node — bất cứ thứ gì task build của recipe cần | `govard deploy build production --output artifacts --revision $CI_COMMIT_SHA` |
| `deploy` | govard, ssh, rsync — không gì khác | `govard deploy production --artifact-dir artifacts --revision $CI_COMMIT_SHA --yes` |

`govard deploy build` materialise revision vào thư mục output, chạy các task build
của recipe ở đó, rồi ghi `manifest.json` gồm revision, phiên bản PHP, hash
`composer.lock` và danh sách sha256 của từng file. Lệnh này không cần target và
không cần container runtime. Job deploy kiểm tra manifest khớp với revision đang
triển khai và so phiên bản PHP đã ghi với PHP của server, từ chối kèm thông báo
hành động được nếu lệch — nhờ vậy image CI không khớp server bị chặn trước khi
publish. Deploy artifact bỏ qua 5 task build và chạy `deploy:artifact` thay thế;
`govard deploy plan` cho biết đang ở nhánh nào.

Thư mục output không rỗng sẽ bị từ chối để một file cũ từ lần build trước không
thể lọt ra production: dùng `--force` nếu muốn thay nội dung.

Flag của `govard deploy build`: `--remote`, `--output` (bắt buộc), `--branch`,
`--revision`, `--tag`, `--force`, `--command-timeout`, `--json`, `--runner`
(`host` là mặc định, hoặc `container`), `--no-cache`. Lệnh này không cần capability nào: `none`.
Chỉ remote `sandbox` cache artifact của nó (`.govard/sandbox/build-cache`, khoá theo
commit, hash `composer.lock`, series PHP, chế độ, deploy settings, hook và lệnh recipe đang hiệu lực, phiên bản công cụ của máy build, và binary govard); `--no-cache`
buộc build lại ở đó và được chấp nhận rồi bỏ qua với mọi remote khác.
`--runner container` là flag duy nhất có đòi hỏi — container app của chính dự án,
thiếu nó thì thoát với mã `3` — và nó còn đòi `--output` nằm trong project root để
container tới được artifact nó đang build; output nằm ngoài đó bị từ chối trước cả
lúc govard tìm Docker, như một lỗi cấu hình (exit `4`).

Container chỉ mang toolchain riêng của container app, nên khi cần build sẽ trộn runner,
và kiểm tra điều đó trước khi thay đổi bất cứ thứ gì. Khi bước frontend của recipe sẽ
chạy (`deploy.settings.frontend_dir` nêu ít nhất một thư mục), build tìm `node` và `npm`
trong container trước task đầu tiên. Container có đủ cả hai thì giữ bước đó. Container
thiếu một trong hai thì bước frontend chạy trên **host** (host phải có `node` và `npm`;
các bước PHP và Composer vẫn ở trong container), timeline in một dòng `runner:` cho mỗi
bước (ví dụ `runner: host (node not in container)`), và manifest ghi `frontend_runner`
cạnh `php_version` của container. Chỉ khi cả hai phía đều không có Node mới từ chối với
exit `3`, nêu tên thứ mỗi phía còn thiếu và lối ra (cài Node trong container hoặc trên
host, vì `--runner host` cũng cần Node trên host; hoặc, với bước dùng placeholder bên dưới, để `frontend_dir` rỗng). Envelope `--error-json` của lần từ chối đó báo `capability: "node"`, một giá trị
mà `govard capabilities` không liệt kê vì nó không phải capability khai báo được. Kiểm
tra này dựa vào placeholder <span v-pre>`{{settings.frontend_dir_args}}`</span> của recipe, nên một dự án
có bước frontend (recipe override hoặc command dạng hook) không dùng placeholder đó vẫn
bị kiểm tra Node, và `frontend_dir` rỗng không bỏ qua kiểm tra đó.
Project container đang dừng hoặc không tồn tại cũng thoát `3`
(gợi ý: `govard env up`, hoặc `--runner host`). Cả hai lần từ chối đều đến trước khi
output directory bị đụng tới, nên artifact trước đó còn nguyên. Ctrl-C và
`--command-timeout` cũng dừng bước đang chạy bên trong container: sau khi client
`docker exec` ở local bị dọn, một `docker exec` thứ hai gửi tín hiệu tới process group
của bước đó, và lỗi nói rõ phần teardown có được thử hay không và có hoàn tất hay không.

**Setting và credential.** `deploy.settings` được đối chiếu với recipe trước khi
chạy: key lạ, hoặc giá trị sai dạng, thoát với mã 4 kèm tên key và gợi ý key gần
đúng. Setting dạng chuỗi phải quote nếu trông giống số — engine đọc chúng dưới dạng
chuỗi. `COMPOSER_AUTH` từ môi trường được chuyển tới bước cài dependency qua
standard input (không bao giờ nằm trong command), và `shared/auth.json` trên target
cũng dùng được; `govard deploy check` cho biết đang dùng nguồn nào và cảnh báo khi
build trên target cần mà không có.

**`deploy check`.** Preflight không để lại gì trên target, dù là local hay remote. Probe
quyền ghi của nó không tạo path nào: một `deploy_path` chưa tồn tại được kiểm tra tại
parent tồn tại gần nhất, ngay trên target, và note của target local nêu rõ parent nào
đã trả lời (một lệnh gọi remote không có note, vì exit code không phân biệt được path
chưa tồn tại với path không ghi được). Probe `mv -T` chạy trên target dạng symlink tạo
một thư mục tạm `.dep` dưới deploy path, và trên một host mới thì cả các tầng còn thiếu
phía trên nó, rồi xoá chúng đi, dừng ở ancestor gần nhất đã tồn tại và giữ lại tầng nào
còn chứa thứ gì. Preflight không bao giờ nhìn vào deploy lock. Các note của nó in ra
trước header `Target <remote> is deployable`, mỗi note một dòng, đánh dấu `  - ` cho
dữ kiện và `  ! ` cho cảnh báo, và một lần deploy thật in đúng các note đó trong output
của bước `deploy:check` (ra stderr khi có `--json`, để stdout vẫn là một document).

**Output cho máy đọc.** Với `--json`, stdout chứa đúng một JSON document còn
timeline cho người đọc đi ra stderr: `schema_version`, `remote`, `branch`,
`revision`, `release`, `build.mode`, `publish.strategy`,
`publish.previous_release`, `verify`, `result`, `duration_ms` và
`tasks[{id,stage,status,duration_ms}]`. Deploy lỗi phát ra cùng document với
`result: "failed"` và `error`, thoát mã 1. Release record mang `ci.pipeline`/`ci.job`
khi lần chạy là CI.

`govard deploy plan --json` phát ra một document **khác** — `kind: "plan"`, để
pipeline dùng cả hai phân biệt bằng field chứ không phải đoán theo hình dạng — mô
tả lần chạy **sẽ** làm gì: `schema_version`, `kind`, `remote`, `branch`, `revision`,
`build.mode`, `publish.strategy` cùng `publish.decided_by` (`configuration`, hoặc
`target` khi strategy là `auto` và chỉ có kết nối mới resolve được), và
`steps[{index,id,kind,stage,title,run_on,source,implementation,command,skipped,skip_reason,needs_application}]`.
`implementation` là `engine`, `command` hoặc `none`; `skipped` phản ánh đúng
executor, nên cả bước bị build mode bỏ qua lẫn task không recipe nào điền đều là
`skipped`, mỗi bước mang `skip_reason` của nó. `run_on` là thứ cây cho người đọc
không thể hiện được: hook khai `run_on: local` trông y hệt mọi bước khác ở đó.
Document không có timestamp, nên hai lần chạy cùng một plan giống nhau từng byte và
CI diff được. Không có kết nối nào: `deploy plan` không cần ssh, rsync hay Docker.

`deploy.lock_stale_after` (2h) và `deploy.maintenance_timeout` (15m) là hai timeout
ngoài `command_timeout`: cái đầu là ngưỡng để `deploy unlock` nhả lock không cần
`--force`, cái sau chặn một bước trong maintenance window.

**Sandbox.** `govard sandbox up` build một container, publish SSH trên một
cổng loopback còn trống, sinh khoá riêng dưới `.govard/sandbox/` (đã gitignore),
và mount read-only một mirror repository local. Mirror được refresh trước mỗi lần
deploy nên commit bạn chưa từng push vẫn triển khai được, và không phần nào trong
pipeline biết nó đang nói chuyện với container — triển khai vào sandbox chính là deploy
production trỏ vào container. `sandbox ssh -- <command>` chạy lệnh trong sandbox mà không ép tty, nên dữ liệu pipe vào
hoạt động (`printf x | govard sandbox ssh -- 'cat > f'`), và govard thoát với đúng exit
status của lệnh đó vì tiến trình được thay bằng `ssh`. Đây là chủ ý riêng cho lệnh này: các
bước deploy vẫn không bao giờ lộ exit code của remote. Các từ sau `--` được nối bằng dấu
cách thành một dòng lệnh remote; lệnh không có `--` là lỗi cách dùng (exit `2`).

Khi container còn chạy, `sandbox` tự resolve thành
remote cho mọi lệnh nhận remote, mà không gì identity nào được ghi vào file cấu
hình.

`sandbox` là lệnh top-level — hãy deploy bằng dạng flag:
`govard deploy --remote sandbox --yes`.

Không có *identity* sandbox nào để ghi vào đâu cả: `host`, `port`, `user`,
`path` và `auth` của `sandbox` được đọc từ container mà `sandbox up` tạo ra, và
`govard remote add` sẽ bỏ qua chúng nếu bạn truyền vào. Thứ bạn có thể cấu
hình là **hình dạng** của buổi diễn tập — `capabilities`, `protected` và cấu
hình `deploy` — qua `govard remote add sandbox --capabilities db --protected`
hoặc bằng tay trong `.govard.yml` / `.govard.local.yml`:

```yaml
remotes:
  sandbox:
    capabilities:
      db: false
    protected: true
    deploy:
      keep_releases: 3
```

Block được lớp **lên trên** remote synthetic, không thay thế nó. Bốn cấu hình
`deploy` bị ghim vào container vì chúng mô tả đúng thứ image mang theo và một
lần deploy sẽ từ chối chạy khi đích không khớp: `owner`, `writable_mode`,
`php_bin` và `php_version`. Phần còn lại của những gì một override per-remote
copy thì thuộc về bạn — `keep_releases`, `command_timeout`, `artifact_dir`,
`repository`, `branch`, `publish`, `deploy_path`, `db_backup`, `verify.url`,
`verify.timeout`, `hooks` và mọi khoá `settings` (trong đó có
`writable_permissions` và `composer_bin`). `deploy_path` là trường duy nhất
dời được buổi diễn tập, và nó chỉ dời *bên trong* container.
`lock_stale_after`, `maintenance_timeout` và `verify.follow_redirects` chỉ đọc ở
cấp project: block `remotes.sandbox` bị bỏ qua với chúng, nên hãy đặt chúng
trong block `deploy:` cấp project.

Profile: `basic` (sshd, rsync, git), `php` (thêm php-cli, composer, node) và
`full` (thêm database và cache), mặc định `php`. `--docroot` định hình target để
chiến lược publish resolve đúng thứ bạn muốn kiểm chứng: `absent` hoặc `symlink`
(mặc định) chọn cú swap nguyên tử, `real` chọn in-place. `down` xoá container —
remote `sandbox` ẩn chỉ tồn tại khi container còn đó — nhưng giữ mọi data volume
để mai diễn tập tiếp (`--volumes`
xoá luôn data); `--purge` xoá thêm image, khoá và mirror. `reset` xoá các thư mục deploy
trên target, và `--layout=deployer` seed một target trông như của công cụ deploy kia.

Profile `php` và `full` còn ship một web tier — nginx phục vụ served path cộng
`stack.web_root` của dự án (`/pub` với Magento), và PHP-FPM chạy bằng chính user
deploy. `up` trỏ `deploy.verify.url` của remote sandbox vào cổng đó, nên deploy vào
sandbox diễn tập nửa HTTP của `deploy:verify`; `basic` không có web tier và không
quảng cáo URL nào. Recipe của framework đóng góp những gì command của nó cần ngoài
profile (với Magento: thư viện build, PHP extension và hai service database cùng
cache). Dự án cần thêm một extension, một apt package, một service mà image không
start, hoặc một binary mà engine biết cách cài thì khai vào danh sách
`deploy.settings.sandbox_*` tương ứng — `sandbox_packages`, `sandbox_extensions`,
`sandbox_services`, `sandbox_tools` — không phải bằng flag. Mỗi danh sách **thay
thế** danh sách của recipe chứ không nối thêm, nên dự án dùng PostgreSQL khai
`postgresql` thay vì `mariadb`, không phải thêm vào; giá trị mặc định theo từng
recipe nằm ở
[Triển khai](/vi/workflows/deployment#sandbox-recipe-defaults).

Sandbox là lệnh deploy duy nhất cần `docker`.

`--resume` tiếp tục release mới nhất có record chưa `ok`; `--from <task>` bắt đầu
từ một task hoặc hook được chỉ định và báo mọi bước trước đó là skipped. Cả hai
đều là đường phục hồi do người vận hành chủ động yêu cầu, không tự động. Một
`--from` vượt qua `deploy:release` còn cần `--resume`: run sẽ không bao giờ tạo số
release, nên `{{release_path}}` sẽ trỏ vào thư mục chứa mọi release.

`govard deploy rollback` không bao giờ build lại: layout symlink được trỏ lại,
còn layout in-place chạy lại phần publish từ thư mục release đã có trên server.
`--with-db` phục hồi dump mà release chạy **sau** release được khôi phục đã ghi lại
— lần deploy đó dump database trước các migration của nó, đúng trạng thái mà release
đích cần — và sẽ phá huỷ dữ liệu hiện tại, nên cần `--yes` (hoặc xác nhận tương
tác). Việc restore chạy trước bước flush cache và bước verify, vì cả hai đều đọc
database.

Exit code: `0` thành công, `1` lỗi thực thi, `2` sai cách dùng, `3` thiếu
capability, `4` lỗi cấu hình. `govard deploy` và `govard deploy rollback` cần
`ssh` và `rsync`; `deploy check`, `deploy releases`, `deploy status` và
`deploy unlock` chỉ cần `ssh`; `deploy build` và `deploy plan` không cần gì.
`govard sandbox *` là ngoại lệ: tạo server giả cần `docker`, sau đó govard
nói chuyện với nó qua SSH như mọi target khác.

`deploy check` và `deploy status` dùng chung các mã cấu hình và dùng lệnh. Một key `deploy.settings` gõ sai,
một project không load được, hoặc một project không có remote nào là lỗi cấu hình
(`4`); một remote positional mâu thuẫn với `--remote` là lỗi dùng lệnh (`2`), cũng như
`deploy check` không nêu remote nào. Exit `1` thì khác: với `check` nó nghĩa là preflight
fail hoặc target được nêu tên không tới được, với `status` nó nghĩa là không remote nào đã
cấu hình tới được. `deploy status --json` vẫn in mảng các dòng
theo từng remote (dòng không tới được có status `unknown` và một `error`) nhưng thoát
`1` khi mọi remote đều không tới được, như chế độ bảng, với lý do ra stderr và stdout
vẫn là một document khi có `--error-json`. Khi không có remote nào được cấu hình, hoặc
project không load được, nó thoát `4` với stdout rỗng.

### `govard sandbox`

Một container ngay trên máy bạn đóng vai đích triển khai — vòng đời top-level
của target diễn tập:

```bash
govard sandbox up                      # tạo (mặc định profile php)
govard sandbox up --profile basic      # chỉ sshd, rsync và git
govard sandbox up --profile full --php 8.4   # database, cache, PHP 8.4
govard sandbox up --profile full --db mariadb:10.6   # choose the database series
govard sandbox up --docroot real       # docroot thật: publish in-place
govard sandbox status
govard sandbox reset --layout deployer # seed target mà công cụ kia đang giữ
govard sandbox ssh                     # shell tương tác
govard sandbox ssh -- php -v           # chạy một lệnh
govard sandbox down [--purge] [--volumes]
```

Khi container còn chạy, `sandbox` tự resolve thành remote cho mọi lệnh nhận
remote (`deploy`, `db`, `remote exec`, `sync`):
`govard deploy --remote sandbox --yes`, `govard sync -e sandbox`,
`govard remote exec sandbox -- <command>`. Không gì *identity* nào được ghi
vào file cấu hình, và `govard remote list` hiện dòng synthetic
`sandbox (implicit) | …` cạnh các remote đã cấu hình. Hình dạng của buổi diễn
tập thì cấu hình được (xem [Cấu hình remote sandbox](/vi/workflows/remotes-and-sync#cấu-hình-remote-sandbox)).
Trên sandbox mới tinh dạng mặc định (`symlink`),
`remote exec` lỗi cho tới lần deploy đầu tiên điền đầy current path — hãy deploy
lần đầu hoặc dùng `--docroot real`.

`--db` chọn database mà profile `full` cung cấp, `mariadb:<series>` (ví dụ
`mariadb:10.6`) hoặc `default` để dùng server của distribution gốc. Với sandbox
mới, mặc định lấy từ `stack.services.db` và `stack.db_version` của dự án; sandbox
đã có giữ database nó đang ship, và `--db` không khớp bị từ chối trừ khi bạn truyền
`--recreate`. Đường `--recreate` kiểm tra database trước, nên database bị từ chối
không làm mất container đang chạy. Phần tóm tắt của `sandbox up` in thêm dòng
`database:` (giữa `php:` và `image:`) nêu server đã cài và việc nó được yêu cầu hay
là mặc định của distribution gốc; profile không có database thì bỏ dòng này.

Profile `full` giữ database trong named volume `<sandbox container name>-db`: `down`
giữ nó, `down --purge` xoá nó, và volume do series database khác ghi ra bị từ chối
kèm gợi ý `--purge`. `up` chỉ seed database trống; `--reseed` làm mới database và
file từ môi trường gốc (bị từ chối khi đi cùng `--no-seed`), và `--recreate` giữ volume.

→ Hướng dẫn đầy đủ: [Triển khai](/vi/workflows/deployment#sandbox) và phần vòng lặp nhanh.

### `govard gateway`

Bastion SSH dùng chung (`govard-proxy-sshd`) do `govard svc up` khởi động,
lắng nghe trên `127.0.0.1:2222` — một địa chỉ ổn định cho mọi sandbox thay vì
cổng tạm mà mỗi lần `sandbox up` chọn:

```bash
govard gateway allow-key "$(cat ~/.ssh/id_ed25519.pub)"
ssh -p 2222 <project-name>@127.0.0.1
sftp -P 2222 <project-name>@127.0.0.1
```

- `govard gateway status` cho biết container bastion có đang chạy không và nó
  biết bao nhiêu target/key.
- `govard gateway allow-key <public-key-line>` /
  `govard gateway revoke-key <fingerprint-or-comment>` quản lý allowlist; cả
  hai đều chạy được khi Docker chưa khởi động (registry là một file cục bộ).
- `sandbox up` tự đăng ký username của dự án và nối vào network của bastion;
  `sandbox down` gỡ đăng ký đó. Không thao tác nào hỏng khi gateway chưa chạy
  — `deploy`, `deploy check` và `remote *` không bao giờ đi qua nó.

→ Hướng dẫn đầy đủ: [Triển khai](/vi/workflows/deployment#shared-ssh-gateway).

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

Các file snapshot chứa database dump chỉ thuộc về chủ sở hữu: `create` ghi `db.sql.gz` với
mode `0600`, và `export` tạo archive với mode `0600` (siết một file đã tồn tại mà rộng quyền
hơn). `create -e` ở remote chạy dưới `umask 077`, nên các thư mục nó tạo, như
`<path>/.govard/snapshots`, là `0700` và file trong đó là `0600`, và nó kết thúc bằng việc
ghi `metadata.yml` của snapshot (tên, `created_at`, framework).

Vị trí lưu snapshot remote: với remote có deploy layout (`releases/`, `shared/` và link
`current` dưới deploy path), `create -e` và `push` lưu vào
`<deploy path>/shared/.govard/snapshots`, nằm ngoài release đang được phục vụ và không bị
mất khi chuyển hay dọn release. Deploy path là `remotes.<name>.deploy.deploy_path` nếu có,
nếu không thì là `path` của remote (thư mục cha khi path là link `current`). Remote không có
layout đó giữ vị trí cũ `<remote path>/.govard/snapshots` và `create` sẽ cảnh báo. `list`,
`restore`, `delete` và `pull` tìm ở cả hai vị trí (deploy layout trước), nên snapshot tạo bởi
bản cũ vẫn hiện; `list -e` thêm cột `LOCATION` (`shared` hoặc `legacy`).

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

`open admin` mở `/admin` cho framework không có route admin mặc định. Với Laravel và Symfony
(không có trang admin riêng) lệnh in một dòng thông báo nói rõ điều đó và path đã mở;
WordPress mở `/wp-admin`. Không có khóa cấu hình admin path.

`open db` in connection URL với mật khẩu được che bằng `***` (không có phần mật khẩu khi
chưa đặt); database client nhận URL đầy đủ.

### `govard tunnel`

Quản lý các đường link public tunnel (`start` yêu cầu `cloudflared`; `stop` và `status` thì không). Govard đăng ký domain tunnel trong Caddy như alias, giữ nguyên `Host` header, và tự động rewrite base URL của framework (Magento, Laravel, …) — khôi phục khi `tunnel stop` hoặc `Ctrl+C`.

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

`start` ghi lại tiến trình mà nó khởi chạy (PID cùng argv đã dùng để khởi chạy) vào
`$GOVARD_HOME_DIR/tunnels/<project>.pid`, và từ chối khởi chạy tunnel thứ hai khi cái đó vẫn còn
sống. Bản ghi được tạo độc quyền, nên hai lần `start` chạy đua cho cùng một project không thể
cùng thắng: bên thua dừng tiến trình provider nó đã khởi chạy, để nguyên bản ghi và base URL của
bên thắng và thất bại với cùng lời từ chối, còn một bản ghi mà tiến trình đã chết, hoặc PID của nó
giờ chạy thứ khác, sẽ được lần `start` kế tiếp thay thế. `stop` và `status` đọc bản ghi đó thay vì dò trên máy, nên không lệnh nào đụng tới một
`cloudflared` mà govard không khởi chạy — trừ khi nó chạy đúng dòng lệnh mà bản ghi ghi, thứ mà một
bản cài khác của cùng binary không phân biệt được: `stop` chỉ gửi tín hiệu tới PID đã ghi và chỉ khi
argv của nó vẫn khớp, còn lại từ chối kèm lỗi — không gửi tín hiệu nào — nếu không khớp hoặc không đọc được
argv. `status` báo `INACTIVE` với bất kỳ tiến trình nào govard không thể nhận là của mình, vì
"govard không có tunnel nào ở đây" vẫn đúng ngay cả khi tiến trình khác đang giữ PID đó. Khi không
có bản ghi, `stop` không làm gì và thoát với mã 0.

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
govard config get deploy.settings.php_bin     # giá trị deploy sẽ expand
govard config set deploy.keep_releases 3
govard config profile              # Hiển thị cấu hình profile đề xuất cho framework
govard config profile --json      # Output thông tin profile dạng JSON
govard config profile apply       # Áp dụng profile đề xuất vào .govard.yml
govard config auto                # Magento 2: inject các thiết lập kết nối vào env.php
```

`config auto` chạy bước sửa cấu hình của Magento (`app:config:import`, và nếu
import chưa đủ thì chuyển sang `setup:upgrade`) ngay khi một lệnh báo rằng cần
đến nó. Khi bước đó để lại `app/etc/config.php` với đúng các dòng cũ nhưng khác
thứ tự, Govard ghi lại đúng các byte gốc để file đang được track không bị bẩn, kể cả sau lần thử lại khi một search index read-only được gỡ khoá.
Một thay đổi module thật sẽ được giữ nguyên, và một thao tác khôi phục mà Govard
không ghi được — checkout read-only, hoặc config.php thuộc container — chỉ được
báo dưới dạng cảnh báo, không phải báo lỗi Magento.

`config get` đọc được cả block `deploy:` bên cạnh các key của project và stack:
`deploy.keep_releases` (số lượng hiệu lực), `deploy.command_timeout`,
`deploy.maintenance_timeout`, `deploy.lock_stale_after`, `deploy.artifact_dir`,
`deploy.db_backup`, `deploy.verify.url`, `deploy.verify.timeout`, và mọi key dưới
`deploy.settings.` — kể cả key dự án chưa từng khai, khi đó trả về rỗng chứ không
phải lỗi "unknown key". Danh sách trả về dạng ngăn cách bởi dấu phẩy, và
`config set` ghi ngược lại thành danh sách; setting vốn là chuỗi thì vẫn là chuỗi.
`config set` từ chối giá trị không thể đúng kiểu (`deploy.keep_releases` phải là số
nguyên, `deploy.db_backup` là true/false) thay vì lưu số 0. Việc một key
`deploy.settings` có tồn tại hay không là câu trả lời của recipe, nên gõ sai sẽ
được `govard deploy plan` báo, không phải lệnh này.

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
govard varnish log
govard varnish ban /.*
govard varnish stats
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
govard elasticsearch _cluster/health
govard opensearch _cluster/health
# Truy vấn search nhận một path duy nhất và luôn chạy GET; tham số thừa bị bỏ qua.
# Muốn dùng curl với cờ tùy ý, hãy curl từ host tới http://<domain>:9200.
```

Host truy cập search cũng được route tự động qua `http://<domain>:9200` (xem [Cấu hình](/vi/reference/configuration#truy-cap-elasticsearch-opensearch-tu-host)) — shortcut trên exec trong container, còn route `:9200` dành cho `curl`/browser trên host.

---

## 🌐 Các cờ toàn cục (Global Flags)

Tất cả các lệnh của Govard đều hỗ trợ:

- `-h, --help` — Hiển thị trợ giúp của lệnh
- `--verbose` — Bật log có cấu trúc chi tiết
- `--error-json` — In lỗi ra stdout dưới dạng envelope JSON machine-readable
  (`schema_version`, `code`, `capability`, `command`, `message`, `hint`) thay vì
  text, để script không phải parse dạng người đọc

Các lệnh forward tham số cho tool khác (`govard tool php ...`,
`govard redis cli ...`) không parse được `--error-json`; lỗi của chúng vẫn giữ
đúng mã thoát đã tài liệu hóa.

Mã thoát ổn định trên toàn CLI: `0` thành công, `1` lỗi thực thi, `2` lỗi usage,
`3` `CAPABILITY_MISSING` (một yêu cầu runtime đã khai báo không có sẵn), `4` lỗi
cấu hình. Danh sách lệnh không cần container runtime nằm ở
[Chạy không cần Docker](/vi/reference/docker-free).

---

[← Bắt đầu](/vi/getting-started/getting-started) | [Cấu hình →](/vi/reference/configuration)
