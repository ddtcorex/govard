---
title: Ứng dụng Desktop Govard
description: Govard Desktop là GUI dựa trên Wails, dùng chung engine lõi với CLI, có live logs, quick actions và dashboard dự án.
---

# Ứng dụng Desktop (Desktop App)

Govard Desktop là ứng dụng giao diện (GUI) viết bằng Wails, tái sử dụng cùng một core engine với phiên bản CLI.

---

## Các chế độ khởi chạy (Launch Modes)

```bash
govard desktop              # Khởi chạy ứng dụng desktop đã được build
govard desktop --dev        # Chạy ở chế độ phát triển Wails dev (live backend)
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

Yêu cầu: Go, Node.js 24+, pnpm và Wails v2 CLI. Giao diện desktop được Vite
đóng gói vào `desktop/frontend/dist` và được nhúng thẳng vào binary Go.

```bash
make frontend                      # Vite build vào desktop/frontend/dist
DISPLAY=:1 govard desktop --dev    # wails dev: Vite HMR + build lại backend Go
```

`make frontend` là bắt buộc: bản build desktop mà bỏ qua bước này sẽ nhúng
`dist/` rỗng và chỉ hiện cửa sổ trắng. Mọi đường build desktop tự động (CI,
goreleaser, `scripts/build-macos-pkg.sh`, `install.sh --source`) đều chạy bước
này trước.

Vite dev server lắng nghe tại `http://localhost:5173`; chế độ Wails dev proxy
server đó và đồng thời mở backend đã biên dịch tại:

```
http://localhost:34115
```

`http://localhost:34115` là cách kiểm thử trên trình duyệt được khuyên dùng vì
cầu nối Go backend (Go backend bridge) hoạt động trực tiếp để tải dữ liệu dự án
thực tế. Mở trực tiếp Vite server hoặc thư mục `dist/` sẽ hiển thị đúng khung
giao diện đó với dữ liệu mock kèm cảnh báo "Desktop bridge not available", đủ để
kiểm tra style và layout.

---

## Cấu trúc thư mục Frontend

| File | Mục đích |
| :--- | :--- |
| `desktop/frontend/index.html` | Điểm vào HTML chính |
| `desktop/frontend/main.js` | Khởi tạo, lắng nghe sự kiện, quản lý tab và state |
| `desktop/frontend/services/bridge.js` | Cầu nối gọi RPC tới Go backend của Wails; là module duy nhất được phép gọi Go |
| `desktop/frontend/services/events.js` | Đăng ký sự kiện từ backend; là module duy nhất được phép dùng `window.runtime` |
| `desktop/frontend/types/wails-v2.d.ts` | Khai báo shape của các global do Wails v2 inject |
| `desktop/frontend/state/store.js` | State UI dùng chung (dự án đang chọn, bộ lọc) |
| `desktop/frontend/modules/` | Các module tính năng (dashboard, logs, remotes, v.v.) |
| `desktop/frontend/ui/toast.js` | Hệ thống hiển thị thông báo toast |
| `desktop/frontend/utils/dom.js` | Các helper xử lý DOM dùng chung |

### Hành vi ở chế độ test (Test Mode)

| Cách thức truy cập | Trạng thái Backend | Dữ liệu hiển thị |
| :--- | :--- | :--- |
| Wails dev (`localhost:34115`) | Hoạt động đầy đủ cầu nối backend | Dữ liệu dự án thực tế |
| Vite dev (`localhost:5173`) hoặc thư mục `dist/` | Cầu nối không khả dụng | Dữ liệu mock fallback + toast cảnh báo |

Một test Go (`tests/desktop_frontend_bridge_guard_test.go`) sẽ fail nếu bất kỳ
file frontend nào ngoài `services/bridge.js`, `services/events.js` và
`types/wails-v2.d.ts` chạm vào `window.go`, `window.runtime` hoặc
`desktopBridge.runtime`. Hai module này được type bằng JSDoc kèm `// @ts-check`,
nên `pnpm typecheck` kiểm tra đúng những shape mà hai module đó khai báo. Nó chưa
xác minh được tên method phía Go: `window.go.desktop.App` đang khai báo bằng
index signature nên tên thuộc tính nào cũng hợp lệ. Bindings sinh tự động - thứ
đưa phía Go trở thành nguồn sự thật - sẽ đến cùng đợt chuyển sang Wails 3.

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