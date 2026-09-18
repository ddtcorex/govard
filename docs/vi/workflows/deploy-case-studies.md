---
title: Case study triển khai
description: Những cấu hình triển khai govard cụ thể cho dự án Magento — Luma, Hyvä, nhiều theme và store view, chế độ developer và production, docroot symlink hoặc docroot thật — mỗi ca kèm cấu hình, lần diễn tập trong sandbox và lỗi mà nó ngăn được.
---

# Case study triển khai

[Triển khai](/vi/workflows/deployment) mô tả engine: pipeline trung tính, recipe,
hai build mode, publish, verify và rollback. Trang này là nửa còn lại — một **dự án
cụ thể** đặt gì vào `.govard.yml`, cách diễn tập đúng dự án đó trong sandbox, và
một lần chạy xanh và một lần chạy đỏ trông như thế nào.

Mỗi ca ở đây là một hình dạng thật, theo đúng thứ tự mà một team thường gặp: một
store Luma nguyên bản, một storefront Hyvä, nhiều storefront trong một release, và
hai kiểu layout docroot mà target có thể đã có sẵn. Hãy đọc mục đầu tiên, tìm dòng
của bạn trong bảng, rồi đọc ca của bạn.

## Ba quyết định định hình một ca

Không gì khác trong file này quan trọng bằng ba câu trả lời sau.

### 1. Build chạy ở đâu

| `--build` | Ai chạy các task build | Dùng khi |
| --- | --- | --- |
| `server` | target, qua SSH | hotfix từ laptop; dự án chưa có CI |
| `artifact` | máy chạy `govard deploy build`, trong CI | job chạm vào production không được có toolchain |

`auto` (mặc định) resolve **theo sự hiện diện**: có thư mục artifact nghĩa là build
đã xong, ngược lại target tự build. Nó không bao giờ dò đoán môi trường.

Recipe đánh dấu những bước cần ứng dụng đã được deploy — với Magento,
`setup:static-content:deploy` là bước quan trọng nhất, vì nó đọc cấu hình store,
website và locale từ database. Ở artifact mode, những bước đó được **để lại trong
deploy** và chạy trên target sau khi `deploy:artifact` bung artifact. Thành ra:

| Chạy trên máy build (artifact mode) | Chạy trên target (cả hai mode) |
| --- | --- |
| Composer install, patches, DI compile, build frontend (Node) | static content, `setup:upgrade`, import cấu hình, flush cache, verify |

Máy build không cần database, web server hay `app/etc/env.php`; job deploy chỉ cần
govard, SSH và rsync, không gì khác.

### 2. Release trở thành live thế nào — hình dạng docroot

`remotes.<name>.path` là **docroot đang được phục vụ**. `remotes.<name>.deploy_path`
là **layout root** chứa `releases/`, `shared/` và `.dep/`. Đó là hai thư mục khác
nhau, và docroot thuộc kiểu nào sẽ quyết định toàn bộ đường publish:

| Docroot đang phục vụ… | Chiến lược resolve ra | Việc kích hoạt làm gì |
| --- | --- | --- |
| không tồn tại (lần deploy đầu) | `symlink` | tạo docroot thành symlink trỏ tới `releases/<n>`, nguyên tử |
| là symlink | `symlink` | trỏ lại bằng `mv -T` — không khách nào thấy một cây file publish dở |
| là thư mục thật | `in_place` | `git reset --hard` trong docroot, copy `sync_paths` vào, ghi file static version cuối cùng |

Đây chính là "docroot symlink" so với "docroot không symlink" ở bên dưới. `auto`
đọc target thay vì đoán; `publish: symlink` / `publish: in_place` trên remote ghi
đè cho target mà phép dò không phân loại được (ví dụ docroot là thư mục thường
nhưng không phải git checkout).

::: warning Docroot in-place chính là cái cần `sync_paths`
`git reset --hard` để nguyên những thư mục gitignored của lần deploy trước đúng chỗ
cũ. Path không được copy là path site giữ lại từ release **cũ** — code mới nằm trên
`vendor/`, `generated/` và `pub/static/` cũ mà vẫn báo deploy thành công. Recipe
Magento ship sẵn danh sách (`vendor`, `generated`, `pub/static/adminhtml`,
`pub/static/frontend`); dự án ghi đè danh sách đó thì tự chịu hệ quả.
:::

### 3. Target chạy ở mode nào

`mage_mode` **mô tả** target; nó không đổi target. Govard không bao giờ chạy
`bin/magento deploy:mode:set`, và không chỗ nào khác trong pipeline đọc setting này —
đây là deploy setting, không phải setting của môi trường local, và tác dụng duy nhất
của nó là lên bước static content:

| `mage_mode` | `build:assets` có chạy | Ý nghĩa |
| --- | --- | --- |
| không đặt (mặc định) | có | hành vi production: mọi theme và locale đã cấu hình được compile lúc deploy |
| `production` | có | y như trên, nhưng nói rõ — mode mà target thật sự chạy |
| `developer` | không | target sinh static file theo nhu cầu, nên deploy chúng là việc vô ích và có rủi ro asset cũ |

Điều kiện chặn chỉ là so sánh chuỗi, nên giá trị rỗng hành xử y hệt `production`.
Chỉ đúng chuỗi `developer` mới bỏ qua bước này. Trên target ở developer mode, asset
đã compile được ứng dụng sinh ra ở request đầu tiên, nên deploy developer mode cho
một storefront lớn xong trong vài phút thay vì mất cả một phần tư giờ như static
content deploy ở production.

::: warning Setting chỉ giả định mode; nó không đặt mode
Hai hệ quả kéo theo, và cả hai đều im lặng:

- **Đặt `developer` cho một target đang chạy production không biến target đó thành
  developer.** `env.php` của nó vẫn không sinh gì theo nhu cầu, nên static file của
  theme chưa từng được deploy và cũng không được sinh khi có request — storefront trả
  `404` cho chúng. Kiểm tra target thật sự chạy gì bằng `bin/magento deploy:mode:show`
  trên target, rồi đặt `mage_mode` cho khớp.
- **Giá trị không đúng chính xác chuỗi `developer` sẽ deploy static content.**
  Validation chỉ đòi setting là string, không phải một tập đóng, nên một lỗi gõ như
  `Development` vẫn được chấp nhận và hành xử như production.
:::

::: info Môi trường nào thì dùng cái nào
Target staging mà team duyệt và debug thường là `developer`. Target production là
`production` (hoặc không đặt). Target staging **dùng chung** mà người ta đo hiệu
năng thì nên là `production`, vì việc sinh theo nhu cầu làm đổi các con số.
`govard deploy plan` cho thấy điều kiện chặn sẽ so sánh với giá trị nào: command
`build:assets` in ra mang sẵn nó, ví dụ `[ developer != developer ]` trên target ở
developer mode.
:::

## Chọn ca trong một cái nhìn

Chọn dòng khớp với dự án bạn đang deploy. "Webroot" là hình dạng của docroot đang
được phục vụ, theo quyết định 2.

| # | Dự án | Webroot | `mage_mode` | `frontend_dir` | Theme | Build |
| --- | --- | --- | --- | --- | --- | --- |
| [1](#case-1-luma-production-mode-symlinked-webroot) | Luma, một store view | symlink | production | (rỗng) | tất cả | server |
| [2](#case-2-luma-production-mode-real-webroot) | Luma, target thừa hưởng | thật | production | (rỗng) | tất cả | server |
| [3](#case-3-hyva-one-theme-production-mode) | Hyvä, một theme | symlink | production | một path | theme đó | server |
| [4](#case-4-hyva-developer-mode) | Hyvä, target để debug | symlink | developer | một path | theme đó | server |
| [5](#case-5-two-node-built-themes) | Hai theme kiểu Hyvä | symlink | production | hai path | cả hai | server |
| [6](#case-6-multi-store-hyva-storefront-with-a-luma-admin) | Hyvä + admin Luma, nhiều locale | symlink | production | một path | map | server |
| [7](#case-7-artifact-mode-in-ci) | bất kỳ ca nào ở trên | hoặc | production | như trên | như trên | artifact |
| [8](#case-8-the-in-place-target-you-inherited) | bất kỳ | thật | production | như trên | như trên | hoặc |

Lệnh sandbox cho mỗi dòng đều cùng một dạng: dựng target, rồi deploy lên remote
`sandbox` ẩn. Các mục theo từng ca đưa ra lệnh chính xác.

## Ca 1: Luma, chế độ production, docroot symlink {#case-1-luma-production-mode-symlinked-webroot}

Hình dạng mặc định, và là chỗ để bắt đầu: một storefront nguyên bản, một locale,
một target có docroot là symlink trỏ vào layout release.

**Hình dạng dự án.** `Magento/luma` (hoặc theme con của nó), không có bước build
Node, `stack.web_root: /pub`, một store view.

**Cấu hình.** Hầu như không cần gì — mặc định của recipe chính là ca này. Cứ ghi ra,
để file nói rõ deploy giả định điều gì:

```yaml
project_name: acme-shop
framework: magento2

stack:
  web_root: /pub            # nginx của sandbox phục vụ <current>/pub nhờ dòng này

deploy:
  settings:
    mage_mode: production
    static_content_locales: [en_US]

remotes:
  production:
    host: m2.example.com
    user: m2-deploy
    path: /home/m2-deploy/public_html     # docroot đang được phục vụ: một symlink
    deploy:
      deploy_path: /home/m2-deploy/.deployer  # releases/, shared/, .dep/
      settings:
        php_bin: php8.3
        composer_bin: composer
        php_version: "8.3"
        owner: m2-deploy:m2-deploy
        writable_mode: chmod+chown
      verify:
        url: https://shop.example.com/
    auth:
      method: keyfile
      key_path: ~/.ssh/m2-production
```

**Cái gì chạy ở đâu.** `server`: trên target, `composer install --no-dev`, DI
compile, rồi `setup:static-content:deploy` cho `en_US` trên các theme, sau đó
`setup:upgrade`, `app:config:import`, `cache:flush`. `build:frontend` có chạy nhưng
không làm gì (không có `frontend_dir`). Publish là một cú swap symlink; maintenance
window mở vì plan có import cấu hình và migrate.

**Diễn tập.**

```bash
govard sandbox up --profile full --php 8.3   # DB + cache + web tier
govard sandbox status
govard deploy --remote sandbox --yes
govard deploy releases sandbox
```

Hãy dùng `--profile full`: deploy Luma đi tới `setup:upgrade` (cần database và một
search engine được hỗ trợ) và `cache:flush` (cần cache backend mà `env.php` của
target khai). `basic` không có PHP lẫn Composer, nên dự án Magento dừng ở
`build:vendors`; `php` có toolchain nhưng không có database, nên nó dừng ở
`db:migrate`.

**Cần chờ đợi gì.** Lần deploy đầu tiên (nguội) là lần chậm: Composer tải mọi thứ,
DI compile quét toàn bộ codebase, và static content được compile theo từng theme và
locale. Trên một dự án 2.4.9 thật, kích thước trung bình, riêng lượt static ở
production mode đã mất vài phút. Thứ làm lần deploy sau rẻ hơn là target giữ Composer
cache giữa các release và `setup:upgrade` chạy với `--keep-generated`, nên bước cài
dependency tải ít hơn. Hai lần chạy đo được của dự án này: một lần build server ở
developer mode xong trong 2m35s, và một lần deploy production mode nhận artifact mất
14m35s — khác biệt chủ yếu nằm ở lượt static content, thứ mà lần chạy developer mode
bỏ qua hoàn toàn.

**Những gì hay hỏng.**

| Thông báo | Ý nghĩa |
| --- | --- |
| `The default website isn't defined` | database của target không có cấu hình store; target chưa phải một ứng dụng đã cài |
| `Your current search engine, 'MySQL', is not supported` | `setup:upgrade` trên dự án cấu hình cho Elasticsearch/OpenSearch mà không có cluster nào kết nối được |
| `Connection "default" is not defined` | `shared/app/etc/env.php` bị thiếu hoặc đã bị bản sao của artifact đè lên |
| `setup:db:status` lỗi ở bước verify, sau khi publish xanh | release đã nằm đúng chỗ nhưng ứng dụng không trả lời được — lỗi ở `env.php` hoặc database, không phải ở deploy |

## Ca 2: Luma, chế độ production, docroot thật {#case-2-luma-production-mode-real-webroot}

Cùng ứng dụng đó, trên một target mà ai đó đã deploy bằng tay: thư mục đang phục vụ
là thư mục thật và là git checkout, không phải symlink.

**Hình dạng dự án.** Y hệt case 1. Khác biệt nằm hoàn toàn trên server.

**Cấu hình.** Remote mô tả nó; thêm đúng một setting mà chiến lược này cần:

```yaml
remotes:
  legacy-staging:
    host: staging.example.com
    user: deploy
    path: /var/www/shop                # một thư mục THẬT, được phục vụ trực tiếp
    deploy:
      deploy_path: /var/www/shop          # releases/ và shared/ nằm dưới nó
      settings:
        php_bin: php8.2
        composer_bin: composer
        php_version: "8.2"
        owner: www-data:www-data
        writable_mode: acl
        # Danh sách in-place. Giữ các entry của recipe và thêm những gì dự án này
        # build ra dưới một path gitignored.
        sync_paths: [vendor, generated, pub/static/adminhtml, pub/static/frontend]
```

Không cần `publish: in_place` khi docroot thật sự là thư mục — `auto` resolve ra nó.
Hãy ghi tường minh nếu thư mục đó không phải git checkout mà bạn vẫn muốn hành vi
in-place, để một thay đổi layout sau này không âm thầm đổi chiến lược.

**Cái gì chạy ở đâu.** `prepare` fetch revision ngoài window; sau đó
`publish:activate` reset docroot về đúng revision đó, copy `sync_paths` kèm
`--delete`, khôi phục các shared link và ghi `pub/static/deployed_version.txt`
**cuối cùng**. Maintenance window mở suốt cả bước kích hoạt — docroot bị ghi đè
trong lúc đang được phục vụ, nên không có thời điểm nào để mà khéo léo.

**Diễn tập.** Sandbox dựng được đúng hình dạng này:

```bash
govard sandbox up --profile full --php 8.2 --docroot real
govard deploy check sandbox          # mong đợi: publish in_place
govard deploy plan sandbox           # mong đợi: nhánh in-place của publish
govard deploy --remote sandbox --yes
```

`--docroot real` seed docroot thành một checkout của mirror — đúng trạng thái mà
một target chưa từng được deploy tới thật sự đang ở. Điều đó quan trọng: điều kiện
chặn maintenance hỏi *ứng dụng đang được phục vụ* có chạy được không (`bin/magento`
**và** `vendor/autoload.php`), nên một checkout không có dependency được coi đúng là
không có gì để bảo vệ, và deploy không chết ở `maintenance:enable`.

**Cần chờ đợi gì.** Deploy in-place không chậm hơn deploy symlink, nhưng nó ít dễ
dãi hơn: chính `pub/static` đó bị ghi đè ngay dưới lưu lượng đang chạy, đó là lý do
window tồn tại.

**Những gì hay hỏng.**

| Thông báo | Ý nghĩa |
| --- | --- |
| Site phục vụ PHP mới với asset cũ | một path mà release build ra bị thiếu trong `sync_paths` |
| `deployed_version.txt` không khớp ở bước verify | static content của docroot không phải của release — bước sync copy sai tập hợp, hoặc `build:assets` bị bỏ qua trong khi check mong đợi nó |
| Deploy từ chối với "not a git checkout" | in-place được chọn cho một thư mục không phải git checkout; sửa layout hoặc ghi rõ chiến lược |

## Ca 3: Hyvä, một theme, chế độ production {#case-3-hyva-one-theme-production-mode}

Storefront Hyvä thêm đúng một thứ mới so với case 1: một bước build Node **bên trong
thư mục Tailwind của theme**, chạy trên máy build.

**Hình dạng dự án.** Theme Hyvä tại `app/design/frontend/Acme/hyva`, với
`package.json` nằm trong `app/design/frontend/Acme/hyva/web/tailwind`.

**Cấu hình.** Thêm thư mục của theme và giữ nguyên phần còn lại:

```yaml
deploy:
  settings:
    mage_mode: production
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind
    frontend_command: npm ci && npm run build     # mặc định; ghi ra nếu bạn dựa vào nó
    static_content_locales: [en_US, fr_CA]
    magento_themes:
      Acme/hyva: [en_US, fr_CA]
```

`frontend_command` chạy **bên trong** từng thư mục, trong release, như một command
shell. Mặc định nối hai command; giá trị một command như
`npx tailwindcss -i input.css -o output.css` cũng viết y hệt.

**Cái gì chạy ở đâu.** Với `--build=server`: `build:frontend` chạy trên target — nên
target cần Node và môi trường đủ để cài `node_modules` của theme. Với
`--build=artifact`: chính bước đó chạy trong CI, còn target bỏ qua hoàn toàn. Đó là
lý do thực tế để thích artifact mode cho dự án Hyvä: **Node thuộc về nơi build
chạy**, không phải trên server production.

**Diễn tập.**

```bash
govard sandbox up --profile full --php 8.3
govard deploy plan sandbox            # build:frontend phải hiện path theme của bạn
govard deploy --remote sandbox --yes
```

`--profile full` có sẵn Node, nên bước frontend chạy được trên target đúng như một
lần build server. Nếu muốn chứng minh đường artifact thay vào đó, hãy build artifact
trên máy bạn rồi deploy nó — khi đó sandbox không bao giờ chạy Node:

```bash
govard deploy build sandbox --output /tmp/acme-artifact
govard deploy --remote sandbox --artifact-dir /tmp/acme-artifact --yes
```

**Cần chờ đợi gì.** `build:frontend` in ra output npm của chính theme. Đó là một lần
build Node như mọi lần build Node khác: nhanh khi `node_modules` đã ấm, chậm hơn khi
phải cài dependency của theme trước. `build:assets` sau đó mới là bước dài.

**Những gì hay hỏng.**

| Thông báo | Ý nghĩa |
| --- | --- |
| `npm ci` lỗi vì thiếu lockfile | thư mục theme không có `package-lock.json` được commit; `npm ci` bắt buộc phải có |
| Bước frontend thành công mà storefront trông như chưa có CSS | `frontend_dir` trỏ sai thư mục (lỗi hay gặp: ghi gốc theme thay vì `web/tailwind` của nó) |
| Deploy xanh nhưng CSS vẫn cũ | artifact được build từ revision khác, hoặc `--force` tái dùng một thư mục output cũ |

## Ca 4: Hyvä, chế độ developer {#case-4-hyva-developer-mode}

Cùng dự án đó, trỏ vào một target mà team duyệt và debug.

**Cấu hình.** Đổi đúng một dòng:

```yaml
deploy:
  settings:
    mage_mode: developer
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind
```

**Cái gì chạy ở đâu.** `build:assets` là no-op: điều kiện chặn so `developer` với
`developer` và bỏ qua cả block, dù có split hay không. Magento sau đó sinh static
file theo nhu cầu khi ứng dụng được duyệt. Mọi thứ còn lại — Composer, DI compile,
build frontend bằng Node, `cache:flush`, verify — vẫn chạy. `setup:upgrade`,
`app:config:import` và maintenance window giờ có điều kiện: probe chạy
`setup:db:status` trước, và deploy chỉ đổi code sẽ bỏ qua cả sáu task downtime mà
không bao giờ mở window.

**Diễn tập.**

```bash
govard sandbox up --profile full --php 8.3
govard deploy --remote sandbox --yes
govard deploy releases sandbox        # xác nhận cái gì đã lên live
```

**Cần chờ đợi gì.** Nhanh hơn case 3 đáng kể trên cùng dự án: lượt static bị bỏ qua.
Request đầu tiên tới một trang sau deploy chậm hơn trong lúc Magento compile những
gì nó cần. Deploy cùng một revision hai lần với `--force` và lần thứ hai sẽ cho thấy
gate: probe exit `0`, sáu task downtime báo `skipped — db up-to-date (probe exit 0)`,
và site không bao giờ mở maintenance window.

**Những gì hay hỏng.**

| Thông báo | Ý nghĩa |
| --- | --- |
| Lần đầu mở một trang mất vài giây | ứng dụng đang sinh static content; đây là mode đang hoạt động, không phải lỗi |
| `mage_mode: "developer"` không có tác dụng | giá trị bị quote theo cách làm đổi nó, hoặc bị đặt ở môi trường local thay vì dưới `deploy.settings` — kiểm tra `govard deploy plan` |
| Site mất CSS trên target *production* | `developer` bị deploy lên production; static content chưa từng được build, và production không sinh nó theo nhu cầu trừ khi `env.php` bật `static_content_on_demand_in_production` |

::: warning `developer` trên target production là cấu hình sai
Mode này không phải một công tắc hiệu năng mà bạn để nguyên vì "site vẫn chạy".
`env.php` của production thường tắt `static_content_on_demand_in_production`, nên
theme mà static file chưa từng được deploy sẽ trả `404` cho chúng.
:::

## Ca 5: Hai theme build bằng Node {#case-5-two-node-built-themes}

Hai storefront (hoặc một theme Hyvä cộng một theme tuỳ biến), mỗi cái có thư mục
Tailwind riêng. Recipe build **mọi** thư mục đã cấu hình, theo thứ tự, mỗi thư mục
trong subshell riêng; lỗi đầu tiên dừng deploy thay vì để build của theme thứ hai
che mất.

**Cấu hình.**

```yaml
deploy:
  settings:
    mage_mode: production
    frontend_dir:
      - app/design/frontend/Acme/hyva/web/tailwind
      - app/design/frontend/Acme/outlet/web/tailwind
    magento_themes:
      Acme/hyva: [en_US, fr_CA]
      Acme/outlet: [en_US, en_GB]
```

Mọi entry đều được quote, nên một entry là đúng một thư mục. Path có dấu cách
**buộc phải** là một entry trong list: dạng một path được đọc như danh sách tách
theo khoảng trắng.

**Quy tắc locale.** Locale trong map theme được **cộng vào**
`static_content_locales`, và union được deploy cho mọi theme trong map. Đây là chủ
ý, và là đặc tính của ứng dụng chứ không phải một cách đơn giản hoá: một lần gọi
`setup:static-content:deploy` chỉ resolve `--language` một lần cho cả lượt chạy, nên
một lần gọi không thể compile theme A với bộ locale này và theme B với bộ locale
khác. Ở đây cả hai storefront đều nhận `en_US`, `fr_CA` và `en_GB`. Muốn thu hẹp
theo từng theme thì phải gọi một lần cho mỗi nhóm locale; union là thứ command diễn
đạt được, và là hướng an toàn — thừa locale chỉ tốn thời gian build, thiếu locale
thì mất một storefront.

**Diễn tập.**

```bash
govard sandbox up --profile full --php 8.3
govard deploy plan sandbox            # cả hai thư mục phải xuất hiện trong build:frontend
govard deploy --remote sandbox --yes
```

**Cần chờ đợi gì.** Cả hai lần build npm đều chạy, rồi một lượt static content phủ
cả hai theme và union locale. Thời gian build tăng theo số theme, và lượt static tăng
theo theme × locale.

**Những gì hay hỏng.**

| Thông báo | Ý nghĩa |
| --- | --- |
| Chỉ theme đầu tiên được build | `frontend_dir` là một chuỗi đơn chứa hai path — nó được đọc như danh sách tách theo khoảng trắng, và chỉ list YAML mới là list |
| Một theme thiếu CSS mà deploy vẫn xanh | theme đó không nằm trong `frontend_dir` (build Node của nó chưa từng chạy) |
| Một store view trả 404 cho static file của nó | theme hoặc locale của store view đó không nằm trong tập deploy — đối chiếu map với output của `setup:static-content:deploy` |

## Ca 6: Multi-store, storefront Hyvä với admin Luma {#case-6-multi-store-hyva-storefront-with-a-luma-admin}

Một release phục vụ nhiều website, với static content của adminhtml và frontend được
deploy thành hai lượt.

**Cấu hình.** Việc tách tồn tại vì hai trường hợp mà một lượt xử lý kém: danh sách
theme chỉ có theme frontend (theme admin sẽ bị bỏ sót) và deployment lớn nơi một
tiến trình ôm mọi area sẽ hết bộ nhớ.

```yaml
deploy:
  settings:
    mage_mode: production
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind

    # Frontend: theme storefront, cho mọi locale mà các store phục vụ.
    magento_themes:
      Acme/hyva: [en_US, fr_CA, de_DE]
    static_content_locales: [en_US]

    split_static_deployment: true
    # Adminhtml: mặc định đã phủ; ghi ra nếu dự án này khác.
    magento_themes_backend: [Magento/backend]
    static_content_locales_backend: [en_US]

    worker_control: true          # cron:remove / queue:consumers:stop quanh bước migration
```

Lượt admin dùng `magento_themes_backend` (mặc định là theme admin) và
`static_content_locales_backend`, mặc định lấy theo ngôn ngữ frontend để hai lượt
khớp nhau trừ khi dự án nói khác. Lượt frontend dùng `magento_themes` và
`static_content_locales`. Lượt thứ hai được nối vào lượt thứ nhất, nên lượt admin
lỗi sẽ dừng deploy thay vì publish một nửa static content.

**Phần còn lại của pipeline đã lo sẵn cho nhiều website:**

- `app/etc/env.php` và `pub/media` được **share** giữa các release, nên cấu hình
  theo scope và media không mất khi swap;
- `app:config:import` áp cấu hình nằm trong `config.php` và `env.php` — nơi cấu hình
  theo scope thuộc về khi chúng được version hoá;
- `setup:upgrade`, flush cache và worker control là toàn cục, đúng với việc mọi
  website dùng chung một database.

**Điều govard chủ động không làm:** nó không bao giờ ghi cấu hình theo store vào
database. Base URL, cấu hình scope và mọi thứ khác mà operator đổi trong admin là
dữ liệu của ứng dụng, không phải của release, và một deploy ghi đè chúng là một
deploy có thể xoá cấu hình của storefront đang chạy.

**Diễn tập.**

```bash
govard sandbox up --profile full --php 8.3
govard deploy --remote sandbox --yes
```

**Kiểm chứng cho nhiều storefront.** `deploy.verify.url` kiểm một URL. Dự án
multi-store muốn kiểm mọi storefront thì neo một hook vào `verify`:

```yaml
deploy:
  hooks:
    - name: every-storefront
      on: "stage:verify"
      position: after
      order: 10
      optional: true
      run: |
        for url in https://shop.example.com/ https://outlet.example.com/; do
          code=$(curl -s -o /dev/null -w '%{http_code}' "$url")
          [ "$code" = "200" ] || { echo "$url returned $code"; exit 1; }
        done
```

## Ca 7: Chế độ artifact trong CI {#case-7-artifact-mode-in-ci}

Bất kỳ ca nào ở trên, với bước build được chuyển ra khỏi target. Điểm mấu chốt là
job chạm vào production chỉ cần **govard, SSH và rsync, không gì khác**.

**Cấu hình.** Không gì trong dự án thay đổi; mode đến từ các flag:

```yaml
# .gitlab-ci.yml (xem thêm /workflows/ci-integration)
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

**Cái gì chạy ở đâu.** Các task build — dependencies, patches, DI compile và bước
build frontend (Node) — chạy trong job `build`. Đó chính là năm build id trung tính
mà artifact mode thay bằng `deploy:artifact`; recipe miễn trừ những bước cần ứng
dụng bằng cách đánh dấu chúng, và với Magento phần được miễn trừ đó là static
content, nên nó ở lại trong deploy và chạy trên target sau khi artifact được bung.
`setup:upgrade`, import cấu hình, flush cache và verify cũng chạy trên target như
trước giờ.

**Artifact từ chối mang theo gì.** Những path mà recipe khai là **shared**
(`shared_files`, `shared_dirs`) bị loại trong lúc build, và `govard deploy build` in
ra từng path bị loại. Nếu không, một artifact dựng trên máy tình cờ có
`app/etc/env.php` riêng sẽ đè lên cái của target — file thường thay thế symlink mà
`deploy:shared` vừa tạo — và release sẽ chết bằng đúng lời của ứng dụng:
`Connection "default" is not defined`.

**Diễn tập ngay ở máy local.** Mô hình hai job diễn tập được mà không cần CI:

```bash
rm -rf /tmp/acme-artifact
govard deploy build sandbox --output /tmp/acme-artifact
govard deploy plan sandbox --artifact-dir /tmp/acme-artifact   # hiện nhánh artifact
govard deploy --remote sandbox --artifact-dir /tmp/acme-artifact --yes
```

**Cần chờ đợi gì.** Trên một dự án 2.4.9 thật, lần build artifact tạo ra 101.366
file / 752,8 MiB trong khoảng bảy phút, và lần deploy production mode nhận nó kết
thúc trong 14m35s (release 11), bước dài nhất là lượt static content trên target —
khoảng năm đến sáu phút trong đó, và là bước buộc phải chạy ở đó. Thư mục output đã
tồn tại sẽ bị từ chối trừ khi bạn truyền `--force`, nên một file còn sót từ lần build
trước không thể lọt ra.

**Những gì hay hỏng.**

| Thông báo | Ý nghĩa |
| --- | --- |
| `artifact manifest does not match the revision` | thư mục artifact đến từ một lần build cũ hơn |
| Lệch phiên bản PHP bị từ chối ở preflight | PHP của image CI không phải của target; artifact được build bằng interpreter sai |
| Máy build lỗi ở `setup:static-content:deploy` | nó không nên chạy bước đó — bước ấy được để lại cho target; kiểm tra recipe có đánh dấu nó và `govard deploy plan` có hiện nó trong deploy |
| Target lỗi ở `app:configure` với `Connection "default" is not defined` | artifact có mang theo một path shared; tìm nó trong các dòng "left to the target" của output build |

## Ca 8: Target in-place bạn thừa hưởng {#case-8-the-in-place-target-you-inherited}

Ca migration: một server đã phục vụ ứng dụng từ thư mục thật, được công cụ khác
deploy tới, và bạn muốn govard tiếp quản.

**Bước 1 — tìm hiểu ở đó có gì trước khi viết cấu hình.**

```bash
govard deploy check legacy-staging
```

Output nêu layout nó tìm thấy, chiến lược publish mà layout đó ngụ ý, dung lượng
trống, PHP mà target chạy, repository có tới được từ target hay không, và đường
credential Composer nào đang được dùng. Các note đến trước, theo thứ tự các phép dò đã
chạy, rồi mới tới các field đã resolve:

```
Target legacy-staging is deployable
  publish strategy: in_place
  repository reachable from the target: refs/heads/main
  free space at the deploy path: 42.4 GiB
  php on the target: 8.2.18
  host:            legacy-staging
  deploy path:     /var/www/shop
  current path:    /var/www/shop
  publish:         in_place
  layout:          current path is a real directory: releases are copied into it
```

Note nào xuất hiện còn tuỳ target và lần chạy: chiến lược symlink thêm
`atomic symlink rename: supported`, sandbox thêm việc mirror đã được refresh, artifact
mode thêm số file, revision và so sánh PHP của artifact, còn dự án có repository riêng
thêm đường credential nó tìm được. Hai note là cảnh báo chứ không phải dữ kiện —
`deploy.settings.sync_paths is empty` với target in-place, và một entry `sync_paths` mà
release link từ `shared/`.

Nếu remote bỏ trống `deploy_path`, govard dò layout mà target đã có (`~`,
`~/.deployer`) và chỉ nhận nó **khi đúng một ứng viên khớp**, đồng thời nói rõ là
cái nào. Không có layout nào, hoặc có nhiều cái, là lỗi cấu hình (exit 4) kèm danh
sách đã dò — trường hợp duy nhất phải giải quyết bằng cách đọc server, không phải
bằng cách đoán.

**Bước 2 — lock của công cụ kia.** Target đang mang lock của công cụ deploy kia sẽ
bị từ chối thay vì bị tranh nhau. Đó không phải lỗi cần lách: hai công cụ cùng ghi
vào một thư mục release chính là cách tạo ra một target deploy dở dang.

**Bước 3 — diễn tập đúng layout đó.** `sandbox reset --layout deployer` seed một
target thuộc công cụ kia, nên lần từ chối cũng diễn tập được:

```bash
govard sandbox up --profile full --php 8.3
govard sandbox reset --layout deployer --docroot absent
govard deploy --remote sandbox --yes     # mong đợi lời từ chối nêu tên lock
govard sandbox reset --docroot real
govard deploy --remote sandbox --yes     # giờ là đường in-place
```

## Ca 9: Laravel, frontend Vite, webroot symlink {#case-9-laravel}

**Ba quyết định:** build trên server (`--build=server`) — Laravel không có bước
compile nào đáng chuyển đi, và cache của framework dù sao cũng phải dựng ở target;
webroot là symlink vào `releases/` (`symlink`); chế độ target là `production` qua
`APP_ENV` trong `.env` của target.

```yaml
deploy:
  keep_releases: 5
  settings:
    shared_files: [".env"]
    shared_dirs: ["storage"]
    writable_dirs: ["storage", "bootstrap/cache"]
    sync_paths: ["vendor", "public/build"]
    frontend_dir: ["."]
    frontend_command: "npm ci && npm run build"
```

- `build:vendors` chạy `composer install --no-dev --optimize-autoloader`.
- `build:frontend` build từng `frontend_dir` (ở đây là gốc repo, nơi Vite nằm);
  vòng lặp bị bỏ qua khi `frontend_dir` rỗng, nên dự án đã commit `public/build`
  không tốn gì.
- `app:configure` chạy `artisan storage:link`, để `public/storage` có trong mọi
  release. Nó exit 0 khi link đã tồn tại, nên ca in-place vẫn an toàn.
- `db:migrate` chạy `artisan migrate --force`.
- `app:cache:flush` chạy `optimize:clear` rồi `optimize` **ở target**: `optimize`
  ghi `bootstrap/cache/config.php`, và khi file đó tồn tại thì biến môi trường
  không còn override `.env`. Cache dựng ở máy build sẽ mang cấu hình của máy đó lên
  production.
- `worker_control: true` gửi `queue:restart` trước khi migrate. Nó exit 0 với mọi
  cache store, nên store phải bền thì worker mới thật sự thấy tín hiệu.
- Check `app` chạy `artisan db:show` (Laravel 11+), lùi về `migrate:status`.
  `about --only=environment` **không** được dùng: nó exit 0 ngay cả khi không có
  database, nên chẳng chứng minh được gì.
- **Ở artifact mode** không bước nào của Laravel ở lại target: không bước nào được
  đánh dấu *cần ứng dụng*, nên artifact phải mang `vendor/` và `public/build`.
  `app:cache:flush` vẫn chạy trên target — đó là thứ giữ cho cache cấu hình không
  bị dựng ở nơi nào khác.

## Ca 10: Symfony, migration Doctrine, target PostgreSQL {#case-10-symfony}

**Ba quyết định:** build trên server; webroot symlink; chế độ target là environment
do `symfony_env` đặt tên (`prod` nếu dự án không nói khác).

```yaml
deploy:
  settings:
    shared_files: [".env.local"]
    shared_dirs: ["var/log"]
    writable_dirs: ["var"]
    sync_paths: ["vendor", "public/bundles"]
    symfony_env: prod
  # Dự án dùng database không phải mặc định của recipe thì khai ở đây, và sandbox
  # sẽ cấp nó. Thay thế, không nối thêm: một database, không phải hai.
  #   sandbox_packages: [postgresql]
  #   sandbox_extensions: [intl, pgsql, mbstring, xml, curl, zip]
  #   sandbox_services: [postgresql, redis-server]
```

- `build:vendors` truyền `--no-scripts`. `auto-scripts` của Composer chạy
  `cache:clear` và `assets:install`, cả hai đều thuộc về target: cache hâm nóng cho
  máy build là vô giá trị, và link asset resolve theo cây vendor thật sự có mặt.
- `build:assets` chạy `bin/console assets:install public --symlink --relative` — ở
  target, vì `public/bundles` bị gitignore và link tương đối phải resolve được từ
  bất kỳ độ sâu nào của docroot. Đó cũng là lý do `public/bundles` nằm trong
  `sync_paths` cho docroot in-place.
- `db:migrate` truyền `--allow-no-migration`: thư mục `migrations/` rỗng là dự án
  khoẻ mạnh, và không có flag đó thì Doctrine exit khác 0.
- `app:cache:flush` clear rồi warmup container cho `symfony_env`.
- **Không có maintenance window.** Symfony không có cơ chế gốc, nên
  `maintenance:enable`/`disable` bị skip và `db:migrate` chạy trên site đang sống.
  Dự án cần thì thêm hook — trang deployment có mẫu.
- Check `app` chạy `dbal:run-sql "SELECT 1"`, hoặc `doctrine:query:sql` với
  DoctrineBundle cũ; nhánh được chọn bằng cách hỏi console.
- **Ở artifact mode** `build:assets` vẫn chạy trên target — `assets:install` đọc
  ứng dụng đã cài, nên artifact không thay thế được bước này. Artifact mang `vendor/`
  và output build frontend; `public/bundles` được ghi trên target, đó cũng là lý do
  nó nằm trong `sync_paths` cho docroot in-place.

## Ca 11: WordPress, layout classic, wp-cli trên target {#case-11-wordpress}

**Ba quyết định:** build trên server; webroot symlink; profile sandbox `full`, vì
`db:migrate`, cache flush và check của recipe đều chạm database.

```yaml
deploy:
  settings:
    shared_files: ["wp-config.php"]
    shared_dirs: ["wp-content/uploads"]
    writable_dirs: ["wp-content/uploads", "wp-content/cache", "wp-content/upgrade", "wp-content/languages"]
```

Chỉ layout classic: file core và `wp-content/` ở gốc repo, không có `composer.json`.
Layout Bedrock (core trong `vendor/`, docroot `web/`) và checkout chỉ có content
không được hỗ trợ.

- **Seed `shared/wp-config.php` trước lần deploy đầu.** `deploy:shared` chỉ link một
  shared entry khi nó đã tồn tại, nên `shared/` chưa seed sẽ để release đầu tiên giữ
  `wp-config.php` của repo — bản trỏ vào database phát triển. Check `app` sau đó
  fail vì không kết nối được database, đúng và rõ, và đó là tín hiệu để seed.
- `build:vendors` chỉ chạy `composer install` khi có `composer.json`.
- `db:migrate` là `wp core update-db` với wp-cli, hoặc bootstrap `wp-load.php` gọi
  `wp_upgrade()` khi không có. `app:cache:flush` cùng hình dạng (`wp cache flush`
  và `wp rewrite flush --hard`, hoặc tương đương bằng PHP).
- Maintenance ghi `.maintenance` và drop-in `wp-content/maintenance.php` vào đường
  dẫn được serve. Timestamp được ghi **vượt đồng hồ** (`time() + 86400`), vì
  WordPress coi cờ cũ hơn mười phút là hết hạn: với window dài hơn, site sẽ lặng lẽ
  sống lại giữa lúc migrate. Drop-in mang marker, nên trang maintenance dự án tự
  ship vẫn được giữ.
- `--db-backup` dùng `wp db export`, và `rollback --with-db` restore bằng
  `wp db import`; cả hai đọc kết nối từ `wp-config.php`. Sandbox tự yêu cầu wp-cli
  và `default-mysql-client` (`mysqldump`).
- Laravel và Symfony **không có dump command**: bật `--db-backup` cho chúng sẽ fail
  kèm thông báo nêu rõ lý do, thay vì lặng lẽ không có backup nào.
- **Ở artifact mode** bước build duy nhất là `composer install` có guard, nên
  artifact mang `vendor/` với dự án có `composer.json` — và không mang gì khác.
  `wp core update-db`, cache flush và check đều chạy trên target, đối diện database,
  ở mọi mode.

## Diễn tập bất kỳ ca nào trong sandbox {#rehearsing-any-case-in-the-sandbox}

`govard sandbox` cho dự án một đích triển khai thật ngay trên máy này: một
container đóng vai remote, kết nối qua SSH thật và rsync thật, với đúng pipeline mà
một lần deploy production chạy. Không phần nào trong pipeline biết sự khác biệt, và
đó là thứ khiến nó là diễn tập thật chứ không phải mô phỏng.

```bash
govard sandbox up [--profile basic|php|full] [--php 8.3] [--docroot absent|symlink|real]
govard sandbox status
govard sandbox ssh
govard sandbox reset [--docroot …] [--layout deployer]
govard sandbox down [--purge]
govard deploy --remote sandbox --yes
```

Vì `sandbox` là lệnh top-level chứ không phải subcommand của deploy, lần deploy
phải dùng dạng flag: `govard deploy --remote sandbox --yes`.

Khi `govard svc up` đã khởi động SSH gateway dùng chung (xem
[Deployment](/workflows/deployment#the-shared-ssh-gateway)), cùng sandbox đó
cũng truy cập được tại `ssh -p 2222 <project-name>@127.0.0.1` -- tiện để trỏ
remote interpreter của IDE hay SFTP deployment target vào một địa chỉ ổn
định thay vì đuổi theo cổng tạm mà mỗi lần `sandbox up` mới chọn.

### Profile nào chứng minh được điều gì

| Profile | Chứa gì | Một lần diễn tập với nó chứng minh được gì |
| --- | --- | --- |
| `basic` | sshd, rsync, git | chính pipeline: layout release, cú swap symlink, lock, `releases`/`status`/`rollback`. Không PHP, Composer hay Node — dự án Magento dừng ở `build:vendors`. |
| `php` (mặc định) | cộng thêm php-cli, composer, node, và web tier | mọi thứ `basic` chứng minh được, cộng thêm command line của recipe, bước cài dependency và build frontend bằng Node, và **nửa HTTP của `verify`** với web tier. Không có database hay cache, nên không gì đọc store chạy được. |
| `full` | cộng thêm database (MariaDB) và cache (Redis/Valkey) | toàn bộ pipeline Magento, kể cả `setup:upgrade`, `app:config:import` và `cache:flush`. Đây là profile mà một lần diễn tập Magento thật cần. |

Recipe cũng đóng góp những gì framework cần ngoài profile: với Magento là
`libxslt1-dev`, `libzip-dev`, `libpng-dev`, `libjpeg-dev`, `libfreetype6-dev`,
`default-mysql-client`, các PHP extension `bcmath curl gd intl mysql soap sockets xsl
zip`, và hai service. Danh sách đó là của ứng dụng, không phải của govard: dự án cần
thêm một extension thì thêm vào recipe, không phải vào flag.

Profile `php` và `full` còn ship **web tier**: nginx phục vụ served path cộng
`stack.web_root` của dự án (`/pub` với Magento) và PHP-FPM chạy bằng chính user
deploy, nên ứng dụng ghi được những thư mục mà `deploy:writable` giao cho. `up`
publish cổng đó trên loopback và trỏ `deploy.verify.url` của remote sandbox vào nó,
nghĩa là deploy vào sandbox diễn tập toàn bộ pipeline, kể cả bước kiểm tra HTTP —
bước mà một target không có web server không bao giờ chạy được. `basic` không ship
web tier và không quảng cáo verify URL.

::: warning `stack.web_root` là một phần của image sandbox
`root` của nginx là `<current><web_root>`, và web root được nướng vào image vì tag
của image là hash của definition đã render. Dự án có `stack.web_root` sai sẽ nhận
một sandbox phục vụ sai thư mục — và cách sửa là `govard sandbox up
--recreate`, không phải sửa container bằng tay.
:::

### Định hình docroot

`--docroot` là cách bạn chọn chiến lược publish mà lần diễn tập sẽ kiểm chứng:

| `--docroot` | Trạng thái target | Chiến lược resolve ra | Ca nó diễn tập |
| --- | --- | --- | --- |
| `absent` | served path không tồn tại | `symlink` | lần deploy đầu tiên |
| `symlink` (mặc định) | symlink gãy trỏ vào `releases/` | `symlink` | mọi lần deploy sau lần đầu |
| `real` | thư mục thật, được seed thành git checkout của mirror | `in_place` | case 2 và 8 |

Việc định hình xảy ra khi target được tạo và khi bạn nói rõ hình dạng muốn có;
`reset` thì luôn định hình. Một sandbox đã tồn tại được mô tả bằng chính nó, không
bằng flag của lệnh vừa gọi tới: `up` báo đúng profile và series PHP mà container
được build, và đòi một profile hoặc series khác sẽ bị từ chối kèm đúng flag thay đổi
được nó (`--recreate`), thay vì âm thầm dán nhãn mới cho container.

### Provision ứng dụng bên trong một sandbox mới

Một sandbox hoàn toàn mới được seed từ môi trường gốc đang chạy ngay lúc `up`:
dump database dạng logical (môi trường gốc vẫn chạy), cây media, và file env được
viết lại cho sandbox (base_url thành URL web của sandbox). Không còn dựng tay:
ba bước thủ công dưới đây thuộc về thời trước seed và chỉ còn đúng với sandbox
`--no-seed`.

```bash
govard sandbox up --profile full   # tự seed DB + media + env.php
govard sandbox up --profile full --no-seed  # cố tình để trắng
# bên trong container --no-seed, với user deploy:
#   viết ~/.deployer/shared/app/etc/env.php
#   import dump database
```

Hai prerequisite vẫn làm tay vì không snapshot nào bịa ra được chúng:

1. **môi trường gốc phải đang chạy** lúc `up` seed — nếu không `up` từ chối và nói
   rõ (hoặc truyền `--no-seed` cho sandbox trắng);
2. **một search engine được hỗ trợ** nếu dự án dùng. `setup:upgrade` từ chối thẳng
   fallback MySQL của Magento, và search cluster thường chạy cạnh sandbox (một
   container `sandbox-opensearch` riêng), không phải bên trong nó.

Reseed là `up --recreate`: sandbox đã có giữ nguyên data, và thay đổi ở gốc sau
lúc seed không bao giờ tự lan sang.

### Credential bên trong sandbox

Có ba đường Composer và chúng không thay thế được cho nhau: `COMPOSER_AUTH` từ môi
trường deploy được chuyển tiếp qua standard input và **đè lên mọi file trên target**;
`auth.json` được commit trong dự án được `deploy:code` materialise vào release;
`shared/auth.json` trên target chỉ được đọc khi dự án liệt kê `auth.json` trong
`shared_files`.

Điều quan trọng với một sandbox là nó là một **target hoàn toàn mới**: package kiểu
`git` cần khoá và một dòng `known_hosts` *bên trong container*, và credential chỉ
nằm trong shell profile của bạn chính là loại có thể đè lên `auth.json` đang chạy
tốt của dự án rồi làm hỏng build, ở chỗ mà một lệnh `govard deploy` thường sẽ thành
công. Hãy đặt credential ở nơi target dùng được (`docker exec`, hoặc một file được
mount) rồi chạy lại — bước hỏng sẽ đi tiếp từ release directory sạch và Composer
cache được giữ nguyên.

### Đọc một lần diễn tập thất bại

| Lần diễn tập báo | Đó là gì |
| --- | --- |
| `the sandbox container behind remote "sandbox" is not running` | container đã dừng hoặc chưa từng được tạo; `govard sandbox up` |
| `the sandbox mirror … is missing` | git mirror ở máy local đã bị xoá; `up` tạo lại nó |
| `deploy path … is not writable` | `owner`/`writable_mode` của profile bị ghi đè do sửa tay; `up` resolve lại remote từ trạng thái live của container |
| `verify http: http://127.0.0.1:PORT/ returned HTTP 403` | web tier đã lên nhưng ứng dụng chưa được cài — prerequisite, không phải lỗi |
| `build:vendors` lỗi với `composer: not found` | profile `basic` không có toolchain PHP; dùng `php` hoặc `full` |
| `The default website isn't defined` | target không có cấu hình store trong database |

### Một script diễn tập đầu-cuối

Toàn bộ vòng lặp cho một ca, theo thứ tự, với những phần quan trọng được nêu rõ:

```bash
# 1. Dựng target khớp với dự án (case 3/5/6 → full).
govard sandbox up --profile full --php 8.3 --docroot symlink

# 2. Đọc target ngụ ý gì trước khi chạy bất cứ thứ gì. Đây là chỗ một chiến lược
#    publish sai hay một series PHP thiếu lộ ra, trong vài giây.
govard deploy check sandbox

# 3. In pipeline đã resolve — từng bước, nguồn của nó và nó chạy ở đâu.
govard deploy plan sandbox

# 4. Provision ứng dụng (env.php, database, search) — xem ở trên.

# 5. Deploy commit bạn chưa push. Mirror được refresh sẵn cho bạn.
govard deploy --remote sandbox --yes --verbose

# 6. Xác nhận cái gì đang live, rồi diễn tập đường phục hồi.
govard deploy releases sandbox
govard deploy rollback sandbox --yes

# 7. Phá nó có chủ ý: Ctrl-C trong lúc upload. Mong đợi "the run was
#    interrupted", lock được nhả, và không còn rsync nào chạy ở cả hai phía.

# 8. Dọn dẹp. `--purge` xoá luôn image, khoá và mirror.
govard sandbox down --purge
```

## Tham chiếu: mọi setting mà recipe framework đọc {#reference-every-setting-the-framework-recipes-read}

Setting ở tầng engine (do recipe mặc định khai, core áp dụng):

| Setting | Mặc định | Việc nó làm |
| --- | --- | --- |
| `shared_files` | `app/etc/env.php`, `var/.maintenance.ip` | file được link từ `shared/` vào mọi release |
| `shared_dirs` | `var/log`, `var/report`, `var/session`, `var/backups`, `var/tmp`, `pub/media`, `pub/sitemap`, `pub/static/_cache` | thư mục được link từ `shared/` |
| `writable_dirs` | `var`, `pub/static`, `pub/media`, `generated`, `app/etc` | path được cấp quyền ghi trong release |
| `writable_mode` | `chmod` | `chmod`, `chown`, `chmod+chown`, `acl`, `skip` |
| `writable_permissions` | `0775` | mode mà `chmod` dùng |
| `owner` | (rỗng; `chown`/`chmod+chown`/`acl` bắt buộc có) | `user` hoặc `user:group` |
| `sync_paths` | `vendor`, `generated`, `pub/static/adminhtml`, `pub/static/frontend` | path được copy vào docroot **in-place** |
| `php_bin` | `php` | interpreter PHP trên target |
| `php_version` | (rỗng; không chặn khi bỏ trống) | series mà target chạy — một cổng chặn, không phải sở thích |
| `composer_bin` | `composer` | binary Composer trên target |
| `content_version` | revision rút gọn | `--content-version`; mang tính xác định, nên lần thử lại ghi ra đúng các asset URL như cũ |

Setting của Magento (do recipe Magento khai):

| Setting | Mặc định | Việc nó làm |
| --- | --- | --- |
| `mage_mode` | (rỗng → hành vi production) | `production` deploy static content; `developer` bỏ qua nó |
| `frontend_dir` | (rỗng → bỏ qua) | một path hoặc một list: các thư mục Tailwind cần build |
| `frontend_command` | `npm ci && npm run build` | command chạy bên trong từng `frontend_dir` |
| `static_jobs` | `4` | `-j` cho `setup:static-content:deploy` |
| `static_content_locales` | (rỗng → locale mặc định của Magento) | `--language`, một hoặc nhiều |
| `magento_themes` | (rỗng → mọi theme) | `-t`; một list theme hoặc map theme → locale |
| `split_static_deployment` | `false` | deploy adminhtml và frontend thành hai lượt |
| `magento_themes_backend` | `Magento/backend` | `-t` cho lượt adminhtml |
| `static_content_locales_backend` | locale của frontend | `--language` cho lượt adminhtml |
| `static_deploy_options` | (rỗng) | cờ thêm cho **mọi** lượt (`--no-parent`, `-s standard`, …) |
| `worker_control` | `false` | `cron:remove` / `queue:consumers:stop` quanh bước migration, khôi phục lại sau đó |
| `runtime_reload_command` | (rỗng) | chạy như phần cuối của bước flush cache (reset opcache, reload FPM) |

`deploy.settings` được đối chiếu với recipe trước khi chạy bất cứ thứ gì: key mà
recipe không biết, hoặc giá trị sai dạng, là lỗi cấu hình (exit 4) có nêu tên key và
gợi ý key gần đúng. Giá trị chuỗi phải được quote nếu trông giống số
(`php_version: "8.2"`) — engine đọc các setting này dưới dạng chuỗi, nên `8.2` không
quote sẽ đọc thành rỗng.

Setting của Laravel, Symfony và WordPress (do recipe của chúng khai):

| Setting | Framework | Mặc định | Tác dụng |
| --- | --- | --- | --- |
| `frontend_dir` | cả ba | (rỗng → skip) | một path hoặc một list: các thư mục có frontend asset được build |
| `frontend_command` | cả ba | `npm ci && npm run build` | command chạy trong mỗi `frontend_dir` |
| `runtime_reload_command` | Laravel, Symfony, WordPress | (rỗng) | chạy ở phần cuối của bước cache |
| `worker_control` | Laravel, Symfony | `false` | `queue:restart` / `messenger:stop-workers` trước khi migrate |
| `symfony_env` | Symfony | `prod` | environment console mà deploy chạy dưới; **không được validate** |

Sandbox cấp gì cho một dự án, ngoài mặc định của recipe — mỗi danh sách **thay thế**
danh sách của recipe, và chỉ được đọc ở tầng project:

| Setting | Mặc định | Tác dụng |
| --- | --- | --- |
| `sandbox_packages` | theo recipe | apt package bổ sung mà image sandbox cài |
| `sandbox_extensions` | theo recipe | PHP extension, cài dưới dạng `php<series>-<name>` |
| `sandbox_services` | theo recipe | init service start trước sshd; tên thuộc bảng của engine mang theo package của nó |
| `sandbox_tools` | theo recipe | binary từ danh sách đóng của engine (`wp-cli`) |

## Checklist trước lần deploy đầu tiên lên target thật

1. **Target đã chạy ứng dụng.** Server chưa từng phục vụ nó thì không nhận được
   release: không có `shared/app/etc/env.php`, không database, không search engine
   nghĩa là không có lần deploy đầu tiên.
2. **`govard deploy check <remote>` xanh** và nêu đúng chiến lược publish bạn mong
   đợi. Bất ngờ ở đây rẻ hơn nhiều so với bất ngờ trong `publish`.
3. **`govard deploy plan <remote>` hiện đúng ca bạn đã cấu hình**: đúng nhánh build,
   đúng `frontend_dir`, bước static content chạy trên target ở artifact mode, và
   `mage_mode` khớp môi trường.
4. **Cùng dự án đó đã được diễn tập trong sandbox** với profile tương đương và hình
   dạng `--docroot` khớp.
5. **Credential nằm đúng nơi bước build sẽ tìm**, và bạn biết đang dùng đường nào
   trong ba đường Composer.
6. **`deploy.verify.url` đã được đặt** nếu bạn muốn deploy chứng minh ứng dụng trả
   lời được, chứ không chỉ là file đã nằm đúng chỗ.
7. **`--db-backup` được bật** cho lần deploy đầu tiên có migrate database production;
   rollback mà không có dump thì khôi phục được code nhưng không khôi phục được dữ
   liệu.
