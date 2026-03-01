# Govard Desktop – Code Review Chi Tiết

> Review toàn diện cho phần **desktop** (Wails + vanilla JS frontend + Go backend) của dự án Govard.

---

## 📋 Tổng Quan

| Thành phần | Files | Kích thước | Đánh giá |
|---|---|---|---|
| Frontend HTML | 1 file | 1,716 dòng | ⚠️ Cần tách |
| Frontend JS | [main.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/main.js) + 10 modules | ~30KB + ~78KB | ✅ Module hóa tốt |
| Frontend CSS | [styles.css](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/styles.css) + inline | 139 + ~170 dòng inline | ⚠️ Trùng lặp |
| Go Backend | 18 files | [app.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go) 423 dòng, [dashboard.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go) 672 dòng | ⚠️ God Object |
| Wails Config | [wails.json](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/wails.json) | 19 dòng | ⚠️ Thiếu metadata |

---

## 🔴 Vấn Đề Nghiêm Trọng (Cần Sửa Ngay)

### 1. [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) là monolith 1,716 dòng

File [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) chứa **tất cả** UI trong một file duy nhất: sidebar, header, dashboard tab, remotes tab, logs tab, onboarding modal, sync modal, settings drawer, và footer.

**Vấn đề:**
- Rất khó maintain khi UI phức tạp hơn
- Merge conflict thường xuyên khi nhiều người cùng sửa
- Không thể tái sử dụng component

**Đề xuất:** Tách thành partial HTML files hoặc sử dụng JS template literals trong các module tương ứng (ví dụ: [modules/onboarding.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/onboarding.js) render HTML cho onboarding modal).

---

### 2. CSS inline trùng lặp với [styles.css](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/styles.css)

Có **~170 dòng CSS** inline trong `<style>` tag ở [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) (dòng 50–220), bao gồm toast system styles. Đồng thời animation `@keyframes toast-shrink` được define **hai lần**: ở cả inline CSS (dòng 212) và [styles.css](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/styles.css) (dòng 78).

**Đề xuất:** Di chuyển toàn bộ inline CSS vào [styles.css](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/styles.css).

---

### 3. [App](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#11-18) struct là God Object (42+ public methods)

File [app.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go) chứa một struct [App](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#11-18) với hơn **42 public methods** expose lên frontend, bao gồm: dashboard, settings, logs, shell, remotes, onboarding, metrics, environment lifecycle...

**Vấn đề:**
- Vi phạm Single Responsibility Principle
- Khó test isolated từng feature
- Method signatures phức tạp, ví dụ [OnboardProject](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#129-153) nhận **8 parameters riêng lẻ** thay vì một struct
- [UpdateSettings](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#317-330) nhận **5 string parameters** – dễ nhầm thứ tự

**Đề xuất:**
- Tách thành domain-specific services: `DashboardService`, `RemoteService`, `ShellService`, etc.
- [App](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#11-18) chỉ delegate xuống các service
- Dùng struct input thay vì positional params: [OnboardProject(input OnboardInput) string](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#129-153)

---

### 4. CDN dependencies trong desktop app

```html
<script src="https://cdn.tailwindcss.com?plugins=forms,container-queries"></script>
<link href="https://fonts.googleapis.com/css2?family=Inter..." rel="stylesheet" />
<script src="https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.js"></script>
```

Desktop app (Wails) nên hoạt động **offline**. Tất cả dependencies hiện tải từ CDN sẽ fail khi không có internet.

**Đề xuất:** Bundle Tailwind CSS (build step), tải fonts và xterm.js vào `frontend/vendor/` hoặc sử dụng `npm` + bundler.

---

## 🟡 Vấn Đề Quan Trọng (Nên Sửa)

### 5. [wails.json](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/wails.json) thiếu metadata

```json
"author": {
  "name": "",
  "email": ""
},
"info": {
  "companyName": ""
}
```

Các trường `author.name`, `author.email`, và `companyName` để trống. Frontend build commands cũng trống — có thể OK nếu không dùng build step, nhưng nên document lý do.

---

### 6. Hardcoded version ở nhiều nơi

Version `1.9.0` xuất hiện ở ít nhất **3 nơi**:
- [wails.json](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/wails.json) dòng 16: `"productVersion": "1.9.0"`
- [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) dòng 1686: `Govard Engine v1.9.0`
- [Makefile](file:///home/kai/Work/htdocs/ddtcorex/govard/Makefile) lấy từ git tags nhưng không truyền vào frontend

**Đề xuất:** Inject version vào frontend từ Go backend qua Wails binding hoặc build-time substitution, chỉ giữ single source of truth từ git tag.

---

### 7. [dashboard.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go) quá lớn (672 dòng, 26 functions)

File [dashboard.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go) chứa **tất cả** logic liên quan đến dashboard: build dashboard, extract projects, parse config, load project info, derive services, format database names, check proxy status...

Nhiều hàm helper ([titleCase](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go#341-347), [uniqueStrings](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go#516-528), [containsService](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go#529-537), [nameMatches](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go#550-558)) nên nằm ở package riêng hoặc `internal/utils`.

---

### 8. Error handling trả về string thay vì structured error

Gần như tất cả Wails-exposed methods trả về `string` (empty = success, non-empty = error message):

```go
func (a *App) StartEnvironment(project string) string { ... }
func (a *App) StopEnvironment(project string) string { ... }
```

**Vấn đề:** Frontend không phân biệt được error types, severity, hay recovery actions.

**Đề xuất:** Cân nhắc trả về struct `{ ok: bool, message: string, code: string }` cho frontend xử lý chi tiết hơn.

---

### 9. Fallback data cứng trong [main.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/main.js)

[main.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/main.js) (dòng 249–287) chứa `safeDashboard` với **hardcoded test data** bao gồm tên projects, domains, technologies đều là placeholder:

```js
const safeDashboard = {
  ActiveEnvironments: 2,
  RunningServices: 5,
  Environments: [
    { Name: "Project Alpha", Domain: "project-alpha.test", ... },
    { Name: "Project Beta", ... },
    { Name: "Project Gamma", ... },
  ],
};
```

Nên hiển thị empty state / skeleton thay vì fake data khi bridge không available.

---

### 10. Background image từ external URL

Dòng 368 trong [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html):
```html
src="https://lh3.googleusercontent.com/aida-public/AB6AXuA4QH2F6JMZkDaFAB41GVa1pezOcG7AJFRfV..."
```

Hero background load từ Google CDN — sẽ fail offline và tạo external dependency không cần thiết.

---

## 🟢 Điểm Tốt

### ✅ Module organization cho JS
10 modules ([actions.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/actions.js), [dashboard.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/dashboard.js), [logs.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/logs.js), [metrics.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/metrics.js), [remotes.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/remotes.js), etc.) được tách rõ ràng theo feature. Controller pattern với dependency injection tốt:

```js
const logsController = createLogsController({
  bridge: desktopBridge,
  refs,
  readSelection,
  onStatus: setStatus,
  onToast: showToast,
});
```

### ✅ Build system và test infrastructure mạnh
[Makefile](file:///home/kai/Work/htdocs/ddtcorex/govard/Makefile) bao gồm: lint, fmt-check, vet, unit tests, integration tests, frontend tests, real environment tests, coverage, Docker image builds — rất comprehensive.

### ✅ Go types rõ ràng
File [types.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/types.go) define các types chính với JSON tags phù hợp. Struct organization sạch.

### ✅ Build tags phân biệt desktop vs CLI
[main.go](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/main.go) dùng `//go:build desktop` và [main_stub.go](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/main_stub.go) dùng `//go:build !desktop` — pattern tốt cho conditional compilation.

### ✅ Tailwind config tùy chỉnh có design system
Custom colors, fonts, border-radius được define trong Tailwind config — tạo visual consistency.

---

## 📊 Mức Ưu Tiên Sửa

| # | Vấn đề | Effort | Impact | Priority |
|---|---|---|---|---|
| 4 | CDN deps cho desktop app | Medium | 🔴 Critical | **P0** |
| 3 | God Object [App](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#11-18) struct | High | 🔴 High | **P1** |
| 1 | Monolith [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) | Medium | 🟡 Medium | **P2** |
| 6 | Hardcoded versions | Low | 🟡 Medium | **P2** |
| 2 | CSS trùng lặp | Low | 🟡 Low | **P3** |
| 8 | String error handling | Medium | 🟡 Medium | **P2** |
| 5 | Thiếu metadata wails.json | Low | 🟢 Low | **P3** |
| 9 | Hardcoded fallback data | Low | 🟡 Low | **P3** |
| 10 | External image URL | Low | 🟡 Low | **P3** |
| 7 | [dashboard.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go) quá lớn | Medium | 🟡 Medium | **P3** |
