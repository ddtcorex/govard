---
title: Đóng góp cho Govard
description: Cách thiết lập môi trường phát triển, tuân thủ chuẩn code, và gửi thay đổi cho dự án Govard.
---

# Đóng góp (Contributing)

Tài liệu này hướng dẫn quy trình đóng góp mã nguồn cho dự án Govard.

## Thêm trang tài liệu mới

`sitemap.xml`, canonical link, hreflang và thẻ Open Graph đều được tự động sinh khi build từ `docs/.vitepress/seo.ts` — không bao giờ sửa tay `sitemap.xml`. Mọi trang mới trong `docs/` phải có `title` và `description` riêng trong frontmatter; trang nào thiếu sẽ dùng mô tả chung của site, làm giảm khả năng xếp hạng tìm kiếm.

---

## Yêu cầu về bộ công cụ phát triển (Toolchain Requirements)

| Công cụ | Phiên bản yêu cầu | Lệnh kiểm tra |
| :--- | :--- | :--- |
| Go | `1.25+` | `go version` |
| Node.js | `20+` | `node --version` |
| Docker Engine + Compose | Mới nhất | `docker --version` |
| Wails | `v2.11+` (chỉ cho desktop) | `wails version` |
| golangci-lint | `v2.11+` | `golangci-lint --version` |

```bash
# Kiểm tra nhanh tất cả công cụ
go version
node --version
wails version
docker --version
golangci-lint --version
```

---

## Bản đồ thư mục mã nguồn (Repository Map)

```
cmd/govard/main.go           Điểm vào của CLI
cmd/govard-desktop/          Điểm vào của Desktop (được build bởi Wails)
desktop/                     Mã nguồn Desktop (Go backend + vanilla JS frontend)
internal/cmd/                Triển khai các lệnh CLI bằng thư viện Cobra
internal/engine/             Logic điều phối container, cấu hình và blueprint
internal/engine/bootstrap/   Các quy trình bootstrap framework
internal/engine/remote/      Các helper quản lý sync/deploy/SSH từ xa
internal/proxy/              Các helper định tuyến và TLS của Caddy proxy
internal/updater/            Logic thông báo và kiểm tra cập nhật phiên bản
internal/ui/                 Các helper kết xuất terminal
tests/                       Các unit/contract tests
tests/integration/           Các bộ tích hợp kiểm thử framework (nặng hơn)
tests/frontend/              Bộ kiểm thử Node cho frontend desktop
install.sh                   Script cài đặt hợp nhất
scripts/                     Script hỗ trợ build (macOS pkg, v.v.)
.goreleaser.yml              Cấu hình phát hành release artifact
.github/workflows/           Tự động hóa CI/release/bảo mật trên GitHub
```

---

## Các lệnh biên dịch (Build Commands)

```bash
make build           # Build Govard cho nền tảng hiện tại của bạn
make install         # Build + cài đặt trực tiếp vào hệ thống PATH
make install-release # Cài đặt binary phát hành chính thức

# Lệnh build trực tiếp
go build -o govard cmd/govard/main.go
```

---

## Cắt bản Beta

Bản beta dùng tag dạng `vX.Y.Z-beta.N` (ví dụ `v1.60.0-beta.1`), tag thẳng
từ commit hiện tại — không cần bump version ở `root.go`, `app.go`,
`package.json`, `wails.json`, hay thêm entry vào `CHANGELOG.md`. Push tag sẽ
kích hoạt cùng pipeline `.github/workflows/release.yml` như bản stable;
`prerelease: auto` trong `.goreleaser.yml` sẽ đánh dấu GitHub Release đó là
prerelease, nên nó sẽ không xuất hiện qua `GET /releases/latest` và không
tự động đến tay user đang ở channel stable. User chủ động opt-in bằng
`govard self-update --channel beta`.

---

## Các lệnh kiểm thử (Test Commands)

### Các lệnh Makefile khuyên dùng

```bash
make test            # Bộ kiểm thử đầy đủ: lint + fmt-check + vet + frontend + unit + integration
make test-unit       # Chỉ chạy các unit test của Go
make test-integration # Chạy tích hợp kiểm thử (yêu cầu Docker)
make vet             # Chạy go vet
make fmt             # Chạy gofmt ./...
```

### Các lệnh tương đương trên CI (CI-Equivalent)

```bash
make lint            # Chạy golangci-lint (khớp với version trên CI)
make fmt-check       # Kiểm tra xem có file Go nào cần chạy gofmt -s
make vet             # Chạy go vet
make test-unit       # Chạy Go unit tests
make test-frontend   # Chạy các kiểm thử Node.js frontend
make test-integration-ci # Chạy các tích hợp kiểm thử song song (hành vi của CI)
```

### Lệnh chạy trực tiếp

```bash
go test ./...
go test ./tests/... -v
go test -tags integration ./tests/integration/... -v -timeout 30m
```

---

## Phát triển Desktop (Desktop Development)

Khởi chạy chế độ phát triển Wails dev thông qua CLI wrapper:

```bash
DISPLAY=:1 govard desktop --dev
```

Để kiểm thử giao diện trực tiếp trên trình duyệt, truy cập địa chỉ:

```
http://localhost:34115
```

Đường dẫn này sẽ kết nối trực tiếp với Go backend thực tế để tải dữ liệu dự án thật.

---

## Cấu trúc và Quy ước kiểm thử (Test Layout & Conventions)

| Thư mục | Mục đích |
| :--- | :--- |
| `tests/` | Package `tests` — chứa hầu hết các kiểm thử đơn vị (unit tests) |
| `tests/fixtures/` | Các file fixture dùng chung cho kiểm thử |
| `tests/integration/` | Các dự án kiểm thử tích hợp thực tế theo từng framework |
| `tests/frontend/` | Bộ kiểm thử Node test runner |

### Các quy tắc vệ sinh mã kiểm thử (Test Hygiene Rules)

- **Đảm bảo tính cô lập (hermetic)** — không phụ thuộc vào dự án local của user, không phụ thuộc vào trạng thái container thực tế.
- **Tên fixture trung lập** — sử dụng `sample-project`, tránh các tên mang tính đặc thù như `magento2-test-instance`.
- **Ưu tiên Mock thay vì gọi mạng thật** — inject các HTTP transport mock; sử dụng fake `RoundTripper`.
- **Cô lập thư mục `GOVARD_HOME_DIR`** — các kiểm thử cần trạng thái runtime phải sử dụng thiết lập `TestMain` riêng.
- **Kiểm soát dịch vụ ngoài** — viết các kiểm thử có điều kiện rõ ràng để bỏ qua (skip) nếu không có dịch vụ thật.

### Export mã nội bộ phục vụ kiểm thử

Khi một kiểm thử cần truy cập logic nội bộ (unexported) từ gói `internal/cmd`:

1. Giữ các helper ở phạm vi không export (nội bộ) trong code production.
2. Thêm các wrapper được export với hậu tố `ForTest`.
3. Gọi các wrapper này từ package `tests/`.

```go
// internal/cmd/thing.go
func buildThing(...) { ... }  // unexported (nội bộ)

// Export phục vụ kiểm thử (chỉ thêm khi thực sự cần)
func BuildThingForTest(...) { return buildThing(...) }

// tests/thing_test.go
result := cmd.BuildThingForTest(...)
```

---

## Kiến trúc CLI

Khi thêm mới hoặc sửa đổi một lệnh CLI:

1. Định nghĩa cấu trúc lệnh tại `internal/cmd/<area>.go`.
2. Đăng ký lệnh với `rootCmd.AddCommand(...)` (hoặc nhóm lệnh con tương ứng).
3. Đảm bảo các cờ (flags) được định nghĩa rõ ràng kèm trợ giúp cụ thể.
4. Trả về lỗi kèm ngữ cảnh rõ ràng: `fmt.Errorf("operation: %w", err)`.
5. Bổ sung/điều chỉnh kiểm thử trong thư mục `tests/`.
6. Cập nhật tài liệu hướng dẫn nếu lệnh hoặc cờ đó thay đổi giao diện sử dụng với user.

---

## Tiêu chuẩn viết code (Coding Standards)

| Quy tắc | Ghi chú |
| :--- | :--- |
| **Chạy `gofmt` sau mỗi chỉnh sửa** | Chạy `gofmt -s -w` trên các file Go có thay đổi |
| **Chỉ sử dụng mã ASCII** | Trừ khi file đó đã yêu cầu ký tự Unicode từ trước |
| **Helper nhỏ gọn và thuần túy (pure)** | Áp dụng cho các logic parse hoặc định dạng (formatting) |
| **Phân nhánh hệ điều hành rõ ràng** | Sử dụng `runtime.GOOS`, `runtime.GOARCH` |
| **Không nuốt lỗi (No swallowed errors)** | Các luồng quan trọng (mạng, file, process) phải báo lỗi cụ thể |
| **Giữ vững phong cách pterm UX** | Đồng bộ phong cách hiển thị và trợ giúp lệnh |

---

## Hướng dẫn về Bảo mật (Security Guidelines)

- Không thêm dependencies mới nếu không thực sự đem lại lợi ích rõ rệt — ưu tiên thư viện chuẩn (stdlib) của Go.
- Không bao giờ log lại thông tin nhạy cảm, token, private key, hay mật khẩu DB.
- Đối với các lệnh tương tác Remote/SSH/DB: cấu hình các giá trị mặc định an toàn, yêu cầu xác nhận rõ ràng với các lệnh ghi dữ liệu.

---

## Quy tắc đóng góp

Trước khi đánh dấu công việc hoàn tất:

1. Chạy các kiểm thử liên quan đến vùng code thay đổi.
2. Chạy `gofmt -s -l .` — kết quả trả về phải rỗng đối với các file Go có sửa đổi.
3. Cập nhật các tài liệu hướng dẫn trong `docs/*.md` tương ứng khi hành vi thay đổi.
4. Kiểm tra `git status` để tránh commit nhầm các file ngoài ý muốn.
5. Đảm bảo thông tin trợ giúp/cờ của lệnh hoạt động nhất quán.

---

## Cửa ải chất lượng của CI (CI Quality Gates)

| Công việc | Lệnh chạy | Mô tả |
| :--- | :--- | :--- |
| Kiểm tra chất lượng | `make lint fmt-check vet` | golangci-lint + gofmt + go vet |
| Chạy full test | `make test` | Lint + format + vet + frontend + unit tests |
| Kiểm thử tích hợp | `make test-integration` | Build binary + chạy tích hợp Docker |
| Biên dịch thử | `make build` | Xác minh dự án biên dịch thành công |

Hệ thống CI theo dõi các nhánh `main`, `master`, và `develop`. Nhánh mặc định là `master`.

---

## Quy tắc cập nhật tài liệu

Cập nhật `README.md` khi thay đổi ảnh hưởng đến:
- Quy trình cài đặt hoặc cập nhật.
- Tên lệnh hoặc các cờ tham số.
- Cách thức sử dụng bản phát hành.

Cập nhật tài liệu chuyên biệt tại `docs/*.md` khi thay đổi ảnh hưởng đến:
- Tên lệnh, tên viết tắt, hoặc cờ tham số.
- Hành vi hoặc phân tầng của cấu hình.
- Các quy trình remote/sync/DB.
- Khả năng hỗ trợ framework hoặc cấu hình runtime mặc định.
- Hành vi trên Desktop hoặc quy trình kiểm thử.

**Nếu hành vi hệ thống thay đổi nhưng tài liệu không được cập nhật → coi như công việc chưa hoàn thành.**

---

## Danh sách tự kiểm tra (Pre-Completion Checklist)

- [ ] Chạy `go test` trên vùng ảnh hưởng thành công.
- [ ] Chạy `gofmt -s -l .` không hiển thị file Go nào có sửa đổi.
- [ ] Thông tin trợ giúp/cờ của lệnh hoạt động nhất quán.
- [ ] Đã cập nhật file `README.md` nếu cần.
- [ ] Đã cập nhật các tài liệu liên quan tại `docs/*.md`.
- [ ] Đã kiểm tra `git status` tránh commit nhầm file.

---

[Kiến trúc](/vi/developer/architecture) | [FAQ](/vi/more/faq)