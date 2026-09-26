---
title: Ứng dụng Desktop Govard
description: Govard Desktop là GUI dựa trên Wails 3, dùng chung engine lõi với CLI, có live logs, quick actions, system tray và dashboard dự án.
---

# Ứng dụng Desktop (Desktop App)

Govard Desktop là ứng dụng giao diện (GUI) viết bằng Wails 3, tái sử dụng cùng một core engine với phiên bản CLI. Ứng dụng chạy trên GTK 4 và WebKitGTK 6.0 (có sẵn từ Ubuntu 24.04+ và Debian 13+); Ubuntu 22.04 và Debian 12 chỉ cài được CLI.

---

## Các chế độ khởi chạy (Launch Modes)

```bash
govard desktop              # Khởi chạy ứng dụng desktop đã được build
govard desktop --dev        # Vite dev server kèm build lại Go
govard desktop --background # Khởi động ẩn, tái sử dụng instance khi mở lại
```

| Chế độ | Mô tả |
| :--- | :--- |
| `govard desktop` | Khởi chạy tiêu chuẩn — sử dụng binary đã build |
| `govard desktop --dev` | Chế độ phát triển — live Go backend, hỗ trợ hot reload frontend |
| `govard desktop --background` | Tiến trình chạy ngầm — giữ ứng dụng hoạt động ngay cả khi đóng cửa sổ |

---

## Giao diện hiện tại

Ứng dụng Desktop tập trung vào các thao tác quản lý cốt lõi:

| Tính năng | Mô tả |
| :--- | :--- |
| **Bảng điều khiển môi trường** | Start/stop/open/delete — bao gồm việc phát hiện các dự án Docker mồ côi |
| **Workspace dự án** | Danh sách các môi trường, các thao tác nhanh, quy trình onboard |
| **Thao tác nhanh** | PHPMyAdmin, bật/tắt Xdebug, kiểm tra sức khỏe, Mailpit, DB client |
| **Tab Remotes** | Quy trình thêm/kiểm tra/mở/lập kế hoạch đồng bộ cho môi trường remote |
| **Giám sát tài nguyên** | CPU, RAM, mạng, cảnh báo OOM |
| **Nhật ký (Logs)** | Lọc logs theo dịch vụ, mức độ nghiêm trọng, tìm kiếm text, stream trực tiếp |
| **Shell Launcher** | Chọn container dịch vụ, user kết nối và loại shell tương ứng |
| **Thông báo hệ thống** | Hiển thị cảnh báo khi các thao tác thành công hoặc thất bại |
| **Bảng Cài đặt** | Thay đổi theme giao diện, cấu hình proxy, trình duyệt ưa thích, DB client |

> Các thao tác khởi động/dừng/tải môi trường và quản lý dịch vụ toàn cục trên Desktop đều gọi trực tiếp tới tầng lệnh CLI của Govard (`govard up`, `govard env ...`, `govard svc ...`), giúp hành vi của ứng dụng Desktop luôn đồng bộ với các cập nhật mới nhất của CLI.

---

## Phím tắt (Keyboard Shortcuts)

| Phím tắt | Thao tác |
| :--- | :--- |
| `Ctrl+,` / `Cmd+,` | Mở Cài đặt |
| `Esc` | Đóng Cài đặt |

---

## Các thao tác Remote trên Desktop

| Thao tác | Hành vi |
| :--- | :--- |
| Mở Database (Remote) | Gọi lệnh `govard open db -e <remote> --client` |
| Mở terminal SSH (Remote) | Ưu tiên các terminal gốc của Linux, fallback về giao thức `ssh://` |
| Mở SFTP (Remote) | Ưu tiên ứng dụng FileZilla, fallback về giao thức `sftp://` |

Đối với phương thức cấu hình `auth.method: ssh-agent`, ứng dụng Desktop tái sử dụng `SSH_AUTH_SOCK` và thăm dò socket tại `/run/user/<uid>/keyring/ssh` trên môi trường Linux.

### Mở Cơ sở dữ liệu local

- Xác định host và port docker được publish trước.
- Fallback về PHPMyAdmin nếu trình kết nối DB client được cấu hình bị thất bại.

---

## Cấu hình ưu tiên của Desktop (Desktop Preferences)

Các cài đặt cấu hình ưu tiên được lưu trữ tại file:

```
~/.govard/desktop-preferences.json
```

Các cấu hình ưu tiên hiện được ghi nhớ:
- Theme giao diện (light/dark)
- Proxy target
- Trình duyệt ưa thích
- Trình kết nối database client ưa thích

---

## Chế độ phát triển (Dev Mode)

Yêu cầu: Go, Node.js 24+, pnpm, và trên Linux cần `libgtk-4-dev` cùng
`libwebkitgtk-6.0-dev`. Giao diện desktop được Vite đóng gói vào
`desktop/frontend/dist` và được nhúng thẳng vào binary Go.

```bash
make frontend                      # Vite build vào desktop/frontend/dist
DISPLAY=:1 govard desktop --dev    # Vite HMR + build lại backend Go
make bindings                      # sinh lại JS bindings từ các Go service
```

`govard desktop --dev` chạy Vite tại `http://localhost:5173` rồi chạy ứng dụng
với `FRONTEND_DEVSERVER_URL` trỏ vào đó; Wails chỉ proxy dev server khi build
không có tag `production`. Không còn bước Wails CLI và không còn `wails.json`.

`make frontend` là bắt buộc: bản build desktop mà bỏ qua bước này sẽ nhúng
`dist/` rỗng và chỉ hiện cửa sổ trắng. Mọi đường build desktop tự động (CI,
goreleaser, `scripts/build-macos-pkg.sh`, `install.sh --source`) đều chạy bước
này trước.

Mở trực tiếp Vite server (`http://localhost:5173`) hoặc thư mục `dist/` sẽ hiển
thị đúng khung giao diện đó với dữ liệu mock kèm cảnh báo "Desktop bridge not
available", đủ để kiểm tra style và layout; muốn có dữ liệu dự án thực tế thì
phải mở trong cửa sổ ứng dụng.

---

## Chế độ Preview và Behaviour Test

Chế độ preview chạy chính `main.js` trong Chrome thường, mọi lời gọi binding Go
đều được trả lời từ fixture JSON, nên việc làm giao diện và các kịch bản tự động
không cần backend Go hay cửa sổ ứng dụng.

```bash
pnpm --dir desktop/frontend dev    # Vite tại http://localhost:5173
# sau đó mở http://localhost:5173/preview.html
```

`preview.html` là bản sao của `index.html`, chỉ khác ở script bootstrap của
preview và container của demo island. Mọi thay đổi markup trong `index.html`
phải được chép sang `preview.html`; `TestPreviewHTMLMirrorsIndexHTML` sẽ báo lỗi
khi hai file lệch nhau. Không có gì trong `desktop/frontend/preview/` đi vào bản
build production.

Trang cung cấp một bề mặt điều khiển duy nhất, `window.__govardPreview`:

| Thành phần | Mục đích |
| :--- | :--- |
| `reset()` | Xóa các fixture đã cài và danh sách lời gọi đã ghi |
| `installFixtures(name)` | Nạp `desktop/frontend/preview/fixtures/<name>.json` |
| `pushEvent(name, data)` | Phát một sự kiện backend tới các subscription của ứng dụng |
| `getCalls()` | Trả về các lời gọi binding mà ứng dụng đã thực hiện |

Một file fixture là mảng JSON gồm các phần tử `{ "service", "method", "args",
"result" }`; lời gọi lỗi dùng `"error"` (chuỗi thông báo) thay cho `"result"`.
Giá trị dùng đúng tên trường JSON mà các Go service trả về (ví dụ `cpuUsage`,
không phải `CPUUsage`). Một lời gọi khớp với phần tử có cùng service, method và
tham số; nếu không có thì lấy phần tử đầu tiên cùng service và method, cuối cùng
là giá trị mặc định sinh sẵn trong `preview/route-defaults.generated.js`.

Chế độ ghi (record mode) thu fixture thật từ ứng dụng đang chạy:

```bash
GOVARD_PREVIEW_RECORD=1 go run ./cmd/govard desktop --dev
```

Mọi lời gọi binding vẫn đi tới backend thật và đồng thời được ghi thêm vào
`preview/fixtures/<mode>.json`, trong đó `<mode>` là chế độ sidebar hiện tại.
Hãy đổi tên file theo module mà nó thuộc về trước khi commit.

Behaviour test điều khiển `preview.html` qua raw CDP trong Chrome headless, mỗi
kịch bản dùng một Vite server tạm riêng:

```bash
make test-frontend-behaviour                                     # tự tìm google-chrome hoặc chromium
make test-frontend-behaviour CHROME_BIN=/opt/google/chrome/chrome
```

Các kịch bản nằm trong `tests/frontend/behaviour/*.behaviour.test.mjs` và CI
chạy chúng sau `make test-frontend`.

Các React island được mount qua `mountIsland(containerId, element)` trong
`desktop/frontend/islands/mount.js`. Hàm trả về `{ unmount() }`, hoặc `null` khi
không có container, để `main.js` vẫn khởi động được trên trang không có nó.

---

## Cấu trúc thư mục Frontend

| File | Mục đích |
| :--- | :--- |
| `desktop/frontend/index.html` | Điểm vào HTML chính |
| `desktop/frontend/main.js` | Khởi tạo, lắng nghe sự kiện, quản lý tab và state |
| `desktop/frontend/services/bridge.js` | Cầu nối gọi RPC tới Go backend của Wails; là module duy nhất được phép gọi Go |
| `desktop/frontend/services/events.js` | Đăng ký sự kiện từ backend; là module duy nhất được phép dùng `@wailsio/runtime` |
| `desktop/frontend/bindings/` | Sinh tự động từ các Go service bằng `make bindings`; được commit, không sửa tay |
| `desktop/frontend/state/store.js` | State UI dùng chung (dự án đang chọn, bộ lọc) |
| `desktop/frontend/modules/` | Các module tính năng (dashboard, logs, remotes, v.v.) |
| `desktop/frontend/ui/toast.js` | Hệ thống hiển thị thông báo toast |
| `desktop/frontend/utils/dom.js` | Các helper xử lý DOM dùng chung |

### Hành vi ở chế độ test (Test Mode)

| Cách thức truy cập | Trạng thái Backend | Dữ liệu hiển thị |
| :--- | :--- | :--- |
| Cửa sổ ứng dụng | Bindings hoạt động | Dữ liệu dự án thực tế |
| Vite dev (`localhost:5173`) hoặc thư mục `dist/` | Bindings không khả dụng | Dữ liệu mock fallback + toast cảnh báo |

Một test Go (`tests/desktop_frontend_bridge_guard_test.go`) sẽ fail nếu bất kỳ
file frontend nào ngoài `services/bridge.js` và `services/events.js` chạm vào
`window.go`, `window.runtime`, `desktopBridge.runtime`, `@wailsio/runtime` hoặc
`bindings/`. Hai module này được type bằng JSDoc kèm `// @ts-check`, nên
`pnpm typecheck` kiểm tra đúng những shape chúng khai báo; thêm một test Go thứ
hai (`tests/desktop_bindings_contract_test.go`) fail khi một route trong
`bridge.js` không còn khớp với bindings đã sinh, nhờ đó việc đổi tên method phía
Go bị CI chặn thay vì để người dùng phát hiện.

### Đóng cửa sổ

Đóng cửa sổ sẽ giữ Govard chạy trong tray khi bật **Run in background** (mặc định)
và máy có tray host; nếu không thì ứng dụng thoát, để không bao giờ rơi vào tình
trạng cửa sổ bị ẩn mà không có cách mở lại. GNOME nguyên bản không có tray host:
cần bật extension AppIndicator, nếu không thì đóng cửa sổ là thoát. Lệnh
`govard desktop doctor` cho biết máy đang ở trường hợp nào, và trang Settings sẽ
hiển thị cảnh báo khi không có tray. Menu tray cho phép hiện/ẩn cửa sổ, liệt kê
dự án (dự án đang chạy lên trước), start/stop, mở trên trình duyệt và thoát.

### Hỗ trợ nền tảng

| Nền tảng | Ứng dụng desktop | Ghi chú |
| :--- | :--- | :--- |
| Linux, Ubuntu 24.04+ / Debian 13+ | Có | GTK 4 và WebKitGTK 6.0 |
| Linux, Ubuntu 22.04 / Debian 12 | Không | Chỉ cài CLI; không có WebKitGTK 6.0 |
| macOS | Chưa | Gói chỉ có CLI; `govard self-update` giữ nguyên binary desktop đang cài |

### Danh tính launcher trên Linux

Gói deb cài launcher với tên `io.github.ddtcorex.govard.desktop`, và ứng dụng
desktop công bố đúng chuỗi đó làm GTK application id (`DesktopApplicationID`
trong `internal/desktop/launch.go`). Trên Wayland, GNOME gom cửa sổ vào launcher
theo id này: nó tra file desktop `"<application id>.desktop"` và **không** dùng
`StartupWMClass` cho surface Wayland. Cửa sổ có id không khớp launcher nào sẽ
thành window-backed app, nên dock hiện thêm một icon thứ hai với tên khác bên
cạnh icon đã ghim. Hằng số, tên file trong `packaging/linux/`, `StartupWMClass`
và mục GoReleaser phải luôn khớp nhau — `tests/linux_desktop_entry_test.go` sẽ
fail nếu chúng lệch.

---

## Ghi chú về Kiến trúc (Architecture Notes)

Ứng dụng Desktop được thiết kế tập trung tối đa vào các quy trình thao tác vận hành:

- Điểm vào desktop: `cmd/govard-desktop`
- Wails bindings: `internal/desktop`
- Khung giao diện frontend: `desktop/frontend/index.html`
- Khởi tạo & Lắng nghe sự kiện: `desktop/frontend/main.js`
- Cầu nối gọi backend: `desktop/frontend/services/bridge.js`
- Quản lý State: `desktop/frontend/state/store.js`
- Module tính năng: `desktop/frontend/modules/`

Để hiểu sâu hơn về kiến trúc hệ thống, xem thêm tài liệu [Kiến trúc](/vi/developer/architecture).

---

[SSL và Tên miền](/vi/workflows/ssl-and-domains) | [Kiến trúc](/vi/developer/architecture)