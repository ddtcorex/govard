---
title: Thêm Framework mới vào Govard
description: Cấu trúc nội bộ của framework registry trong Govard, và hướng dẫn từng file cụ thể để thêm hỗ trợ cho một framework mới.
---

# Thêm Framework mới

Govard hỗ trợ một danh sách framework ngày càng mở rộng — Magento 2, Mage-OS, Magento 1, OpenMage, Laravel, Symfony, Drupal, WordPress, Next.js, Emdash, Shopware, CakePHP, PrestaShop, Django, và sẽ còn thêm nữa theo thời gian. Trang này mô tả cấu trúc nội bộ của phần hỗ trợ đó, và những gì cần đụng vào để thêm một framework mới.

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

`internal/frameworks/all_generated.go` — được sinh bởi `go generate ./internal/frameworks/...` từ `Definition()` của mỗi package (xem bước 6 dưới đây) — gọi `Register(<pkg>.Definition())` cho từng framework đã đăng ký, theo một thứ tự cụ thể (lý do ở phần dưới), tạo nên một registry cấp package mà phần còn lại của Govard đọc qua 3 file nhỏ, tập trung:

| File | Vai trò |
| :--- | :--- |
| `internal/frameworks/registry.go` | `Get(name)`, `All()`, `Normalize(name)` — bản thân registry, xử lý alias |
| `internal/frameworks/run.go` | `RunBootstrap(name, opts)` — dispatch tới `def.Bootstrap` thay vì switch |
| `internal/frameworks/base_url.go` | `NewBaseURLManager(name)` — dispatch tới `def.BaseURLManager`, fallback về `tunnel.NoopManager` |

Mọi nơi đọc dữ liệu framework theo tên — allowlist của `govard bootstrap`, base-URL rewriting của `govard tunnel`, bootstrap dispatcher — đều đi qua 1 trong 3 file này thay vì `switch framework { case "magento2": ... }` hardcode rải rác. Thêm framework vào registry nghĩa là nó tự động tham gia cả 3 nơi đó, không cần sửa switch nào.

### Những gì *chưa* nằm trên registry

Có 1 phần vẫn là dữ liệu tĩnh theo tên framework, một cách có chủ đích, không phải do bỏ sót, và 1 dispatcher vẫn còn đúng một case switch sót lại:

1. **`internal/cmd/bootstrap_remote.go`** (orchestration cho clone workflow) giờ đã lên registry cho mọi framework đã đăng ký, kể cả Magento 2/Mage-OS: dispatch qua interface `FrameworkBootstrap.PostClone` chung (`bootstrapPostCloneDefinition`) đã lên registry từ trước nỗ lực này, còn các bước riêng của nhóm Magento trong clone workflow (sinh env.php trước khi chạy `govard config auto`, tạo admin user/reindex sau bước post-clone chung) giờ được gắn qua field `PreConfigureHook`/`PostCloneHook` trên `Definition()` thay vì một switch theo tên framework (xem bước 7 bên dưới). Ngược lại, **`internal/cmd/bootstrap_fresh_install.go`** (orchestration cho fresh-install) giờ chỉ còn đúng một `case`: Magento 1, vì fresh install của nó vốn không được hỗ trợ theo thiết kế (`CreateProject` chỉ trả lỗi bảo người dùng dùng `--clone`). Mọi framework trừ Magento 1 và PrestaShop giờ đều tự có field `FreshInstall` trên `Definition()` của mình trong registry (`internal/frameworks/<name>/freshinstall.go`) — phần lớn (Symfony, Laravel, Drupal, WordPress, Shopware, CakePHP) delegate sang helper chung `bootstrap.GenericFreshInstall` cho chuỗi `CreateProject → Install → govard config auto`, còn số khác (OpenMage, Next.js, Emdash, Django, và giờ có thêm Magento 2/Mage-OS) tự viết trình tự riêng ngay trong `freshinstall.go` của mình — Magento 2 và Mage-OS đều delegate sang bộ orchestrator chung `bootstrap.MagentoFamilyFreshInstall` (`internal/engine/bootstrap/magento_family.go`), tham số hóa qua `bootstrap.Magento2Variant`/`bootstrap.MageOSVariant` — dù theo cách nào, `runBootstrapFrameworkFreshInstall` cũng dispatch tới chúng mà không cần case riêng. Magento 1 và PrestaShop là hai framework hoàn toàn không có field `FreshInstall` (fresh install không được hỗ trợ với cả hai), nhưng lý do khác nhau: PrestaShop chưa từng có case trong switch nên không có gì để xóa, còn case của Magento 1 chính là case đang còn lại trong `bootstrap_fresh_install.go` ở trên, vì lỗi của nó cần được trả về trước khi dispatcher chung kịp tìm field `FreshInstall`.
2. **`custom`** là entry duy nhất còn nằm ở dạng dữ liệu tĩnh trong map `FrameworkConfigs` của `internal/engine/framework_config.go` và object `"frameworks"` của `internal/engine/framework_manifest.json`. `Config`/`Manifest` của mọi framework khác — kể cả của Magento 2 và Mage-OS — giờ nằm ngay trong package `internal/frameworks/<name>/` của chính nó, dưới dạng literal `config.go`/`manifest.go`, được đẩy vào `engine.FrameworkConfigs`/manifest store lúc registration qua `engine.RegisterFrameworkConfig`/`RegisterFrameworkManifest` (được gọi từ `frameworks.Register`, và hàm này lại được gọi từ code init sinh tự động trong `internal/frameworks/all_generated.go`) — `engine.GetFrameworkConfig`/`GetFrameworkManifestConfig` vẫn tồn tại như phía đọc của map/store đó, chỉ không còn là nơi framework mới thêm dữ liệu vào nữa.

---

## Thêm một framework mới: checklist

Giả sử bạn đang thêm một framework hư cấu tên `whimsy`. Mỗi bước dưới đây đều có một ví dụ thật, đang chạy trong codebase — các đường dẫn file trỏ tới ví dụ gần giống nhất để copy.

### 1. Cấu hình runtime mặc định — `internal/frameworks/whimsy/config.go`

Tạo file `config.go` với một biến cấp package `var config = engine.FrameworkConfig{...}` (phiên bản PHP/Node, nginx template, engine/phiên bản DB, danh sách includes). Copy theo framework gần giống nhất — vd `internal/frameworks/cakephp/config.go` cho stack PHP+MariaDB thông thường, `internal/frameworks/nextjs/config.go` cho stack chỉ Node không DB.

Nếu `whimsy` là bản fork gần giống một framework đã có (cùng stack runtime, giá trị mặc định gần như giống hệt — như Mage-OS với Magento 2, hay OpenMage với Magento 1), đừng copy nguyên literal: thêm một constructor dùng chung vào `internal/engine/bootstrap` (xem `bootstrap.BuildMagento2FamilyConfig`/`BuildMagento1FamilyConfig` trong `internal/engine/bootstrap/magento_family_config.go`) tham số hóa theo đúng vài field thực sự khác nhau, rồi gọi nó từ `config.go` của cả hai framework.

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

Quy tắc tương tự `config.go` ở trên: nếu `whimsy` là bản fork gần giống một framework đã có, hãy tham chiếu tới một giá trị dùng chung trong `internal/engine/bootstrap` (xem `bootstrap.Magento2FamilyManifest`/`Magento1FamilyManifest` trong `internal/engine/bootstrap/magento_family_manifest.go`) thay vì copy nguyên literal.

### 3. Blueprint compose — `internal/blueprints/files/whimsy/`

Một file `services.yml` (đoạn Docker Compose) được render qua Go template — copy theo ví dụ gần nhất (`internal/blueprints/files/nextjs/services.yml` cho runtime Node, `internal/blueprints/files/cakephp/` cho PHP). Không phải framework nào cũng cần thư mục riêng: Mage-OS tái dùng thẳng blueprint compose/nginx/Varnish của Magento 2 (xem `varnishTemplateFramework` trong `internal/engine/render.go`) vì nó là bản fork drop-in với cùng hình dạng runtime.

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

`config` và `manifest` chính là 2 biến cấp package từ bước 1 và 2 — không cần lookup theo tên, vì `Definition()` nằm cùng package với nơi khai báo chúng. `NewWhimsyBootstrap` là constructor cục bộ từ `bootstrap.go` ở bước 4 (không có tiền tố `bootstrap.` — bộ bootstrapper giờ nằm ngay trong package `whimsy`, không phải `internal/engine/bootstrap`). Chỉ đặt `BaseURLManager` nếu framework cần rewrite base-URL riêng cho `govard tunnel` (đa số không cần — `tunnel.NoopManager` mặc định là no-op, đúng cho bất kỳ framework nào không tự lưu base URL trong database hay file config).

### 6. Đăng ký — không cần sửa gì

Việc đăng ký được sinh tự động, không còn duy trì thủ công: chạy `make generate` (hoặc `go generate ./internal/frameworks/...`) và `internal/frameworks/all_generated.go` sẽ tự nhận package `whimsy` mới — không cần thêm import hay dòng `Register()` nào bằng tay. `make build` và `make test` đã tự chạy bước này cho bạn, nên trên thực tế bạn chỉ cần chạy tay khi muốn xem trước file sinh ra trước khi build.

**Vị trí vẫn có ý nghĩa cho detection**: `DetectFramework` duyệt qua các framework theo đúng thứ tự đăng ký và trả về kết quả khớp đầu tiên, nên nếu chữ ký nhận diện của framework mới có thể trùng với framework khác đã đăng ký, nó cần được đăng ký ở đúng vị trí tương đối. Thứ tự mặc định là theo alphabet; trường hợp ngoại lệ duy nhất đã biết (Emdash phải đăng ký trước Next.js, giữ đúng thứ tự ưu tiên phân giải xung đột của detector cũ) được khai báo trong map `PriorityOverrides` của `internal/frameworks/gen/generator/order.go` — thêm một entry ở đó, không phải trong `all_generated.go`, nếu chữ ký nhận diện của `whimsy` cũng mơ hồ tương tự với một framework đã có.

### 7. Orchestration fresh-install / clone — `internal/cmd`

Phần lớn chỗ này đã lên registry; chỉ còn đúng một case switch sót lại (xem "Những gì chưa nằm trên registry" ở trên):

- Nếu `whimsy` khớp khuôn chung `CreateProject → Install → govard config auto`, thêm file `internal/frameworks/whimsy/freshinstall.go` với một hàm `freshInstall` chỉ đơn giản delegate sang `bootstrap.GenericFreshInstall(NewWhimsyBootstrap(opts), projectDir, helpers)` — copy theo `internal/frameworks/cakephp/freshinstall.go` — rồi gắn vào qua field `FreshInstall` (cùng `FreshInstallNeedsDB`/`FreshInstallNeedsDomain`) trên `Definition()`. `runBootstrapFrameworkFreshInstall` sẽ tự nhận, không cần sửa switch nào. Nếu service compose của `whimsy` không thể khởi động được với một project rỗng/chưa migrate (nên `FreshInstall` phải tự bật môi trường lên trước khi chạy `Install()`/migrate — Django là framework duy nhất cần điều này hiện nay), hãy đặt thêm `FreshInstallManagesOwnEnvUp: true` trên `Definition()` để `bootstrapCmd.RunE` bỏ qua bước `env up` dư thừa của chính nó sau đó.
- Nếu cần các bước riêng, viết orchestration đó ngay trong `internal/frameworks/whimsy/freshinstall.go` — vẫn gắn vào theo đúng cách qua field `FreshInstall` của `Definition()`, chỉ là không delegate sang `bootstrap.GenericFreshInstall`; copy `internal/frameworks/openmage/freshinstall.go` hoặc `internal/frameworks/django/freshinstall.go` làm khung ban đầu (file của Django cho thấy cách override `Options.Runner` thành một runner `CmdHelpers` không phải PHP, và cách dùng `Options.SkipUp`/`bootstrap.ErrFreshInstallSkipUp`). Chỉ nên quay lại dùng `case "whimsy":` trong switch của `runBootstrapFrameworkFreshInstall` nếu orchestration đó thực sự không thể diễn đạt được bằng một hàm `FreshInstall` — mọi framework đã đăng ký đến nay đều tránh được việc này, trừ Magento 1, vì fresh install của nó không được hỗ trợ theo thiết kế và vẫn trả lỗi ngay từ switch đó.
- Nếu nó hỗ trợ quy trình remote/clone (`SupportsBootstrap: true`, không chỉ fresh-install), nó tự động được `bootstrap_remote.go`'s post-clone dispatch (`bootstrapPostCloneDefinition`) nhận diện — không cần sửa switch nào ở đó trừ khi nó thuộc nhóm Magento (bị `bootstrapPostCloneDefinition` loại trừ qua `engine.IsMagento2Family`, vì bước pre-configure/post-clone thật của nó đi qua field `PreConfigureHook`/`PostCloneHook` thay thế - xem đoạn ngay dưới đây).

Nếu bước post-clone của `whimsy` trong clone workflow cần dùng `*cobra.Command` (để chạy `govard tool <x>`) mà interface method thuần `FrameworkBootstrap.PostClone(projectDir)` không diễn đạt được, hoặc cần chạy trước `govard config auto` thay vì sau bước post-clone dispatch chung, hãy set `PreConfigureHook`/`PostCloneHook` trên `Definition()` — cùng kiểu chữ ký `func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error` như `FreshInstall`. Copy `bootstrap.MagentoFamilyPreConfigure`/`MagentoFamilyPostClone` (`internal/engine/bootstrap/magento_family.go`) làm khung ban đầu, và thêm bất kỳ closure `CmdHelpers` mới nào hook của bạn cần vào `internal/engine/bootstrap/base.go` và dispatcher trong `internal/cmd/bootstrap_remote.go` — cả hai field này là hạ tầng chung cho mọi framework, không riêng gì Magento, dù Magento 2/Mage-OS là hai framework đầu tiên dùng đến chúng.

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
