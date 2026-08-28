---
title: Kiểm định dự án — Lint & Profiler
description: Chạy kiểm định lint và profiler bền vững của Govard — backend native, provider ngoài, cache và các chế độ target.
---

# Audit

Govard lưu mỗi lần audit như một session bất biến tại `~/.govard/audit/<project-id>/sessions/<session-id>/`. Dùng để lint code Magento, bắt webshell và capture CSV profiler stock của Magento mà không sửa `env.php`.

---

## Bắt đầu nhanh

```bash
# Lint toàn project (tự dò mode)
govard audit run

# Lint + profiler trên một URL (lần đầu cần --url)
govard audit run --checks lint,profiler --url 'https://shop.test/category.html?product_list_limit=48'

# Chỉ profiler
govard audit run --checks profiler --url 'https://shop.test/'

# Chạy lại đúng bộ check cũ (dùng lại URL profiler đã freeze)
govard audit rerun --session 20260816T010203Z-a1b2c3d4

# Kiểm tra
govard audit status --session 20260816T010203Z-a1b2c3d4
govard audit result --session 20260816T010203Z-a1b2c3d4 --run run-0001
govard audit diff --base origin/master

# Machine-readable
govard audit run --format json
```

Exit code: `0` khi mọi check pass, khác `0` sau summary khi có check fail/cancel — dùng được cho CI.

---

## Các check

| Check | Chức năng |
| :--- | :--- |
| `lint` | Phân tích tĩnh qua backend native (`phpcs` + `phpstan` + media guard). |
| `profiler` | Capture `MAGE_PROFILER=csvfile` stock của Magento qua include web-server có lease, một `GET` có giới hạn tới `--url`, rồi khôi phục. |

`profiler` yêu cầu:
- Target là whole Govard project (standalone/module-only bị từ chối trước khi mutate).
- `--url` tuyệt đối `http(s)://` ở lần chạy đầu (freeze trong run và được `rerun` dùng lại).
- Đã `govard env up` với bản Govard hiện tại (để mount custom config tồn tại).

Artifact: `runs/<run-id>/artifacts/profiler/profile.csv` + digest SHA-256.

---

## Chế độ target (`--mode`)

`auto` (mặc định) tự phân loại thư mục hiện tại:

| Mode | Khi nào | Phạm vi phân tích |
| :--- | :--- | :--- |
| `project` | Thư mục chứa root Magento (`bin/magento` + Composer requirement Magento) và không nằm trong module | Toàn project |
| `module_in_project` | Thư mục là module (`etc/module.xml` hoặc Composer type `magento2-module`) trong project Magento | Chỉ module đó (toàn project mount read-only) |
| `standalone` | Thư mục là module không có project Magento ở trên | Chỉ module đó (deps cài vào worktree tạm) |

Ép buộc với `--mode project|module_in_project|standalone` — lỗi nếu thư mục không thỏa.

---

## Phiên bản PHP (`--php`)

Lint image cung cấp `7.4`, `8.0`, `8.1`, `8.2`, `8.3`, `8.4`, `8.5`.

- `project` / `module_in_project`: đúng một version — `stack.php_version` đang active. Truyền `--php` chỉ chấp nhận khi trùng version đó; từ chối nếu container đang chạy báo PHP khác config.
- `standalone`: nhận `8.1`–`8.5` (mặc định cả năm). `7.4`/`8.0` bị từ chối trước khi làm gì (`unsupported_php:`).

---

## Đường dẫn quét & Media guard

Analyzer bỏ qua `vendor/`, `generated/`, `var/`, `pub/static/`, `pub/media/` (không phải shipped code). Vì `pub/media` là nơi webshell được upload, mỗi PHP version còn chạy **media guard**: quét theo tên trong `pub/media` tìm `.php/.phtml/.pht`. Mỗi hit là `M2-LINT-MEDIA` và fail run — chỉ mili giây, chỉ tên file.

---

## Provider (`--lint-provider`)

| Giá trị | Backend |
| :--- | :--- |
| `govard` (mặc định) | Native: context build nhúng, ghim theo digest. Nếu image đã ghim không pull được hoặc label lệch, Govard build local và tiếp tục. |
| `<external>` | Phải trùng key trong `audit.lint.external_providers` ở `.govard.yml` ([Cấu hình](/vi/reference/configuration#audit-lint-providers)). Không bao giờ là fallback; tên lạ = lỗi. |

Module standalone không có project config nên chỉ dùng được `govard`.

---

## Cache

State tái sử dụng: `~/.govard/cache/audit/lint/<target-id>/` (sống sót sau `audit cleanup`). Một generation cho mỗi toolchain identity (image, runner, PHP matrix, analyzer policy).

- Đổi `composer.json`/`composer.lock` hoặc ruleset (`phpcs.xml`, `phpstan.neon`, `*.dist`) loại bỏ analyzer state nhưng giữ Composer download cache.
- `--no-lint-result-cache` báo `bypassed` và giữ download cache.

Evidence ghi cache state (`cold`/`warm`/`bypassed`) + lý do theo từng PHP, image digest, toolchain digest, timing từng phase.

Credential: `~/.composer/auth.json` mount read-only khi tồn tại. `SSH_AUTH_SOCK` chỉ forward với `--allow-lint-ssh-agent`. Source tree luôn read-only.

---

## Toolchain

Image lint dùng chung cả máy — không cần project, không gọi external provider:

```bash
govard audit toolchain status  # chỉ local — nên chạy gì tiếp
govard audit toolchain pull    # chỉ image official đã ghim, không build
govard audit toolchain build   # chỉ context nhúng, không pull
```

---

## File

| Đường dẫn | Nội dung |
| :--- | :--- |
| `~/.govard/audit/<project-id>/sessions/<session-id>/manifest.json` | Session manifest |
| `.../runs/<run-id>/audit-result.json` | Evidence từng run (ghi atomically) |
| `.../runs/<run-id>/report.json` | Report native của provider |
| `.../runs/<run-id>/artifacts/profiler/profile.csv` | CSV profiler (khi bật) |
| `~/.govard/cache/audit/lint/<target-id>/` | Cache lint tái sử dụng |

→ Tham khảo: [Lệnh CLI](/vi/reference/cli-commands#govard-audit) · [Cấu hình](/vi/reference/configuration#audit-lint-providers)
