---
title: Thêm Framework mới vào Govard
description: Cấu trúc nội bộ của framework registry trong Govard, và hướng dẫn từng file cụ thể để thêm hỗ trợ cho một framework mới.
---

# Thêm Framework mới

Govard hiện hỗ trợ 13 framework: Magento 2, Mage-OS, Magento 1, OpenMage, Laravel, Symfony, Drupal, WordPress, Next.js, Emdash, Shopware, CakePHP, và PrestaShop. Trang này mô tả cấu trúc nội bộ của phần hỗ trợ đó, và những gì cần đụng vào để thêm framework thứ 14.

---

## Registry: `internal/frameworks`

Mỗi framework có một package nhỏ tại `internal/frameworks/<name>/` tạo ra một `types.FrameworkDefinition` — một struct duy nhất, chính là "chứng minh thư" của framework đó bên trong Govard:

```go
// internal/frameworks/types/definition.go
type FrameworkDefinition struct {
    Name        string   // key chuẩn hóa, vd "magento2"
    Aliases     []string // vd "magento" -> "magento2"
    DisplayName string   // nhãn hiển thị cho người dùng, vd "Magento 2"

    Config   engine.FrameworkConfig         // phiên bản PHP/Node, nginx template, DB mặc định...
    Manifest engine.FrameworkManifestConfig // exclude khi sync, bảng nhạy cảm, feature flags
    Detect   engine.DetectionSpec           // chữ ký nhận diện composer/package.json/auth.json/đường dẫn file

    Bootstrap      BootstrapFactory              // func(bootstrap.Options) bootstrap.FrameworkBootstrap
    BaseURLManager func() tunnel.BaseURLManager  // nil nếu framework không cần rewrite base-URL cho tunnel

    SupportsBootstrap    bool // cho phép `govard bootstrap` (quy trình remote/clone)
    SupportsFreshInstall bool // cho phép `govard bootstrap --fresh`
}
```

`init()` của `internal/frameworks/all.go` gọi `Register(<pkg>.Definition())` cho cả 13 package, theo một thứ tự cụ thể (lý do ở phần dưới), tạo nên một registry cấp package mà phần còn lại của Govard đọc qua 3 file nhỏ, tập trung:

| File | Vai trò |
| :--- | :--- |
| `internal/frameworks/registry.go` | `Get(name)`, `All()`, `Normalize(name)` — bản thân registry, xử lý alias |
| `internal/frameworks/run.go` | `RunBootstrap(name, opts)` — dispatch tới `def.Bootstrap` thay vì switch |
| `internal/frameworks/base_url.go` | `NewBaseURLManager(name)` — dispatch tới `def.BaseURLManager`, fallback về `tunnel.NoopManager` |

Mọi nơi đọc dữ liệu framework theo tên — allowlist của `govard bootstrap`, base-URL rewriting của `govard tunnel`, bootstrap dispatcher — đều đi qua 1 trong 3 file này thay vì `switch framework { case "magento2": ... }` hardcode rải rác. Thêm framework vào registry nghĩa là nó tự động tham gia cả 3 nơi đó, không cần sửa switch nào.

### Những gì *chưa* nằm trên registry

Có 2 phần vẫn là switch theo tên framework, một cách có chủ đích, không phải do bỏ sót:

1. **`internal/cmd/bootstrap_fresh_install.go` / `bootstrap_remote.go`** — phần *orchestration* thực sự của fresh-install/clone (lệnh shell nào chạy, theo thứ tự nào, điền field nào của `bootstrap.Options`). 6 framework (Symfony, Laravel, Drupal, WordPress, Shopware, CakePHP) dùng chung một chuỗi `CreateProject → Install → govard config auto` qua một bảng tra nhỏ (`genericFreshInstallFrameworks` trong `bootstrap_fresh_install.go`) — nhưng OpenMage, Next.js, Emdash, và nhóm Magento mỗi cái làm khác nhau về bản chất (git clone và `composer create-project` và tải HTTP; tạo admin user; sinh `.env`; sample data), không chỉ là "gọi constructor khác." Không thể data-hóa việc này nếu không ép các framework vào một khuôn không hợp, hoặc tái cấu trúc quan hệ phụ thuộc giữa `internal/cmd` và `internal/frameworks`. Xem mục "Orchestration fresh-install" bên dưới để biết framework mới cần gì ở đây.
2. **`engine.GetFrameworkConfig` / `engine.GetFrameworkManifestConfig`** (`internal/engine/framework_config.go`, `internal/engine/framework_manifest.json`) — dữ liệu mặc định phiên bản PHP/Node/DB thực sự. Field `Config`/`Manifest` của registry chỉ là đọc-lại (read-through) từ đây — `internal/engine` vẫn là nguồn dữ liệu gốc; `internal/frameworks` chỉ gộp nó vào 1 struct/framework cùng với detection và dispatch.

---

## Thêm framework #14: một checklist

Giả sử bạn đang thêm một framework hư cấu tên `whimsy`. Mỗi bước dưới đây đều có một ví dụ thật, đang chạy trong codebase — các đường dẫn file trỏ tới ví dụ gần giống nhất để copy.

### 1. Cấu hình runtime mặc định — `internal/engine/framework_config.go`

Thêm một entry `FrameworkConfig` (phiên bản PHP/Node, nginx template, engine/phiên bản DB, danh sách includes). Copy theo framework gần giống nhất — vd `"cakephp"` cho stack PHP+MariaDB thông thường, `"nextjs"` cho stack chỉ Node không DB.

### 2. Dữ liệu manifest — `internal/engine/framework_manifest.json`

Thêm entry dưới `frameworks.whimsy`: exclude khi sync (đường dẫn `local_media`/`remote_media`, web-root candidates), bảng DB nhạy cảm/bỏ qua, và khối `features`:

```json
"whimsy": {
  "paths": { "local_media": "public/uploads", "remote_media": "public/uploads", "web_root_candidates": [] },
  "features": {
    "requires_running_env_for_fresh_install": false,
    "supports_post_clone": true
  }
}
```

`requires_running_env_for_fresh_install` quyết định `govard bootstrap --fresh` khởi động container *trước* hay *sau* khi chạy `CreateProject` — xem mục "gotcha" bên dưới về vấn đề này trước khi đặt nó `true`.

### 3. Blueprint compose — `internal/blueprints/files/whimsy/`

Một file `services.yml` (đoạn Docker Compose) được render qua Go template — copy theo ví dụ gần nhất (`internal/blueprints/files/nextjs/services.yml` cho runtime Node, `internal/blueprints/files/cakephp/` cho PHP). Không phải framework nào cũng cần thư mục riêng: Mage-OS tái dùng thẳng blueprint compose/nginx/Varnish của Magento 2 (xem `varnishTemplateFramework` trong `internal/engine/render.go`) vì nó là bản fork drop-in với cùng hình dạng runtime.

### 4. Triển khai Bootstrap — `internal/engine/bootstrap/whimsy.go`

Triển khai interface `FrameworkBootstrap` (`internal/engine/bootstrap/base.go`):

```go
type FrameworkBootstrap interface {
    Name() string
    SupportsFreshInstall() bool
    SupportsClone() bool
    FreshCommands() []string        // tóm tắt dễ đọc, không nhất thiết là lệnh thực sự chạy
    CreateProject(projectDir string) error
    Install(projectDir string) error
    Configure(projectDir string) error
    PostClone(projectDir string) error
}
```

Copy `internal/engine/bootstrap/cakephp.go` cho framework PHP dùng chung helper `runStagedCreateProject` (`internal/engine/bootstrap/staged_project.go`), hoặc `internal/engine/bootstrap/emdash.go` cho framework mà `CreateProject` không cần container nào cả (chỉ tải HTTP thuần).

**Nếu framework cần chạy một CLI tool (`npx`, `composer`, v.v.) để scaffold dự án, nó phải chạy bên trong container — không bao giờ được giả định là công cụ đã có sẵn trên host.** Xem mục "gotcha" về container execution bên dưới; đây là bài học quan trọng nhất rút ra từ lịch sử hệ thống này.

### 5. Gắn vào registry — `internal/frameworks/whimsy/whimsy.go`

```go
package whimsy

import (
    "govard/internal/engine"
    "govard/internal/engine/bootstrap"
    "govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
    config, _ := engine.GetFrameworkConfig("whimsy")
    manifest, _ := engine.GetFrameworkManifestConfig("whimsy")
    return types.FrameworkDefinition{
        Name:        "whimsy",
        DisplayName: "Whimsy",
        Config:      config,
        Manifest:    manifest,
        Detect: engine.DetectionSpec{
            ComposerPackages: []string{"whimsy/framework"}, // hoặc PackageJSONDeps, AuthJSONHosts, FilePaths
        },
        Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
            return bootstrap.NewWhimsyBootstrap(opts)
        },
        SupportsFreshInstall: true,
        SupportsBootstrap:    true, // chỉ nếu nó cũng hỗ trợ quy trình remote/clone
    }
}
```

Chỉ đặt `BaseURLManager` nếu framework cần rewrite base-URL riêng cho `govard tunnel` (đa số không cần — `tunnel.NoopManager` mặc định là no-op, đúng cho bất kỳ framework nào không tự lưu base URL trong database hay file config).

### 6. Đăng ký — `internal/frameworks/all.go`

Thêm import và một dòng `Register(whimsy.Definition())` bên trong `init()`. **Vị trí có ý nghĩa**: detection duyệt qua các framework đã đăng ký theo đúng thứ tự đăng ký và trả về kết quả khớp đầu tiên, nên nếu chữ ký nhận diện của framework mới có thể trùng với framework khác, nó cần được đăng ký ở đúng vị trí tương đối. Comment sẵn có trong `all.go` ghi lại 1 trường hợp đã biết (Emdash trước Next.js, giữ đúng thứ tự ưu tiên phân giải xung đột của detector cũ).

### 7. Orchestration fresh-install / clone — `internal/cmd`

Đây là phần vẫn là switch (xem "Những gì chưa nằm trên registry" ở trên):

- Nếu `whimsy` khớp khuôn chung `CreateProject → Install → govard config auto`, chỉ cần thêm 1 dòng vào `genericFreshInstallFrameworks` trong `bootstrap_fresh_install.go` (một `map[string]struct{ needsDB, needsDomain bool }`) thay vì viết cả 1 hàm.
- Nếu cần các bước riêng, thêm `case "whimsy":` vào switch của `runBootstrapFrameworkFreshInstall` gọi một hàm mới `runBootstrapWhimsyFreshInstall` — copy `runBootstrapOpenMageFreshInstall` hoặc `runBootstrapNextJSFreshInstall` làm khung ban đầu.
- Nếu nó hỗ trợ quy trình remote/clone (`SupportsBootstrap: true`, không chỉ fresh-install), nó tự động được `bootstrap_remote.go`'s post-clone dispatch (`bootstrapPostCloneDefinition`) nhận diện — không cần sửa switch nào ở đó trừ khi nó thuộc nhóm Magento (được xử lý ở một nhánh riêng, sớm hơn, dựa trên `engine.IsMagento2Family`).

### 8. Docs

Thêm dòng vào bảng support/runtime-defaults và một mục ngắn trong [`docs/reference/frameworks.md`](/reference/frameworks) (và bản tiếng Việt, `docs/vi/reference/frameworks.md`).

### 9. Test

- `tests/framework_detection_test.go` — một `TestWhimsyDiscovery` khớp với chữ ký `Detect` bạn đã dùng.
- `tests/framework_definitions_test.go` hoặc một file test riêng cho `whimsy` — assert `Definition()`'s `Config`/`Manifest`/`Bootstrap` được điền đúng như kỳ vọng.
- `tests/framework_snapshot_test.go` — đây là lưới an toàn golden-snapshot bao phủ *mọi* framework đã đăng ký tự động (render blueprint, `FreshCommands()`, config/profile đã resolve, DB credentials/manifest mặc định) qua `allFrameworkNames`; đăng ký `whimsy` trong `all.go` khiến nó bắt đầu chạy, nhưng fixture golden của nó tại `tests/testdata/framework_snapshots/whimsy/` chưa tồn tại. Sinh chúng khi bạn chắc chắn output render đã đúng:

  ```bash
  UPDATE_GOLDEN=1 go test ./tests/... -run TestFrameworkSnapshot
  ```

  Luôn xem lại diff của fixture vừa sinh trước khi commit — `UPDATE_GOLDEN=1` ghi ra bất kể code hiện đang sinh ra gì, đúng hay sai.

### 10. Validate thật

Unit test chỉ kiểm tra render/dispatch cho ra output *như kỳ vọng* — chúng không bắt được việc container không thể ra internet, image thiếu 1 binary, hay race condition giữa lúc container khởi động và lúc nó thực sự sẵn sàng phục vụ traffic. Trước khi coi framework đã xong, hãy chạy thật:

```bash
mkdir -p /tmp/whimsy-test && cd /tmp/whimsy-test
govard bootstrap --framework whimsy --fresh --yes
curl -sk -o /dev/null -w '%{http_code}\n' https://whimsy-test.test/   # kỳ vọng 200, không phải lỗi docker/proxy
govard env down
```

Đây không phải thủ tục hình thức — mọi bug thật tìm được trong lúc xây dựng registry này (bước auto-configuration của Mage-OS âm thầm dùng DB credentials của Magento 2; `CreateProject` của Next.js phụ thuộc vào npm install trên host; race condition đăng ký proxy với container chưa sẵn sàng) đều vô hình với `go test ./...` và chỉ lộ ra khi thực sự chạy lệnh và kiểm tra kết quả.

---

## Những "gotcha" học được theo cách khó

### Chạy trong container, không phải trên host

Mọi `FrameworkBootstrap.CreateProject`/`Install` nào shell ra một CLI tool (`composer`, `npx`, `npm`) phải chạy tool đó **bên trong container**, không bao giờ qua `exec.Command` trần trên host. Các framework PHP làm điều này qua `bootstrap.Options.Runner` (một closure `func(command string) error` mà `internal/cmd` gắn với `runPHPContainerShellCommand`, exec vào container PHP đang chạy sẵn). Next.js ban đầu chạy `npx create-next-app` thẳng trên host — nghĩa là fresh-install của nó phụ thuộc vào bất kỳ npm/node nào đang cài (và cấu hình) trên *máy của dev*, hoàn toàn ngoài tầm kiểm soát của Govard. Trên một máy thật, một setting `~/.npmrc` toàn cục "lạc trôi" đã làm hỏng âm thầm mọi lần fresh-install Next.js (container khởi động lên, `govard bootstrap` báo thành công, nhưng app chưa bao giờ thực sự được cài — `next: not found` khi chạy).

Fix (`nodeCreateProjectRunner` trong `internal/cmd/bootstrap.go`) chạy lệnh scaffold trong một container `docker run --rm -v <projectDir>:/app node:<version> ...` dùng 1 lần rồi bỏ — độc lập với cả môi trường host lẫn việc có service nào do compose quản lý đang chạy hay không.

### Đừng giả định service compose đã sẵn sàng

Một lựa chọn hấp dẫn thay cho cách container-dùng-1-lần ở trên là exec thẳng vào container service "web" đang chạy dài hạn của framework đó (giống pattern PHP). Cách này *chỉ* work nếu container đó đã chạy sẵn vào lúc `CreateProject` thực thi — mà với đa số framework thì chưa: `govard bootstrap --fresh` chạy fresh-install *trước* `env up` cho bất kỳ framework nào không đặt `requires_running_env_for_fresh_install: true` trong manifest. Lật cờ đó để buộc env-up chạy trước lại tạo ra vấn đề tinh vi hơn: container bắt đầu chạy lệnh dài hạn bình thường của nó (vd `npm run dev`) trong khi thư mục dự án vẫn còn *rỗng*, nên nó thoát ngay lập tức — và bước đăng ký domain/proxy của pipeline bootstrap chạy đúng trong khoảng thời gian đó, đăng ký route tới một container chưa thực sự phục vụ gì. Việc đăng ký âm thầm "thành công", nhưng reverse proxy không bao giờ có backend hoạt động, và request `https://<project>.test/` đầu tiên trả về 502 cho tới khi chạy tay `env down && env up`.

Nếu một framework thực sự cần container service dài hạn của nó chạy trước khi `CreateProject` có thể exec vào, lệnh khởi động của container đó cần chịu được thư mục dự án rỗng/chưa đầy đủ (chờ vòng lặp cho tới khi có file đánh dấu, hoặc tương tự) *và* thời điểm đăng ký domain cần xảy ra sau khi app thực sự đang phục vụ — không chỉ sau khi tiến trình container khởi động. Emdash tránh hoàn toàn vấn đề này: `CreateProject` của nó không cần container nào (chỉ tải tarball qua HTTP thuần), và lệnh compose của nó đã có sẵn cơ chế "cài nếu thiếu `node_modules`" để phòng thủ, nhưng chưa bao giờ cần "chờ file xuất hiện", vì tới lúc container khởi động thì file đã có sẵn rồi.

### Thuộc cùng "họ" không có nghĩa tự động giống hệt

Mage-OS là bản fork drop-in của Magento 2 và tái dùng phần lớn hành vi runtime của nó, nhưng "tái dùng phần lớn" không phải "tái dùng toàn bộ" — DB credentials mặc định, ngưỡng phiên bản chọn search engine, và exec user đều là những điểm quyết định riêng, mỗi cái cần một check tường minh. `engine.IsMagento2Family(framework)` / `engine.Magento2FamilyDisplayName(framework)` (`internal/engine/framework_family.go`) tồn tại như nơi duy nhất chứa quyết định đó, áp dụng ở mọi call site trước đây check `framework == "magento2"` theo nghĩa đen. Khi thêm một framework là biến thể gần của framework có sẵn, hãy grep mọi so sánh chuỗi `== "<framework-có-sẵn>"` trong `internal/cmd` và `internal/engine`, rồi quyết định từng trường hợp một xem framework mới có thuộc về check đó không — đừng giả định "trông giống nhau" nghĩa là "hành xử giống hệt ở mọi nơi." Một bug thật đã lên production đúng kiểu này: bước auto-configuration `setup:config:set` của Mage-OS dùng DB credentials hardcode `"magento"`/`"magento"`/`"magento"` của Magento 2 thay vì `"mageos"`/`"mageos"`/`"mageos"` của chính Mage-OS, vì call site đó bị bỏ sót khi thêm Mage-OS — chỉ bắt được khi thực sự chạy thử bootstrap Mage-OS thật từ đầu đến cuối.

---

[Kiến trúc](/vi/developer/architecture) | [Đóng góp](/vi/developer/contributing) | [Tham khảo Frameworks](/vi/reference/frameworks)
