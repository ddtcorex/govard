---
title: Kiến trúc Govard
description: Tổng quan kiến trúc nội bộ của Govard — engine Go, render blueprint, sinh Docker Compose, và tính tương đồng giữa CLI/desktop.
---

# Kiến trúc (Architecture)

Tài liệu này mô tả chi tiết kiến trúc hệ thống hiện tại của Govard ở mức độ tổng quan.

---

## Các bề mặt ứng dụng (Product Shape)

Govard là công cụ điều phối phát triển local viết bằng Go, cung cấp hai giao diện người dùng:

- **CLI**: Được xây dựng bằng thư viện Cobra.
- **Ứng dụng Desktop**: Được xây dựng bằng framework Wails.

Cả hai giao diện này đều tái sử dụng chung một lõi runtime engine để xử lý nhận diện, render cấu hình, điều phối Docker, tích hợp proxy và các quy trình công việc remote.

---

## Nhân Runtime Core (Core Runtime)

### Tầng CLI

```
cmd/govard/main.go           Điểm vào của CLI
internal/cmd/                Khai báo và triển khai các lệnh CLI (Cobra)
internal/ui/                 Các helper kết xuất terminal (pterm)
```

### Tầng Engine

```
internal/engine/             Logic nhận diện, cấu hình, xử lý blueprint
internal/engine/bootstrap/   Các quy trình bootstrap framework
internal/engine/remote/      Các helper quản lý sync/deploy/SSH từ xa
internal/proxy/              Các helper xử lý định tuyến và TLS của Caddy proxy
internal/updater/            Logic thông báo và kiểm tra cập nhật phiên bản
```

### Quy trình khởi động (Startup Pipeline)

Lệnh `govard env up` tuân thủ các giai đoạn cốt lõi giống nhau trên tất cả các framework:

```
1. Nhận diện ngữ cảnh framework
       ↓
2. Xác thực cấu hình và điều kiện tiên quyết của máy host
       ↓
3. Render file compose tương ứng vào ~/.govard/compose/
       ↓
4. Khởi động các container tương ứng
       ↓
5. Xác minh proxy và cấu hình host wiring
```

---

## Mạng kết nối (Networking)

| Thành phần | Vai trò |
| :--- | :--- |
| **Caddy** | Reverse proxy dùng chung; đóng vai trò điểm cuối (terminate) HTTPS cho tất cả domain `.test` |
| **dnsmasq** | Dịch vụ DNS local; phân giải tất cả domain `*.test` về IP loopback local |
| **Docker networks** | Network PHP/DB riêng của từng dự án + network dùng chung `govard-proxy` |

Proxy tiếp nhận HTTPS và chuyển tiếp traffic vào stack dự án hiện tại.
Đối với container PHP, Govard tự động inject các host alias `.test` đã biết thông qua `host-gateway`, giúp các dự án có thể gọi lẫn nhau thông qua Caddy proxy mà không cần phải kết nối trực tiếp container `php` hay `php-debug` vào chung network proxy.

---

## Cấu hình phân tầng (Configuration Model)

Govard gộp các cấu hình phân tầng trên nền tảng các file blueprint của framework:

```
.govard.yml                  Cấu hình cơ sở (thuộc sở hữu của team, được phép ghi)
   ↓
.govard.<profile>.yml        Ghi đè cấu hình profile (chỉ đọc)
   ↓
.govard.local.yml            Ghi đè cấu hình local của dev (chỉ đọc)
   ↓
.govard.<env>.yml            Ghi đè cấu hình môi trường (chỉ đọc)
```

Điểm cốt lõi trong thiết kế:
- `.govard.yml` là file cấu hình duy nhất được phép ghi tự động từ CLI.
- Các cấu hình runtime mặc định được nhận diện theo framework và theo phiên bản (tùy chọn).
- Các định nghĩa remote, hook và tiện ích mở rộng của dự án nằm tại thư mục `.govard/*`.

Xem thêm tài liệu [Cấu hình](/vi/reference/configuration) để biết chi tiết.

---

## Hỗ trợ Framework

Mỗi framework trong số 13 framework Govard hỗ trợ được đăng ký tại `internal/frameworks/<name>/` dưới dạng một `types.FrameworkDefinition` — một struct duy nhất mang chữ ký nhận diện, dữ liệu runtime/manifest, và các hook dispatch (bootstrap factory, base-URL rewriter cho tunnel, cờ hỗ trợ `govard bootstrap`) của framework đó. `init()` của `internal/frameworks/all.go` đăng ký cả 13 framework vào một registry cấp package (`internal/frameworks/registry.go`), và `internal/frameworks/run.go`/`internal/frameworks/base_url.go` dispatch thông qua registry này thay vì rải rác các `switch framework { ... }` khắp codebase.

Bộ nhận diện (`engine.DetectFramework`) quét file manifest của dự án — composer.json requires, package.json deps, auth.json hosts, chữ ký đường dẫn file — và ánh xạ về cấu hình mặc định của framework (web root, phiên bản PHP/Node, database engine, các dịch vụ cache/search/queue/Varnish tùy chọn), lấy từ `engine.GetFrameworkConfig`/`engine.GetFrameworkManifestConfig` (vẫn là nguồn dữ liệu gốc, được compose vào từng `FrameworkDefinition`).

Magento 2 và bản fork Mage-OS được hỗ trợ sâu sắc nhất: tự động cấu hình, cấu hình mặc định search/cache theo phiên bản, và định tuyến debug riêng biệt.

Xem [Thêm Framework](/vi/developer/adding-a-framework) để biết cấu trúc nội bộ đầy đủ và hướng dẫn từng file để thêm một framework mới.

---

## Kiến trúc Desktop (Desktop Architecture)

Ứng dụng Desktop tập trung vào các thao tác quản lý thông qua frontend dạng vanilla JS dạng module:

```
cmd/govard-desktop/          Điểm vào desktop app
internal/desktop/            Các binding Wails Go
desktop/frontend/            Mã nguồn frontend (nhúng trực tiếp vào binary)
  ├── index.html             File HTML chính
  ├── main.js                Khởi tạo & Lắng nghe sự kiện
  ├── services/bridge.js     Cầu nối RPC gọi backend
  ├── state/store.js         State UI dùng chung
  ├── modules/               Các module tính năng
  ├── ui/                    Toast, thông báo hệ thống
  └── utils/                 Các helper DOM dùng chung
```

Các thao tác trên Desktop gọi trực tiếp tới tầng CLI (ví dụ: `govard up`, `govard svc up`) thay vì bỏ qua nó — đảm bảo hành vi của CLI và Desktop luôn đồng bộ.

---

## Cấu trúc thư mục dự án

```
.
├── cmd/
│   ├── govard/              Điểm vào của CLI
│   └── govard-desktop/      Điểm vào của Desktop (Wails)
├── desktop/                 Mã nguồn giao diện desktop (Wails frontend/config)
├── internal/
│   ├── cmd/                 Khai báo lệnh CLI (Cobra)
│   ├── blueprints/          File compose template cho từng framework
│   ├── engine/              Logic cốt lõi (Docker SDK, nhận diện, rendering)
│   │   └── bootstrap/       Triển khai FrameworkBootstrap cho từng framework
│   ├── frameworks/          Registry framework (mỗi framework một FrameworkDefinition)
│   ├── desktop/             Chất keo liên kết desktop (Wails bindings)
│   ├── proxy/               Helper định tuyến Caddy/proxy và TLS
│   ├── ui/                  Định dạng terminal output (pterm)
│   └── updater/             Kiểm tra cập nhật chạy ngầm
├── docker/                  PHP Dockerfiles và các file build context
├── tests/                   Unit + integration tests
│   ├── fixtures/            Các file fixture test dùng chung
│   └── integration/         Các dự án test integration cho từng framework
├── docs/                    Tài liệu hướng dẫn của dự án
└── scripts/                 Script bổ trợ build (macOS pkg, v.v.)
```

---

## Điểm mở rộng (Extension Points)

| Điểm mở rộng | Cách thức thực hiện |
| :--- | :--- |
| Thêm framework mới | `internal/frameworks/<name>/` — xem [Thêm Framework](/vi/developer/adding-a-framework) |
| Mở rộng lựa chọn runtime | Thao tác trên profile/config engine |
| Thêm các mảnh blueprint compose | Thêm các compose template logic |
| Tiện ích mở rộng dự án | `.govard/commands`, `.govard/hooks`, `.govard/docker-compose.override.yml` |

---

## Các Artifact phát hành (Release Shape)

| Artifact | Mô tả |
| :--- | :--- |
| Bản nén theo nền tảng | Binary CLI build qua GoReleaser (`.tar.gz` / `.zip`) |
| `govard_<version>_linux_<arch>.deb` | Gói cài đặt Linux chứa cả `govard` + `govard-desktop` |
| `govard_<version>_Darwin_<arch>.pkg` | Gói cài đặt macOS chứa cả `govard` + `govard-desktop` |
| `checksums.txt` | File mã băm SHA-256 xác minh cho mọi artifact phát hành |

Lệnh `govard self-update` kiểm tra phiên bản mới, tải gói cài đặt tương ứng với nền tảng hệ điều hành, xác minh mã băm SHA-256 và thực hiện thay thế binary hiện tại.

---

[Ứng dụng Desktop](/vi/workflows/desktop-app) | [Đóng góp](/vi/developer/contributing)