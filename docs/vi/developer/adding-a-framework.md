---
title: Thêm Framework mới vào Govard
description: Cấu trúc nội bộ của framework registry trong Govard, và hướng dẫn từng file cụ thể để thêm hỗ trợ cho một framework mới.
---

# Thêm Framework mới

Govard hỗ trợ một danh sách framework ngày càng mở rộng — Magento 2, Mage-OS, Magento 1, OpenMage, Laravel, Symfony, Drupal, WordPress, Next.js, Emdash, Shopware, CakePHP, PrestaShop, Django, Custom, và sẽ còn thêm nữa theo thời gian. Trang này mô tả cấu trúc nội bộ của phần hỗ trợ đó, và những gì cần đụng vào để thêm một framework mới.

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

    DefaultDBCredentials   DefaultDBCredentials   // port/username/password/database-name mặc định cho local dev
    PHPStanPaths           []string               // đường dẫn phân tích `govard test phpstan` mặc định, nil dùng mặc định chung {"app","src"}
    ComposerCodingStandard ComposerCodingStandard // package Composer + label --standard phpcs của coding standard framework này

    Bootstrap      BootstrapFactory              // func(bootstrap.Options) bootstrap.FrameworkBootstrap
    BaseURLManager func() tunnel.BaseURLManager  // nil nếu framework không cần rewrite base-URL cho tunnel

    SupportsBootstrap    bool // cho phép `govard bootstrap` (quy trình remote/clone)
    SupportsFreshInstall bool // cho phép `govard bootstrap --fresh`

    FreshInstall                func(bootstrap.Options, string, bootstrap.CmdHelpers) error // orchestration fresh-install; nil nếu chưa migrate khỏi switch cũ
    FreshInstallNeedsDB         bool // populate DB credentials trước khi gọi FreshInstall
    FreshInstallNeedsDomain     bool // populate domain trước khi gọi FreshInstall
    FreshInstallManagesOwnEnvUp bool // true nếu FreshInstall đã tự gọi `env up`

    PreConfigureHook func(bootstrap.Options, string, bootstrap.CmdHelpers) error // setup của clone workflow phải chạy trước `govard config auto`
    PostCloneHook    func(bootstrap.Options, string, bootstrap.CmdHelpers) error // setup của clone workflow chạy sau dispatch PostClone chung

    PHPImageVariant string // hậu tố variant image PHP cho container của framework này, "" dùng image thường

    DBDriverCategory string // category DB-user/label per-project cho phpMyAdmin, "" fallback về "app"

    Upgrade engine.UpgradeFunc // pipeline `govard upgrade` (deps, migration, flush cache), nil nếu chưa triển khai

    RunMappingAssetPreparer engine.RunMappingAssetPreparer // chuẩn bị asset "run mapping" nginx/apache theo store trước khi render, nil nếu không có

    TablePrefixDetector engine.TablePrefixDetector // đọc file config riêng của framework để lấy table prefix DB, nil nếu không có khái niệm table prefix

    VersionProfileResolver engine.VersionProfileResolver // resolve override runtime-profile theo phiên bản, nil cho mọi framework trừ magento2 hiện tại

    TemplateFuncs template.FuncMap // các hàm template blueprint bổ sung framework này góp vào, nil với hầu hết

    ProbeRemoteDB func(remoteName string, remoteCfg engine.RemoteConfig) (remote.MagentoDBInfo, error) // probe một remote để lấy DB credentials thực, nil nếu chưa triển khai

    AutoConfigure func(cmd *cobra.Command, config engine.Config) error // setup sau khi render riêng của framework cho `govard config auto`, nil nếu chưa hỗ trợ
}
```

`internal/frameworks/all_generated.go` — được sinh bởi `go generate ./internal/frameworks/...` từ `Spec()` của mỗi package (xem bước 6 dưới đây) — gọi `RegisterSpecs([]types.FrameworkSpec{<pkg>.Spec(), ...})` một lần với spec của mọi framework đã đăng ký, theo một thứ tự cụ thể (lý do ở phần dưới). `RegisterSpecs` resolve từng spec thành một `types.FrameworkDefinition` đầy đủ (`Definition` của một spec gốc được dùng nguyên vẹn; `Definition` của parent thuộc một spec con được resolve trước, sau đó `types.FrameworkPatch` của spec con được áp lên trên — xem "Fork một framework có sẵn" dưới đây) và tạo nên một registry cấp package. Thêm framework vào registry nghĩa là các field trong `Definition()` đã resolve của nó tự động chảy qua mọi nơi dispatch theo danh tính framework — nhưng giờ có 3 cơ chế khác nhau, không chỉ 1:

| File | Vai trò |
| :--- | :--- |
| `internal/frameworks/registry.go` | `Get(name)`, `All()`, `Normalize(name)` — bản thân registry, xử lý alias |
| `internal/frameworks/run.go` | `RunBootstrap(name, opts)` — dispatch tới `def.Bootstrap` thay vì switch |
| `internal/frameworks/base_url.go` | `NewBaseURLManager(name)` — dispatch tới `def.BaseURLManager`, fallback về `tunnel.NoopManager` |

- **3 file trên** là phía đọc của chính registry, dùng bởi allowlist của `govard bootstrap`, base-URL rewriting của `govard tunnel`, và bootstrap dispatcher.
- **Đọc field top-down**: code trong `internal/cmd`/`internal/desktop` (đã import sẵn `internal/frameworks`) gọi `frameworks.Get(name)` rồi đọc trực tiếp một field của `Definition()` — `DefaultDBCredentials`, `PHPStanPaths`, `ComposerCodingStandard`, `ProbeRemoteDB`, `AutoConfigure`, `FreshInstall` (cùng các field đi kèm `FreshInstallNeedsDB`/`FreshInstallNeedsDomain`/`FreshInstallManagesOwnEnvUp`), `PreConfigureHook`/`PostCloneHook`. Đây là lựa chọn mặc định cho những gì chỉ `cmd`/`desktop` cần.
- **Registry do engine sở hữu**: `internal/engine` không thể import ngược `internal/frameworks` (`frameworks` import `engine`, không phải ngược lại), nên một số thứ engine tự cần dispatch — `PHPImageVariant`, `DBDriverCategory`, `Upgrade`, `RunMappingAssetPreparer`, `TablePrefixDetector`, `VersionProfileResolver`, và từng entry của `TemplateFuncs` — được đẩy vào một lệnh gọi `engine.RegisterX(...)` tương ứng từ `frameworks.Register` (`internal/frameworks/registry.go`) lúc registration, vd `engine.RegisterPHPImageVariant`, `engine.RegisterUpgrader`. Các hàm phía đọc của engine (`PHPImageVariantForFramework`, `UpgradeFramework`, v.v.) sau đó dispatch dựa trên registry đó thay vì switch theo từng framework.

Bất kể field nào đi theo đường nào, không còn `switch framework { case "magento2": ... }` hardcode cho nó — thêm framework vào registry nghĩa là nó tự động tham gia ở mọi nơi field đó được đọc, không cần sửa switch nào.

### Dispatch cho fresh-install và clone workflow

`internal/cmd/bootstrap_remote.go` (orchestration cho clone workflow) và `internal/cmd/bootstrap_fresh_install.go` (orchestration cho fresh-install) dispatch hoàn toàn qua registry, không còn switch theo tên framework nào trong cả hai.

Interface method `FrameworkBootstrap.PostClone` chung (`bootstrapPostCloneDefinition`) xử lý bước post-clone của clone workflow cho đa số framework. Field tùy chọn `PreConfigureHook`/`PostCloneHook` trên `Definition()` dành cho framework nào cần timing bước chi tiết hơn, hoặc cần quyền truy cập `*cobra.Command`, mà interface method đó không diễn đạt được — hiện chỉ có nhóm Magento dùng (sinh env.php trước khi chạy `govard config auto`, tạo admin user/reindex sau bước post-clone chung).

Mọi framework đã đăng ký đều tự có field `FreshInstall` trên `Definition()` của mình (`internal/frameworks/<name>/freshinstall.go`):

- Symfony, Laravel, Drupal, WordPress, Shopware, CakePHP delegate sang helper chung `bootstrap.GenericFreshInstall` cho chuỗi `CreateProject → Install → govard config auto`.
- OpenMage, Next.js, Emdash, Django tự viết trình tự riêng ngay trong `freshinstall.go` của mình.
- Magento 2 và Mage-OS delegate sang bộ orchestrator chung `magento2.FreshInstall` (`internal/frameworks/magento2/bootstrap.go`), tham số hóa qua `magento2.Variant`/`mageos.Variant` — package của Mage-OS import thẳng package của Magento 2 thay vì cả hai cùng phụ thuộc một package trung lập thứ ba, vì Magento 2 là bản triển khai chính còn Mage-OS là bản fork (`Config`/`Manifest` dùng chung của cả hai cũng theo đúng cách sở hữu này — xem bước 1 bên dưới).
- `FreshInstall` của Magento 1 trả về cố định lỗi "dùng OpenMage thay thế": fresh install của nó không được hỗ trợ theo thiết kế, nhưng `SupportsFreshInstall` vẫn để `true` để lỗi riêng này được trả về thay vì bị chặn ngay ở allowlist CLI chung.
- PrestaShop là framework đã đăng ký duy nhất hoàn toàn không có field `FreshInstall`: nó không bao giờ set `SupportsFreshInstall`, nên `runBootstrapFrameworkFreshInstall` không bao giờ chạy cho nó.

### Dữ liệu tĩnh nằm ngoài registry

`custom` là entry duy nhất trong map `FrameworkConfigs` của `internal/engine/framework_config.go` và object `"frameworks"` của `internal/engine/framework_manifest.json`. `Config`/`Manifest` của mọi framework khác nằm ngay trong package `internal/frameworks/<name>/` của chính nó, dưới dạng literal `config.go`/`manifest.go`, được đẩy vào `engine.FrameworkConfigs`/manifest store lúc registration qua `engine.RegisterFrameworkConfig`/`RegisterFrameworkManifest` (được gọi từ `frameworks.Register`, và hàm này lại được gọi từ code init sinh tự động trong `internal/frameworks/all_generated.go`). `engine.GetFrameworkConfig`/`GetFrameworkManifestConfig` là phía đọc của map/store đó.

---

## Thêm một framework mới: checklist

Giả sử bạn đang thêm một framework hư cấu tên `whimsy`. Mỗi bước dưới đây đều có một ví dụ thật, đang chạy trong codebase — các đường dẫn file trỏ tới ví dụ gần giống nhất để copy.

### 1. Cấu hình runtime mặc định — `internal/frameworks/whimsy/config.go`

Tạo file `config.go` với một biến cấp package `var config = engine.FrameworkConfig{...}` (phiên bản PHP/Node, nginx template, engine/phiên bản DB, danh sách includes). Copy theo framework gần giống nhất — vd `internal/frameworks/cakephp/config.go` cho stack PHP+MariaDB thông thường, `internal/frameworks/nextjs/config.go` cho stack chỉ Node không DB.

Nếu `whimsy` là bản fork gần giống một framework đã có (cùng stack runtime, giá trị mặc định gần như giống hệt), đừng copy nguyên literal — đặt một constructor dùng chung, tham số hóa theo đúng vài field thực sự khác nhau, vào package của framework nào là bản chính/lâu đời hơn, rồi để package của bản fork import thẳng. Hai ví dụ: `magento2.BuildConfig` (`internal/frameworks/magento2/config.go`, được gọi từ `internal/frameworks/mageos/config.go`) và `magento1.BuildConfig` (`internal/frameworks/magento1/config.go`, được gọi từ `internal/frameworks/openmage/config.go`).

### 2. Dữ liệu manifest — `internal/frameworks/whimsy/manifest.go`

Tạo file `manifest.go` với một biến cấp package `var manifest = engine.FrameworkManifestConfig{...}`: exclude khi sync (`Paths.LocalMedia`/`RemoteMedia`, `WebRootCandidates`), bảng DB nhạy cảm/bỏ qua, và khối `Features`:

```go
package whimsy

import "govard/internal/engine"

var manifest = engine.FrameworkManifestConfig{
    Ignored:   []string{},
    Sensitive: []string{},
    Paths: engine.FrameworkPathConfig{
        LocalMedia:        "public/uploads",
        RemoteMedia:       "public/uploads",
        WebRootCandidates: []engine.FrameworkWebRootCandidate{},
    },
    Features: engine.FrameworkFeatureConfig{
        RequiresRunningEnvForFreshInstall: false,
        SupportsPostClone:                 true,
    },
}
```

Copy theo framework gần giống nhất — vd `internal/frameworks/cakephp/manifest.go` hoặc `internal/frameworks/django/manifest.go`. `RequiresRunningEnvForFreshInstall` quyết định `govard bootstrap --fresh` khởi động container *trước* hay *sau* khi chạy `CreateProject` — xem mục "gotcha" bên dưới về vấn đề này trước khi đặt nó `true`.

Quy tắc tương tự `config.go` ở trên: `magento2.Manifest` (`internal/frameworks/magento2/manifest.go`, được `internal/frameworks/mageos/manifest.go` tham chiếu) và `magento1.Manifest` (`internal/frameworks/magento1/manifest.go`, được `internal/frameworks/openmage/manifest.go` tham chiếu).

### 3. Tài sản blueprint — `internal/frameworks/whimsy/blueprint/` + `embed.go`

Tài sản blueprint của framework — đoạn Compose, template vhost nginx, và bất kỳ thứ gì khác blueprint cần — giờ nằm ngay trong package riêng của framework, không còn nằm dưới thư mục dùng chung `internal/blueprints/files/<name>/` nữa (cây thư mục đó vẫn tồn tại, nhưng chỉ chứa tài sản thực sự dùng chung giữa các framework: `proxy.yml`, `includes/`, và template mặc định chung trong `support/nginx/templates`). Gồm hai phần:

1. **`internal/frameworks/whimsy/blueprint/`** — các file tài sản thực tế:
   - `services.yml` (đoạn Docker Compose, được render qua Go template) nếu framework cần — copy theo ví dụ gần nhất (`internal/frameworks/nextjs/blueprint/services.yml` cho runtime Node, `internal/frameworks/cakephp/blueprint/` cho framework PHP chỉ cần template nginx, không cần đoạn compose riêng). Không phải framework nào cũng cần file này: framework tái dùng hẳn compose của framework khác (Mage-OS tái dùng của Magento 2 — xem `varnishTemplateFramework` trong `internal/engine/render.go`) bỏ qua file này, cũng như framework chỉ đóng góp template nginx (cakephp, drupal, wordpress hiện tại).
   - một template vhost nginx (vd `whimsy.conf`) nếu framework cần một template khác với mặc định chung.
   - bất kỳ tài sản lồng nhau nào khác mà blueprint cần — xem `internal/frameworks/magento2/blueprint/varnish/default.vcl` như một ví dụ ngoài `services.yml`/template nginx.

2. **`internal/frameworks/whimsy/embed.go`** — embed thư mục đó và ghép nó vào cây `blueprints.FS` hợp nhất tại thời điểm package init. Copy theo `internal/frameworks/magento2/embed.go` (có cả tài sản lồng nhau lẫn template nginx) hoặc bản đơn giản hơn `internal/frameworks/cakephp/embed.go` (chỉ có template nginx) làm khuôn mẫu ban đầu:

   ```go
   package whimsy

   import (
       "embed"
       "io/fs"

       "govard/internal/blueprints"
   )

   //go:embed all:blueprint
   var blueprintFiles embed.FS

   var BlueprintFS fs.FS

   func init() {
       var err error
       BlueprintFS, err = fs.Sub(blueprintFiles, "blueprint")
       if err != nil {
           panic(err)
       }

       blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
           Framework:     "whimsy",
           FS:            BlueprintFS,
           HasDir:        true, // false nếu whimsy chỉ đóng góp template nginx, không có services.yml/tài sản khác
           NginxTemplate: "whimsy.conf", // "" nếu whimsy không có template nginx riêng
       })
   }
   ```

   `HasDir: true` ghép toàn bộ `BlueprintFS` thành `whimsy/` trong cây hợp nhất (vd `whimsy/services.yml`); `NginxTemplate`, nếu được đặt, luôn được ghép tại `support/nginx/templates/whimsy.conf` bất kể `HasDir`. Xem chú thích doc của `FrameworkMount` trong `internal/blueprints/blueprints.go` để biết đầy đủ hợp đồng (contract). Không cần thêm lệnh đăng ký nào ngoài `init()` này — miễn là có thứ gì đó import `internal/frameworks/whimsy` (bước 5's `all_generated.go` đã làm việc này), Go sẽ chạy `init()` này trước khi `blueprints.FS` được đọc lần nào.

Nếu service của bạn cần chạy `user: root` (vd image Node/Python gốc cần root để `npm`/`pip` install không lỗi quyền), hãy cách ly thư mục nào nó ghi vào mà là cache build hay cây dependency — không phải source bạn cần xem trên host — bằng named Docker volume thay vì bind mount, để file root-owned không bao giờ chạm tới host filesystem. Xem `node-modules:` trong `internal/frameworks/emdash/blueprint/services.yml` và `next-cache:` trong `internal/frameworks/nextjs/blueprint/services.yml`. Với những chỗ ghi không cách ly được theo cách đó (vd `__pycache__` của Django, rải rác khắp cây thư mục dự án), chown lại thư mục về đúng owner của bind mount (`stat -c %u:%g .` — không cần truyền UID qua đâu cả) sau lệnh đã chạy as root; xem `installAndMigrate` trong `internal/frameworks/django/bootstrap.go` và `command:` trong `internal/frameworks/django/blueprint/services.yml`.

### 4. Triển khai Bootstrap — `internal/frameworks/whimsy/bootstrap.go`

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

Copy `internal/frameworks/cakephp/bootstrap.go` cho framework PHP dùng chung helper `bootstrap.RunStagedCreateProject` (`internal/engine/bootstrap/staged_project.go`), hoặc `internal/frameworks/emdash/bootstrap.go` cho framework mà `CreateProject` không cần container nào cả (chỉ tải HTTP thuần).

**Nếu framework cần chạy một CLI tool (`npx`, `composer`, v.v.) để scaffold dự án, nó phải chạy bên trong container — không bao giờ được giả định là công cụ đã có sẵn trên host.** Xem mục "gotcha" về container execution bên dưới.

### 5. Gắn vào registry — `internal/frameworks/whimsy/whimsy.go`

```go
package whimsy

import (
    "govard/internal/engine"
    "govard/internal/engine/bootstrap"
    "govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
    return types.FrameworkDefinition{
        Name:        "whimsy",
        DisplayName: "Whimsy",
        Config:      config,
        Manifest:    manifest,
        Detect: engine.DetectionSpec{
            ComposerPackages: []string{"whimsy/framework"}, // hoặc PackageJSONDeps, AuthJSONHosts, FilePaths
        },
        Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
            return NewWhimsyBootstrap(opts)
        },
        SupportsFreshInstall: true,
        SupportsBootstrap:    true, // chỉ nếu nó cũng hỗ trợ quy trình remote/clone
    }
}
```

`config` và `manifest` chính là 2 biến cấp package từ bước 1 và 2 — không cần lookup theo tên, vì `Definition()` nằm cùng package với nơi khai báo chúng. `NewWhimsyBootstrap` là constructor cục bộ từ `bootstrap.go` ở bước 4 (không có tiền tố `bootstrap.` — bộ bootstrapper nằm ngay trong package `whimsy`, không phải `internal/engine/bootstrap`). Chỉ đặt `BaseURLManager` nếu framework cần rewrite base-URL riêng cho `govard tunnel` (đa số không cần — `tunnel.NoopManager` mặc định là no-op, đúng cho bất kỳ framework nào không tự lưu base URL trong database hay file config).

Mỗi package framework cũng cần một hàm `Spec()` — đây, không phải `Definition()`, mới là thứ `all_generated.go` thực sự gọi. Với một framework hoàn toàn mới, độc lập như `whimsy`, đây chỉ là một dòng bọc trực tiếp `Definition()`:

```go
// Spec declares Whimsy as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
```

Copy nguyên văn từ bất kỳ framework không-fork nào, vd `internal/frameworks/wordpress/spec.go` hoặc `internal/frameworks/custom/spec.go`. Nếu `whimsy` lại là bản fork gần của một framework có sẵn, xem mục "Fork một framework có sẵn" dưới đây trước khi viết `Spec()` — `Spec()` của một bản fork trông khác hẳn dòng bọc đơn giản này.

### 5b. Fork một framework có sẵn — `Parent` và `FrameworkPatch`

Nếu `whimsy` là bản fork gần như giống hệt một framework đã đăng ký (như cách Mage-OS fork từ Magento 2, hay OpenMage fork từ Magento 1), đừng copy lại nguyên `Definition()` của framework đó. Thay vào đó, khai báo `whimsy` như một **spec con**: cho nó một `Parent` và một `types.FrameworkPatch` chỉ liệt kê những field thực sự khác với parent. Mọi field khác được kế thừa tự động khi `RegisterSpecs` resolve registry lúc khởi động.

`types.FrameworkPatch` có một field `types.Override[T]` cho mỗi field có thể kế thừa của `FrameworkDefinition` (xem `internal/frameworks/types/spec.go` để biết danh sách đầy đủ). Giá trị zero của `Override[T]` nghĩa là "kế thừa nguyên giá trị của parent" — để thực sự thay đổi gì đó, bạn phải gọi một trong hai:

- `types.Set(value)` — thay giá trị kế thừa bằng `value`.
- `types.Clear[T]()` — reset tường minh về giá trị zero của `T` (khác với "kế thừa", vì với một số field, giá trị đúng của bản fork thực sự *là* giá trị zero, vd một pipeline `Upgrade` mà parent có nhưng bản fork chủ ý không có).

Ví dụ thật — `Spec()` của `internal/frameworks/mageos/mageos.go` (Mage-OS kế thừa hầu hết hành vi của Magento 2, nhưng patch riêng display name, DB mặc định, chữ ký nhận diện, và một số field đặc thù Magento 2 mà nó không dùng chung):

```go
// Spec declares Mage-OS as a Magento 2 child. Every inherited behavior is
// intentionally omitted; only distribution-specific deltas remain here.
func Spec() types.FrameworkSpec {
    def := Definition()
    return types.FrameworkSpec{
        Parent: "magento2",
        Definition: types.FrameworkDefinition{
            Name:    def.Name,
            Aliases: def.Aliases,
        },
        Patch: types.FrameworkPatch{
            DisplayName:            types.Set(def.DisplayName),
            MigrationTypes:         types.Clear[types.MigrationTypes](),
            Config:                 types.Set(def.Config),
            DefaultDBCredentials:   types.Set(def.DefaultDBCredentials),
            Detect:                 types.Set(def.Detect),
            Bootstrap:              types.Set(def.Bootstrap),
            FreshInstall:           types.Set(def.FreshInstall),
            DBDriverCategory:       types.Clear[string](),
            Upgrade:                types.Set(def.Upgrade),
            VersionProfileResolver: types.Clear[engine.VersionProfileResolver](),
            // ...and so on for every other field that differs from magento2.
        },
    }
}
```

Lưu ý `def := Definition()` vẫn tồn tại và vẫn được điền đầy đủ (code khác, và chính `Spec()` này, đọc field trực tiếp từ nó) — pattern fork chỉ thay đổi những gì `Spec()` báo cho registry, không thay đổi `Definition()` bản thân nó. Xem `internal/frameworks/openmage/openmage.go` để có ví dụ thật thứ hai (OpenMage fork từ Magento 1) với một bộ patch khác, nhỏ hơn — hai bản fork không patch cùng field, vì mỗi bản fork có delta hành vi thực tế khác nhau so với parent của nó.

`Parent` phải trỏ tới một framework đã đăng ký (được kiểm tra lúc khởi động — `RegisterSpecs` panic nếu gặp parent không tồn tại hoặc chu trình kế thừa, nên lỗi gõ nhầm ở đây fail ngay lúc process khởi động, không âm thầm ở runtime). Chỉ fork một framework theo cách này nếu nó thực sự là biến thể gần với một parent có thật, đứng trước nó theo alphabet, để kế thừa; đa số framework mới là framework gốc, không phải fork.

### 6. Đăng ký — không cần sửa gì

Việc đăng ký được sinh tự động, không còn duy trì thủ công: chạy `make generate` (hoặc `go generate ./internal/frameworks/...`) và `internal/frameworks/all_generated.go` sẽ tự nhận `Spec()` của package `whimsy` mới — không cần thêm import hay dòng `RegisterSpecs()` nào bằng tay. `make build` và `make test` đã tự chạy bước này cho bạn, nên trên thực tế bạn chỉ cần chạy tay khi muốn xem trước file sinh ra trước khi build.

**Vị trí vẫn có ý nghĩa cho detection**: `DetectFramework` duyệt qua các framework theo đúng thứ tự đăng ký và trả về kết quả khớp đầu tiên, nên nếu chữ ký nhận diện của framework mới có thể trùng với framework khác đã đăng ký, nó cần được đăng ký ở đúng vị trí tương đối. Thứ tự mặc định là theo alphabet; trường hợp ngoại lệ duy nhất đã biết (Emdash phải đăng ký trước Next.js) được khai báo trong map `PriorityOverrides` của `internal/frameworks/gen/generator/order.go` — thêm một entry ở đó, không phải trong `all_generated.go`, nếu chữ ký nhận diện của `whimsy` cũng mơ hồ tương tự với một framework đã có.

### 7. Orchestration fresh-install / clone — `internal/cmd`

Toàn bộ chỗ này đã lên registry — không còn switch theo tên framework nào để sửa:

- Nếu `whimsy` khớp khuôn chung `CreateProject → Install → govard config auto`, thêm file `internal/frameworks/whimsy/freshinstall.go` với một hàm `freshInstall` chỉ đơn giản delegate sang `bootstrap.GenericFreshInstall(NewWhimsyBootstrap(opts), projectDir, helpers)` — copy theo `internal/frameworks/cakephp/freshinstall.go` — rồi gắn vào qua field `FreshInstall` (cùng `FreshInstallNeedsDB`/`FreshInstallNeedsDomain`) trên `Definition()`. `runBootstrapFrameworkFreshInstall` sẽ tự nhận, không cần sửa switch nào. Nếu service compose của `whimsy` không thể khởi động được với một project rỗng/chưa migrate (nên `FreshInstall` phải tự bật môi trường lên trước khi chạy `Install()`/migrate — Django cần điều này), hãy đặt thêm `FreshInstallManagesOwnEnvUp: true` trên `Definition()` để `bootstrapCmd.RunE` bỏ qua bước `env up` dư thừa của chính nó sau đó.
- Nếu cần các bước riêng, viết orchestration đó ngay trong `internal/frameworks/whimsy/freshinstall.go` — vẫn gắn vào theo đúng cách qua field `FreshInstall` của `Definition()`, chỉ là không delegate sang `bootstrap.GenericFreshInstall`; copy `internal/frameworks/openmage/freshinstall.go` hoặc `internal/frameworks/django/freshinstall.go` làm khung ban đầu (file của Django cho thấy cách override `Options.Runner` thành một runner `CmdHelpers` không phải PHP, và cách dùng `Options.SkipUp`/`bootstrap.ErrFreshInstallSkipUp`). Nếu fresh install thực sự không được hỗ trợ (giống Magento 1: `FreshInstall` chỉ trả về lỗi bảo người dùng dùng OpenMage thay thế), một closure `FreshInstall` trả lỗi đó vẫn đơn giản hơn một case switch — mọi framework đã đăng ký đều diễn đạt hành vi fresh-install theo cách này, không còn ngoại lệ nào.
- Nếu nó hỗ trợ quy trình remote/clone (`SupportsBootstrap: true`, không chỉ fresh-install), nó tự động được `bootstrap_remote.go`'s post-clone dispatch (`bootstrapPostCloneDefinition`) nhận diện — không cần sửa switch nào ở đó trừ khi nó thuộc nhóm Magento (bị `bootstrapPostCloneDefinition` loại trừ qua `engine.IsMagento2Family`, vì bước pre-configure/post-clone thật của nó đi qua field `PreConfigureHook`/`PostCloneHook` thay thế - xem đoạn ngay dưới đây).

Nếu bước post-clone của `whimsy` trong clone workflow cần dùng `*cobra.Command` (để chạy `govard tool <x>`) mà interface method thuần `FrameworkBootstrap.PostClone(projectDir)` không diễn đạt được, hoặc cần chạy trước `govard config auto` thay vì sau bước post-clone dispatch chung, hãy set `PreConfigureHook`/`PostCloneHook` trên `Definition()` — cùng kiểu chữ ký `func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error` như `FreshInstall`. Copy `magento2.PreConfigure`/`magento2.PostClone` (`internal/frameworks/magento2/bootstrap.go`) làm khung ban đầu, và thêm bất kỳ closure `CmdHelpers` mới nào hook của bạn cần vào `internal/engine/bootstrap/base.go` và dispatcher trong `internal/cmd/bootstrap_remote.go` — cả hai field này là hạ tầng chung cho mọi framework, không riêng gì Magento, dù Magento 2/Mage-OS là hai framework đầu tiên dùng đến chúng.

### 8. Docs

Thêm dòng vào bảng support/runtime-defaults và một mục ngắn trong [`docs/reference/frameworks.md`](/reference/frameworks) (và bản tiếng Việt, `docs/vi/reference/frameworks.md`).

### 9. Test

- `tests/framework_detection_test.go` — một `TestWhimsyDiscovery` khớp với chữ ký `Detect` bạn đã dùng.
- `tests/framework_definitions_test.go` hoặc một file test riêng cho `whimsy` — assert `Definition()`'s `Config`/`Manifest`/`Bootstrap` được điền đúng như kỳ vọng.
- `tests/framework_snapshot_test.go` — đây là lưới an toàn golden-snapshot bao phủ *mọi* framework đã đăng ký tự động (render blueprint, `FreshCommands()`, config/profile đã resolve, DB credentials/manifest mặc định) qua `allFrameworkNames`; đăng ký `whimsy` (tự động ngay khi bạn chạy `make generate` sau khi thêm folder — xem bước 6) khiến nó bắt đầu chạy, nhưng fixture golden của nó tại `tests/testdata/framework_snapshots/whimsy/` chưa tồn tại. Sinh chúng khi bạn chắc chắn output render đã đúng:

  ```bash
  UPDATE_GOLDEN=1 go test ./tests/... -run TestFrameworkSnapshot
  ```

  Luôn xem lại diff của fixture vừa sinh trước khi commit — `UPDATE_GOLDEN=1` ghi ra bất kể code hiện đang sinh ra gì, đúng hay sai.

### 10. Validate thật

Test render/dispatch chỉ xác nhận output *như kỳ vọng* — không bắt được việc container không thể ra internet, image thiếu 1 binary, hay việc container có thực sự sẵn sàng phục vụ traffic đúng lúc reverse proxy đăng ký nó hay không. Những kiểu lỗi đó vô hình với `go test ./...` và chỉ lộ ra khi thực sự chạy lệnh và kiểm tra kết quả:

```bash
mkdir -p /tmp/whimsy-test && cd /tmp/whimsy-test
govard bootstrap --framework whimsy --fresh --yes
curl -sk -o /dev/null -w '%{http_code}\n' https://whimsy-test.test/   # kỳ vọng 200, không phải lỗi docker/proxy
govard env down
```

---

## Những "gotcha" cần biết

### Chạy trong container, không phải trên host

Mọi `FrameworkBootstrap.CreateProject`/`Install` nào shell ra một CLI tool (`composer`, `npx`, `npm`) phải chạy tool đó **bên trong container**, không bao giờ qua `exec.Command` trần trên host — tooling của host (hoặc việc thiếu nó, hay một config toàn cục lạc trôi như `~/.npmrc`) nằm ngoài tầm kiểm soát của Govard và gây ra lỗi âm thầm, chỉ xảy ra trên từng máy cụ thể. Các framework PHP làm điều này qua `bootstrap.Options.Runner` (một closure `func(command string) error` mà `internal/cmd` gắn với `runPHPContainerShellCommand`, exec vào container PHP đang chạy sẵn). Framework dùng Node mà chưa có service compose nào chạy sẵn để exec vào (`CreateProject` của Next.js) dùng `nodeCreateProjectRunner` trong `internal/cmd/bootstrap.go` thay vào đó, chạy lệnh scaffold trong một container `docker run --rm -v <projectDir>:/app node:<version> ...` dùng 1 lần rồi bỏ — độc lập với cả môi trường host lẫn việc có service nào do compose quản lý đang chạy hay không.

### Đừng giả định service compose đã sẵn sàng

Một lựa chọn hấp dẫn thay cho cách container-dùng-1-lần ở trên là exec thẳng vào container service "web" đang chạy dài hạn của framework đó (giống pattern PHP). Cách này *chỉ* work nếu container đó đã chạy sẵn vào lúc `CreateProject` thực thi — mà với đa số framework thì chưa: `govard bootstrap --fresh` chạy fresh-install *trước* `env up` cho bất kỳ framework nào không đặt `requires_running_env_for_fresh_install: true` trong manifest. Lật cờ đó để buộc env-up chạy trước lại tạo ra vấn đề tinh vi hơn: container bắt đầu chạy lệnh dài hạn bình thường của nó (vd `npm run dev`) trong khi thư mục dự án vẫn còn *rỗng*, nên nó thoát ngay lập tức — và bước đăng ký domain/proxy của pipeline bootstrap chạy đúng trong khoảng thời gian đó, đăng ký route tới một container chưa thực sự phục vụ gì. Việc đăng ký âm thầm "thành công", nhưng reverse proxy không bao giờ có backend hoạt động, và request `https://<project>.test/` đầu tiên trả về 502 cho tới khi chạy tay `env down && env up`.

Nếu một framework thực sự cần container service dài hạn của nó chạy trước khi `CreateProject` có thể exec vào, lệnh khởi động của container đó cần chịu được thư mục dự án rỗng/chưa đầy đủ (chờ vòng lặp cho tới khi có file đánh dấu, hoặc tương tự) *và* thời điểm đăng ký domain cần xảy ra sau khi app thực sự đang phục vụ — không chỉ sau khi tiến trình container khởi động. Emdash tránh hoàn toàn vấn đề này: `CreateProject` của nó không cần container nào (chỉ tải tarball qua HTTP thuần), và lệnh compose của nó đã có sẵn cơ chế "cài nếu thiếu `node_modules`" để phòng thủ, nhưng chưa bao giờ cần "chờ file xuất hiện", vì tới lúc container khởi động thì file đã có sẵn rồi.

### Thuộc cùng "họ" không có nghĩa tự động giống hệt

Mage-OS là bản fork drop-in của Magento 2 và tái dùng phần lớn hành vi runtime của nó, nhưng "tái dùng phần lớn" không phải "tái dùng toàn bộ" — DB credentials mặc định, ngưỡng phiên bản chọn search engine, và exec user đều là những điểm quyết định riêng, mỗi cái cần một check tường minh thay vì một giả định. `engine.IsMagento2Family(framework)` / `engine.Magento2FamilyDisplayName(framework)` (`internal/engine/framework_family.go`) là nơi duy nhất chứa quyết định đó, thay vì so sánh chuỗi `framework == "magento2"` ở từng call site. Khi thêm một framework là biến thể gần của framework có sẵn, hãy grep mọi so sánh chuỗi `== "<framework-có-sẵn>"` trong `internal/cmd` và `internal/engine`, rồi quyết định từng trường hợp một xem framework mới có thuộc về check đó không — đừng giả định "trông giống nhau" nghĩa là "hành xử giống hệt ở mọi nơi" (một call site auto-configuration DB chính là kiểu chỗ dễ âm thầm sai nhất: nó vẫn chạy được, chỉ là nhắm nhầm database).

### File root-owned trên bind mount của host

Một service chạy `user: root` (image Node/Python gốc cần vậy để `npm`/`pip` install không lỗi quyền) sẽ ghi bất kỳ file nào nó tạo trong thư mục dự án bind-mount dưới quyền root — user trên host không xóa/sửa được nếu không có `sudo`. Không có 1 fix chung cho mọi trường hợp; chọn theo từng thư mục:

- Nếu thư mục đó là cache có thể build lại hoặc cây dependency, không phải thứ dev cần xem trên host, hãy cách ly nó bằng named Docker volume thay vì bind mount (`node-modules:` trong `internal/frameworks/emdash/blueprint/services.yml`, `next-cache:` trong `internal/frameworks/nextjs/blueprint/services.yml`) — file root-owned không bao giờ chạm tới host filesystem.
- Nếu file root-owned có thể xuất hiện ở bất kỳ đâu trong cây thư mục (`__pycache__` của Django), chown lại thư mục dự án về đúng owner của bind mount sau lệnh đã chạy as root: `chown -R "$(stat -c %u:%g .)" .` — lệnh này đọc owner *hiện tại* của mount point thay vì phải truyền UID/GID host qua như một tham số mới. Xem `installAndMigrate` trong `internal/frameworks/django/bootstrap.go` và `command:` trong `internal/frameworks/django/blueprint/services.yml`.

---

[Kiến trúc](/vi/developer/architecture) | [Đóng góp](/vi/developer/contributing) | [Tham khảo Frameworks](/vi/reference/frameworks)
