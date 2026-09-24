---
title: Cài đặt Govard trên Linux & macOS
description: Cài đặt Govard qua gói .deb, make install, hoặc self-update. Hướng dẫn các kênh cài đặt và cách tránh xung đột binary.
---

# Cài đặt

Trang này hướng dẫn tất cả các phương pháp cài đặt Govard trên Linux và macOS.

::: warning QUAN TRỌNG
Không nên trộn lẫn các kênh cài đặt trên cùng một máy (ví dụ: `.deb` + `make install` + `self-update` ở các đường dẫn khác nhau). Chỉ sử dụng **một kênh duy nhất** để tránh xung đột binary trong `/usr/bin` và `/usr/local/bin`.
:::

---

## 🚀 Cài đặt một dòng lệnh (Linux/macOS)

Cài đặt phiên bản release mới nhất bằng một lệnh duy nhất:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

Sử dụng `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

### Các tùy chọn cài đặt phổ biến

```bash
# Cài vào ~/.local/bin (không cần sudo)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --local

# Build từ source (tự động cài Go 1.25 nếu cần)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --source

# Chỉ cài CLI (bắt buộc với Ubuntu 20.04)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --cli-only
```

Mặc định, script sẽ cài `govard` (CLI) và, trên Linux khi có `WebKitGTK 6.0`, cả `govard-desktop` (Desktop app) vào `/usr/local/bin` rồi:
- Tự động phát hiện và cài đặt system dependencies cần thiết.
- Khởi chạy các global services.
- Cấu hình SSL trust.
- Trên Linux, tự động fallback sang package `.deb` riêng của `govard-desktop` nếu archive độc lập không có sẵn trong bản release.

Govard Desktop cần WebKitGTK 6.0 và GTK 4, có sẵn từ Ubuntu 24.04+ và Debian 13+. Ubuntu 22.04, Debian 12 và các bản cũ hơn, cùng mọi nền tảng không phải Linux, chỉ cài được CLI: script tự nhận diện và bỏ qua Desktop, còn `--cli-only` để chủ động bỏ qua ở bất kỳ đâu.

---

## 📦 Release Installers

Mỗi release được gắn tag đều publish hai package Linux riêng biệt.

Tải từ [trang releases](https://github.com/ddtcorex/govard/releases):

### Linux (`.deb`)

Chỉ CLI (bao gồm Ubuntu 20.04):

```bash
sudo apt install ./govard_<version>_linux_<arch>.deb
```

CLI + Desktop (WebKitGTK 6.0 / Ubuntu 24.04+, Debian 13+):

```bash
sudo apt install ./govard_<version>_linux_<arch>.deb ./govard-desktop_<version>_linux_<arch>.deb
```

### Kênh package (Snap / CloudSmith)

```bash
# Snap (Linux, chỉ CLI — bắt buộc classic confinement vì
# Govard điều khiển Docker và ghi system paths)
sudo snap install govard --classic

# Debian/Ubuntu qua CloudSmith (setup repo một lần, rồi cài)
curl -1sLf https://dl.cloudsmith.io/public/ddtcorex/govard-deb/setup.deb.sh | sudo -E bash
sudo apt install govard

# Fedora/RHEL qua CloudSmith (setup repo một lần, rồi cài)
curl -1sLf https://dl.cloudsmith.io/public/ddtcorex/govard-rpm/setup.rpm.sh | sudo -E bash
sudo dnf install govard

# Alpine qua CloudSmith (setup repo một lần, rồi cài)
curl -1sLf https://dl.cloudsmith.io/public/ddtcorex/govard-apk/setup.alpine.sh | sudo -E bash
sudo apk add govard
```

### macOS (`.pkg`)

```bash
sudo installer -pkg govard_<version>_Darwin_arm64.pkg -target /
```

---

## 🔧 Build từ Source

### Điều kiện tiên quyết

Đảm bảo bạn đã cài đặt các công cụ sau:

| Công cụ | Phiên bản yêu cầu |
| :--- | :--- |
| Go | `1.25+` |
| Node.js | `20+` |
| pnpm | 11.x (chỉ cho frontend desktop) |
| golangci-lint | v2.11+ |
| Docker + Docker Compose | Bản mới nhất (chỉ cho lệnh stack — bản thân CLI không phụ thuộc Docker) |
| Wails | `v3` dưới dạng Go module tool (chỉ khi phát triển desktop, không cần cài global) |

### Cài đặt từ Source

```bash
git clone https://github.com/ddtcorex/govard.git
cd govard
./install.sh --source
```

### Thiết lập cho local development

1. **Cài đặt Go 1.25+** từ [go.dev](https://go.dev/dl/)

2. **Kích hoạt pnpm** qua Corepack:
   ```bash
   corepack enable pnpm
   pnpm --version
   ```

3. **Cài đặt golangci-lint**:
   ```bash
   curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
   ```

4. **Phát triển desktop** không cần cài Wails: runtime và CLI của nó là Go module tool trong `go.mod`, nên `go tool wails3 generate bindings` chạy được ngay từ bản clone. Trên Linux cần cài header build:
   ```bash
   sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev
   ```

Không cần quyền `sudo` — bạn có thể cài đặt mọi thứ ở local và cập nhật biến `PATH`.

---

## 🐳 Docker Images

Govard sử dụng một Dockerfile PHP duy nhất với build args thay vì các thư mục phân chia theo version.

```bash
# Image PHP tiêu chuẩn
docker build -f docker/php/Dockerfile \
  -t ddtcorex/govard-php:8.4 \
  --build-arg PHP_VERSION=8.4 \
  docker/php

# Image PHP tối ưu hóa riêng cho Magento 2
docker build -f docker/php/magento2/Dockerfile \
  -t ddtcorex/govard-php-magento2:8.4 \
  --build-arg PHP_VERSION=8.4 \
  docker/php
```

---

## 🖥️ Shell Completions

Release archives đóng gói sẵn scripts completion trong thư mục `completion/`, và package `.deb` tự cài chúng (bash, fish, zsh). Với các kênh cài khác, sinh từ binary:

```bash
govard completion bash > /etc/bash_completion.d/govard   # root
govard completion zsh > "${fpath[1]}/_govard"
govard completion fish > ~/.config/fish/completions/govard.fish
govard completion powershell | Out-String | Invoke-Expression
```

---

## 🔄 Cập nhật Govard

```bash
govard self-update
```

Lệnh `self-update` tự động tải về release artifact phù hợp với nền tảng, **xác minh mã băm SHA-256 checksum**, và thay thế các file binary đã cài đặt một cách atomic (`govard` + `govard-desktop`).

---

## ✅ Xác minh cài đặt

```bash
govard version
govard doctor
```

Lệnh `govard doctor` chạy các chẩn đoán hệ thống (system diagnostics) bao gồm kiểm tra Docker, DNS, ports và SSL trust store.

Các bản tải release (`install.sh` và npm) đều được xác minh sha256 với `checksums.txt` trước khi cài đặt; nếu hash không khớp, quá trình cài đặt dừng ngay. Không có flag bỏ qua bước kiểm tra này theo thiết kế.

---

**[← Trang chủ](/vi/)** | **[Dự án đầu tiên →](/vi/getting-started/getting-started)**
