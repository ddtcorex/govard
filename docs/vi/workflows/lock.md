---
title: Lock File — Phát hiện lệch môi trường
description: Dùng govard.lock để snapshot và ép buộc môi trường team đồng nhất — generate, check, diff và strict mode.
---

# Lock File

`govard.lock` snapshot môi trường đã resolve (framework, version stack, hash compose, version Docker host, blueprint version) để team phát hiện drift và ép buộc tính tái lặp giữa các máy.

---

## Bắt đầu nhanh

```bash
# Snapshot project hiện tại
govard lock generate
govard lock check          # pass/fail so với trạng thái hiện tại

# So sánh không fail
govard lock diff

# Đường dẫn tùy chỉnh (vd. layout .govard/)
govard lock generate --file .govard/govard.lock
```

Commit `govard.lock` (hoặc `.govard/govard.lock`) vào Git — đó là hợp đồng của team.

---

## Strict Mode

Trong `.govard.yml`:

```yaml
lock:
  strict: true
  ignore_fields: ["host.docker_version"]  # bỏ qua field nhiễu
```

| `lock.strict` | Hành vi khi `govard env up` |
| :--- | :--- |
| `false` (mặc định) | Cảnh báo khi lệch nhưng vẫn chạy. |
| `true` | Fail nhanh khi lock thiếu hoặc lệch — dev phải `govard lock generate` sau thay đổi có chủ đích. |

`lock.ignore_fields` liệt kê JSON path cần bỏ qua khi compliance (vd. `host.docker_version` khác nhau theo máy).

---

## Khi nào regenerate

Regenerate sau mọi thay đổi stack có chủ đích:

- `stack.php_version`, `stack.db_version`, `stack.search_version`, …
- Sửa `.govard.yml` / profile
- Nâng cấp Govard làm tăng `BlueprintVersion` (xem `CHANGELOG.md` “Blueprint Lifecycle”)

```bash
govard env up --update-lock   # update lock atomically nếu phát hiện lệch
# hoặc
govard lock generate
```

---

## Ví dụ CI

```yaml
# .github/workflows/ci.yml
- run: govard lock check
```

`lock check` trả về non-zero khi drift, nên CI fail cho đến khi lock được update có chủ đích.

---

→ Tham khảo: [Lệnh CLI](/vi/reference/cli-commands#govard-lock) · [Cấu hình](/vi/reference/configuration#safety-and-reproducibility)
