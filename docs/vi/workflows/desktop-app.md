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
| **Thao tác nhanh** | PHPMyAdmin, bật/tắt Xdebug, Mailpit, DB client |
| **Tab Remotes** | Quy trình thêm/kiểm tra/mở/lập kế hoạch đồng bộ cho môi trường remote |
| **Tab Data** | Thông tin DB đã che, query có kiểm soát, dump/import, snapshot (xem bên dưới) |
| **Giám sát tài nguyên** | CPU, RAM, mạng, cảnh báo OOM |
| **Nhật ký (Logs)** | Lọc logs theo dịch vụ, mức độ nghiêm trọng, tìm kiếm text, stream trực tiếp |
| **Shell Launcher** | Chọn container dịch vụ, user kết nối và loại shell tương ứng |
| **Tool Runner** | Chạy các lệnh tool theo preset của framework, có stream output (xem bên dưới) |
| **Thông báo hệ thống** | Hiển thị cảnh báo khi các thao tác thành công hoặc thất bại |
| **Bảng Cài đặt** | Thay đổi theme giao diện, cấu hình proxy, trình duyệt ưa thích, DB client |

> Các thao tác khởi động/dừng/tải môi trường và quản lý dịch vụ toàn cục trên Desktop đều gọi trực tiếp tới tầng lệnh CLI của Govard (`govard up`, `govard env ...`, `govard svc ...`), giúp hành vi của ứng dụng Desktop luôn đồng bộ với các cập nhật mới nhất của CLI.

Các thao tác dài được chờ lâu hơn: `up`, `bootstrap`, restart và pull chờ tối đa 15 phút (10 phút cho global-services start) thay vì thất bại sau hai phút. Thông báo timeout nêu rõ thời lượng; khi thao tác có thể đã để lại container dở dang, thông báo sẽ nói rõ và chỉ tới chức năng phát hiện dự án mồ côi trên dashboard. Mỗi project chỉ chạy một thao tác thay đổi môi trường tại một thời điểm — bấm Start hai lần hay Start rồi Stop dồn dập sẽ báo thao tác đang chạy thay vì khởi động trùng lặp, và khóa chỉ được thả khi lệnh backend thực sự kết thúc kể cả khi bộ đếm dự phòng của UI đã hết trước.

Các panel log trực tiếp giữ 2.000 dòng gần nhất và tạm dừng polling khi cửa sổ bị ẩn, rồi tải dữ liệu mới khi quay lại. Nếu một panel gặp lỗi, nó hiển thị thông báo kèm nút thử lại thay vì trắng trơn.

---

## Sức khỏe: Doctor và Audit toàn vẹn

Thanh sức khỏe ở đầu dashboard chạy hai kiểm tra chỉ đọc cho dự án đang
chọn: `RunDoctor` (chẩn đoán máy và dự án, kèm nút sửa từng mục) và
`RunIntegrityAudit` (audit toàn vẹn dự án không cần container). Cả hai đều đi
qua các route `HealthService` trong `desktop/frontend/services/bridge.js`
(`runDoctor(project)`, `runIntegrityAudit(project)`);
tiến trình audit phát trên sự kiện `audit:progress`. Các lượt chạy dài có thể
hủy (`CancelDoctorRun`, `CancelAudit`); hủy audit sẽ loại bỏ kết quả đến muộn
và banner giữ nguyên kết quả hoàn tất gần nhất.

Lượt chạy doctor bị giới hạn theo thư mục gốc của dự án đang chọn: các mục
kiểm tra gắn với dự án đọc file của đúng dự án đó, và báo cáo được gom theo
đúng dự án. Dự án có tên nhưng không phân giải được thành thư mục sẽ bị từ
chối kèm hướng dẫn chọn dự án, thay vì audit nhầm thư mục làm việc hiện tại.
Khi chưa chọn dự án, lượt chạy ở mức máy (thư mục làm việc) thuộc scope rỗng.

### Nút sửa của Doctor có ý nghĩa gì

Mỗi mục kiểm tra của doctor thuộc một trong ba loại. `Fix` sửa ngay trong một
lượt bấm. `Preview fix` mở kế hoạch chạy thử (dry-run) và phải được xác nhận
rõ ràng trước khi thay đổi bất cứ thứ gì. Các mục không có cách sửa an toàn
không hiện nút sửa; hãy làm theo gợi ý và lệnh CLI được đề xuất. Mục kiểm tra
nào chưa từng được allowlist phân loại sẽ bị từ chối thay vì được gắn nút.

| ID mục kiểm tra | Nút | Cách sửa làm gì |
| :--- | :--- | :--- |
| `host.govard.home` | Fix | Sửa quyền các thư mục runtime của Govard |
| `host.compose.spam` | Fix | Dọn các file compose tồn quá 7 ngày |
| `host.govard.registry` | Fix | Sửa đường dẫn registry dự án (gặp thư mục nằm ở vị trí file thì từ chối, không bao giờ tự xóa) |
| `host.search.index_block` | Preview fix | Gỡ nghẽn chỉ mục tìm kiếm bằng một lệnh ghi vào container search đang chạy |
| `project.runtime.images` | Preview fix | Tải các image Docker runtime còn thiếu (tốn mạng) |
| `project.config.audit` | Preview fix | Chuyển các key cấu hình cũ sang khối services trong `.govard.yml` |
| `project.config.drift` | Preview fix | Đồng bộ `.govard.yml` với framework profile đã phát hiện |
| `project.profile.sync` | Preview fix | Bù các thiếu hụt từ framework profile, giữ nguyên mọi thiết lập người dùng đã khai báo |
| `docker.daemon` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `docker.compose` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `host.port.80` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `host.port.443` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `host.disk.scratch` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `host.network.outbound` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `host.ssh.agent` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `host.deps.docker` | Không có | Không sửa trên desktop; làm theo gợi ý |
| `host.deps.remote` | Không có | Không sửa trên desktop; làm theo gợi ý |

Các cách sửa chạm tới dự án (`host.search.index_block` và mọi mục
`project.*`) yêu cầu đang chọn một dự án trong ứng dụng desktop; các cách sửa
toàn cục của máy có thể chạy mà không cần chọn dự án. Sau mỗi lần sửa, panel
tự chạy lại doctor trong nền, nên banner chỉ tắt khi một lượt chạy mới sạch
hoàn toàn.

### Cách sửa rủi ro: xem trước, xác nhận, hết hạn

`PreviewDoctorFix` trả về kế hoạch chạy thử (tóm tắt kèm danh sách hành động)
mà không thay đổi gì; `ApplyDoctorFix` kèm xác nhận rõ ràng mới áp dụng.
Hiểu đơn giản: một bản xem trước chỉ hiệu lực trong thời gian ngắn (khoảng 15
phút) và dùng được một lần. Nếu file dự án hay trạng thái container đổi sau
lúc xem trước, lệnh áp dụng bị từ chối và panel yêu cầu xem trước lại, lúc đó
hãy xác nhận trên kế hoạch mới. Đóng bản xem trước hay chuyển dự án sẽ hủy bản
xem trước đang chờ.

### Phạm vi audit toàn vẹn

Desktop chỉ chạy đúng mục `integrity` không cần container, cùng phân tích mà
`govard audit run --checks integrity` thực hiện. Hai dòng `lint` và `profiler`
vẫn hiển thị trong kết quả ở trạng thái `Enabled=false` kèm lý do và hướng tới
CLI (`govard audit run --checks lint`, `govard audit run --checks profiler`);
chúng không bao giờ chạy từ desktop và không bao giờ bị ẩn. `GetAuditResult`
đọc một lượt chạy đã lưu, `GetAuditDiff` tóm tắt chênh lệch finding giữa hai
lượt chạy đã lưu, còn `LastAuditResult` điền banner từ lượt chạy hoàn tất gần
nhất mà không chạy gì thêm. Panel hỏi `RunningAuditOp` khi mở, nên nút hủy
luôn có mặt kể cả khi lượt chạy đã bắt đầu từ trước lúc mở panel.

### Banner và phạm vi từng dự án

Banner luôn suy ra từ kết quả mới nhất của đúng dự án đang chọn. Chuyển dự án
sẽ tính lại banner của dự án đó và hiển thị trạng thái đang tải hoặc trống
trong lúc chờ, không bao giờ hiện banner của dự án khác.

| Banner | Ý nghĩa |
| :--- | :--- |
| Doctor, đỏ (`error`) | Báo cáo doctor mới nhất có ít nhất một mục lỗi. Banner nêu số vấn đề và số mục sửa được bên dưới |
| Doctor, vàng (`warning`) | Chỉ có cảnh báo, không có lỗi |
| Không có banner doctor | Chưa chạy lần nào, hoặc lượt chạy mới nhất đạt toàn bộ. Chỉ lượt chạy sạch mới tắt được banner cũ |
| Audit, vàng (`warning`) | Kết quả toàn vẹn mới nhất còn finding (hoặc trạng thái chưa đạt). Banner nêu số finding; panel liệt kê từng finding cùng hai dòng `lint` và `profiler` bị tắt |
| Không có banner audit | Dự án này chưa audit lần nào, hoặc lượt chạy mới nhất sạch. Các lượt bị hủy hay lỗi không thay thế kết quả hoàn tất gần nhất |

## Tool Runner

Bảng Tool Runner chạy các lệnh tool của framework từ danh sách preset do
backend sở hữu. Chọn project, chọn preset, điền tham số rồi bấm Run. Output
được stream vào panel (giữ 2.000 dòng gần nhất, cùng giới hạn với các panel
log). Dropdown Recent runs nạp lại preset cùng tham số của một lần chạy trước
(các giá trị đã scrub hiển thị `***` sẽ không được nạp lại). Khi chưa chọn
project, bảng hiển thị `Select a project to run tools.` và không cho chạy.

Giao diện nằm trong React island `desktop/frontend/islands/ToolPalette.tsx`
với các helper thuần trong `desktop/frontend/modules/tool-palette.js`. Nó gọi
Go chỉ qua `services/bridge.js`:

| Wrapper frontend | Route backend |
| :--- | :--- |
| `listToolPresets(project)` | `ToolService.ListToolPresets` |
| `runTool(project, presetId, args)` | `ToolService.RunTool` |
| `cancelToolRun(opId)` | `ToolService.CancelToolRun` |
| `getToolHistory(project)` | `ToolService.GetToolHistory` |

Tiến trình chạy được stream qua `tool:output`; kết quả đến qua
`tool:completed` hoặc `tool:failed`.

### Preset

Preset được định nghĩa trong `internal/desktop/tool_runner.go`. Bảng liệt kê
các preset theo framework của project đang chọn cộng thêm nhóm generic;
framework lạ hoặc rỗng chỉ nhận nhóm generic.

| ID preset | Chạy lệnh | Tham số | Áp dụng cho |
| :--- | :--- | :--- | :--- |
| `magento-cache-flush` | `bin/magento cache:flush` | `type` (tùy chọn, `^[a-z_]+$`) | Họ Magento |
| `magento-cache-clean` | `bin/magento cache:clean` | `type` (tùy chọn, `^[a-z_]+$`) | Họ Magento |
| `magento-setup-di-compile` | `bin/magento setup:di:compile` | không có | Họ Magento |
| `magento-indexer-reindex` | `bin/magento indexer:reindex` | `indexer` (tùy chọn, `^[a-z_]+$`) | Họ Magento |
| `laravel-migrate` | `php artisan migrate` | `database` (flag tùy chọn), `force` (flag tùy chọn, `true/false/1/0`) | Laravel |
| `laravel-cache-clear` | `php artisan cache:clear` | không có | Laravel |
| `laravel-config-clear` | `php artisan config:clear` | không có | Laravel |
| `composer-install` | `composer install` | `no-dev` (flag tùy chọn, `true/false/1/0`) | Mọi project |
| `npm-install` | `npm install` | không có | Mọi project |
| `npm-run-build` | `npm run build` | không có | Mọi project |

Họ Magento gồm `magento1`, `magento2`, `mageos` và `openmage` (kể cả các alias
framework đã đăng ký dẫn xuất từ Magento).

Các preset chậm (`magento-setup-di-compile`, `magento-indexer-reindex`,
`laravel-migrate`, `composer-install`, `npm-install`, `npm-run-build`) chạy
với trần 15 phút; các lệnh xóa cache nhanh giữ trần mặc định 2 phút. Timeout
báo rõ thời lượng, và entry history ghi lần chạy là failed (timeout) hoặc
cancelled (hủy).

### Đảm bảo không shell

Bảng chỉ gửi ID preset kèm map tham số. Argv thực thi do backend sở hữu: ID
preset lạ bị từ chối trước khi có process nào tồn tại, và tham số được kiểm
tra theo `ArgSpec` của preset (tên lạ, thiếu tham số bắt buộc, giá trị dài
quá 256 ký tự và giá trị ngoài pattern của arg đều bị loại; arg không có
pattern riêng dùng `^[A-Za-z0-9_@+.,:/=-]+$`, loại mọi khoảng trắng và ký tự
đặc biệt của shell mà không có ngoại lệ quoting). Tham số hợp lệ được ráp vào
argv của preset theo thứ tự `ArgSpec` (flag thành `--name` hoặc
`--name=value`), rồi chạy trực tiếp qua `exec.CommandContext`: không shell,
không `sh -c`, không nối chuỗi, trong mọi trường hợp.

### Đảm bảo hủy (cancel)

Mỗi project chỉ chạy một tool tại một thời điểm. Lần Run thứ hai khi đang có
lần chạy active bị từ chối với lỗi already-running, và khóa được thả khi chạy
xong hoặc khi hủy. `CancelToolRun` kill process đang chạy qua op registry và
thả khóa; hủy một ID không tồn tại hoặc đã xong là no-op vô hại trả về
`false`, và giá trị trả về đồng bộ chính là xác nhận cho phía gọi hủy.

Sự kiện terminal mang theo generation của lần chạy (`OpID`, `Gen` trong
`ToolResultPayload`). Bảng chỉ áp dụng receipt `tool:completed` hoặc
`tool:failed` khi generation của nó vẫn còn hiện hành
(`IsCurrentToolGeneration` ở backend, `applyToolTerminalEvent` trong
`modules/tool-palette.js`), và một lần chạy đã kết thúc không bao giờ lật lại
thành đang chạy. Các batch `tool:output` từ lần chạy đã hủy, đã timeout hoặc
đã bị thay thế đều bị bỏ, nên rerun nhanh không bao giờ trộn output cũ; còn
receipt terminal luôn đến để history và trạng thái được chốt.

### Lịch sử và scrub secret

Mỗi lần chạy được duyệt đều ghi vào file history JSONL riêng từng project
trong thư mục `tool-history/` thuộc Govard home, giữ tối đa 50 lần chạy gần
nhất (entry cũ nhất bị loại trước). Entry lúc duyệt mang trạng thái `unknown`
rồi được nâng thành `success`, `failed` hoặc `cancelled` trước khi sự kiện
terminal phát ra, nên lần chạy mất terminal (crash) đọc là `unknown`, không
bao giờ là success. Lỗi ghi history không bao giờ làm hỏng lần chạy.

Tham số được scrub ngay khi ghi, nên secret thô không bao giờ chạm tới file:
giá trị dưới tên key secret (`password`, `passwd`, `passphrase`, `secret`,
`token`, `apikey`, `accesskey`, `privatekey`, `pwd`, kể cả khớp hậu tố như
`db-password` hay `apiKey`) thành `***`, và text mang dạng secret (cặp
`KEY=value`, URL có credential dạng `scheme://user:pass@`, block private-key
PEM) cũng bị che tương tự. Có một giới hạn: giá trị secret trần đặt dưới tên
key không phải secret thì không phân biệt được với giá trị vô hại và không bị
phát hiện, nên chỉ tên key và dạng giá trị mới quyết định việc scrub. File
được ghi nguyên tử (atomic) với quyền hạn chế.

## Tab Data

Tab Data quản lý database của project đang chọn và các snapshot của nó:
bảng thông tin kết nối đã che, query tự do có kiểm soát, dump/import với
tiến trình streaming, và liệt kê/tạo/export snapshot cùng khôi phục qua
cổng preview.

### Query chỉ đọc theo mặc định, ghi phải opt-in rõ ràng

`DBService.RunDBQuery(project, sql, allowWrite, confirmToken)` phân loại
mọi câu lệnh trước khi chạm vào bất cứ thứ gì:

- Các từ mở đầu chỉ đọc là `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, `DESC`,
  `EXPLAIN`. Mọi từ khác đi theo đường ghi.
- Từ mở đầu trông như đọc là chưa đủ: toàn bộ câu lệnh còn bị quét tìm
  các từ thay đổi dữ liệu (`DELETE`, `UPDATE`, `INSERT`, `INTO`, `UNION`
  và các từ tương tự, các lệnh khóa đọc như `FOR UPDATE`), nên một lệnh
  ghi giấu trong thân query vẫn đi theo đường ghi.
- Input nhiều câu lệnh luôn bị từ chối. Việc từ chối diễn ra trước khi
  resolve project, nên câu lệnh bị từ chối không chạm vào gì cả.

Một lệnh ghi cần đồng thời hai thứ: `allowWrite=true` cộng token xác
nhận của từng project từ `DBService.GetDBWriteConfirmToken(project)`.
Luồng UI là: huy hiệu mode coi mọi thứ chưa chắc chắn là "write",
checkbox ghi mở hộp thoại xác nhận, hộp thoại lấy token rồi trả lại cho
`RunDBQuery`. Định dạng token là
`confirm-write:<tên-project-viết-thường>` và do phía Go sở hữu; island
chỉ mang token đi. Token chống bấm nhầm, không chống phá hoại có chủ ý:
bản thân desktop đã giữ credentials.

### Khôi phục chỉ qua preview cộng xác nhận, không có đường tắt

`SnapshotService.PreviewSnapshotRestore(project, name, dbOnly, mediaOnly)`
không ghi gì cả. Nó báo chính xác những gì lệnh apply sẽ thay thế
(`willReplaceDb`, `willReplaceMedia`, kèm dòng `summary` cho người đọc)
và trả về một `previewToken` dùng một lần. Nút xác nhận giữ disabled cho
tới khi preview này hiển thị.

`SnapshotService.ApplySnapshotRestore(project, name, previewToken,
confirm, dbOnly, mediaOnly)` đòi token cộng `confirm=true`. Thiếu một
trong hai, lệnh khôi phục không bao giờ chạy. Token gắn với project,
snapshot và scope, hết hạn sau 15 phút, và bị tiêu thụ bởi một lần apply
hợp lệ, nên phát lại không bao giờ tới được engine. Lần apply bị từ chối
không tiêu thụ gì, nên xác nhận lại sau đó vẫn được. Scope mặc định là
`full`, hoặc `dbOnly` / `mediaOnly` từ nút radio scope.

### Cam kết che dữ liệu nhạy cảm

- `DBService.GetDBInfo` trả về struct không có trường password theo thiết
  kế. `hasPassword` chỉ báo có cấu hình password hay không, và
  `displayUrl` hiển thị password dưới dạng `****`.
- Output của query đi qua `sanitizeUIText`, giới hạn 20.000 ký tự, đồng
  thời loại bỏ escape ANSI và byte điều khiển.
- Frontend chỉ hiển thị thông tin DB qua allowlist `pickDisplayDBInfo`
  (`project`, `engine`, `host`, `port`, `database`, `user`,
  `hasPassword`, `displayUrl`): mọi khóa lạ, kể cả password dưới bất kỳ
  tên nào, đều bị loại trước khi chạm tới DOM.
- Cố tình không có lịch sử query: không có gì gõ vào ô query được lưu
  lại, nên không có secret nào kẹt lại trong đó.

### Ngữ nghĩa của cancel

`DBService.CancelDBOp(opId)` và
`SnapshotService.CancelSnapshotOp(opId)` dừng theo dõi của desktop trước:
định danh op bị xóa để các sự kiện muộn từ goroutine đang tắt không bao
giờ tới UI, và người dùng nhận một sự kiện kết thúc `db:failed` /
`snapshot:failed`. Những gì diễn ra bên dưới thì khác nhau:

- Dump/import chạy qua tiến trình `docker` khởi động cùng context của op,
  nên cancel kích hoạt context đó và kill tiến trình. Dump bị hủy còn xóa
  file `.partial` của nó, và file đã có sẵn tại đích vẫn nguyên vẹn.
- Khôi phục snapshot gọi engine, mà engine không nhận context một khi
  import đã chạy. Cancel dừng op desktop và các sự kiện của nó trong khi
  import bên dưới có thể vẫn chạy tới cùng, giống như ngắt CLI giữa
  chừng. Thông báo nói đúng như vậy: "`<Kind>` tracking stopped by user; the
  underlying operation may run to completion" (ví dụ "Snapshot restore
  tracking stopped by user; ..."). Nó không bao giờ báo đã hủy
  điều chưa được hủy.

### Kiểm tra đĩa trước khi chạy

- `DumpDB` ước lượng kích thước database, rồi thăm dò dung lượng trống
  tại thư mục cha gần nhất còn tồn tại của đích trước khi tạo bất kỳ
  thư mục nào hay khởi động tiến trình nào. Lần từ chối không để lại
  file, thư mục hay tiến trình nào.
- `ImportDB` kiểm tra file đồng bộ trước khi đăng ký op: file phải là
  file thường không rỗng (không phải thư mục), mang header dump được
  nhận diện (`mysqldump`/`MariaDB` hoặc PostgreSQL, gzip được xử lý
  trong suốt), và engine phải khớp database của project. File bị từ chối
  không bao giờ mở writer tới database.
- `ExportSnapshot` từ chối đích là thư mục hoặc có thư mục cha không tồn
  tại. Xác nhận ghi đè file đã có là việc của UI: các hộp thoại lưu trả
  về `{ path, exists }` để island hỏi trước khi gọi dump hay export.

### Route và sự kiện

Các route của bridge trong `desktop/frontend/services/bridge.js` trỏ tới
bindings đã sinh (đã đối chiếu với `DBService`, `SnapshotService` và
`data_dialog.go`):

| Route | Go service.method |
| :--- | :--- |
| `GetDBInfo` | `DBService.GetDBInfo` |
| `RunDBQuery` | `DBService.RunDBQuery` |
| `DumpDB` | `DBService.DumpDB` |
| `ImportDB` | `DBService.ImportDB` |
| `CancelDBOp` | `DBService.CancelDBOp` |
| `PickDumpDestination` | `DBService.PickDumpDestination` |
| `PickImportSource` | `DBService.PickImportSource` |
| `GetDBWriteConfirmToken` | `DBService.GetDBWriteConfirmToken` |
| `ListSnapshots` | `SnapshotService.ListSnapshots` |
| `CreateSnapshot` | `SnapshotService.CreateSnapshot` |
| `PreviewSnapshotRestore` | `SnapshotService.PreviewSnapshotRestore` |
| `ApplySnapshotRestore` | `SnapshotService.ApplySnapshotRestore` |
| `ExportSnapshot` | `SnapshotService.ExportSnapshot` |
| `CancelSnapshotOp` | `SnapshotService.CancelSnapshotOp` |
| `PickExportDestination` | `SnapshotService.PickExportDestination` |

Các sự kiện backend trong `internal/desktop/events.go`:

| Sự kiện | Mục đích |
| :--- | :--- |
| `db:progress` | Tiến trình dump/import có cấu trúc (`opId`, `generation`, số byte, phần trăm) |
| `db:completed` | Dump/import hoàn tất |
| `db:failed` | Dump/import thất bại hoặc bị hủy |
| `snapshot:completed` | Tạo/khôi phục/export snapshot hoàn tất |
| `snapshot:failed` | Op snapshot thất bại hoặc bị hủy |

Các op snapshot chỉ phát sự kiện kết thúc, không có sự kiện tiến trình.
Lựa chọn trống trong hộp thoại (lưu hay mở) mà không có lỗi nghĩa là
người dùng đã hủy, và island gửi đúng đường dẫn đã chọn tới
dump/import/export, không bao giờ tự bịa đường dẫn. Hộp thoại dump mặc
định là `<project>-dump.sql`; hộp thoại export mặc định là
`<name>.tar.gz`, khớp với CLI.

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
| Mở URL admin (Remote) | `OpenRemoteURL` dựng `https://<url hoặc host>/<admin path>` từ admin path của framework |

Đối với phương thức cấu hình `auth.method: ssh-agent`, ứng dụng Desktop tái sử dụng `SSH_AUTH_SOCK` và thăm dò socket tại `/run/user/<uid>/keyring/ssh` trên môi trường Linux.

Mọi thao tác đều resolve remote trước khi dựng bất cứ thứ gì, và một remote không
resolve được cả `url` lẫn `host` sẽ bị từ chối kèm lỗi thay vì được mở. URL admin
cùng các launcher SFTP/SSH đều sẽ fallback về chuỗi `localhost`, tức là mở chính máy
của lập trình viên trong khi vẫn báo rằng remote đã được mở.

Remote synthetic `sandbox` được resolve từ container mà `govard sandbox up` đã
tạo, không bao giờ từ block `remotes.sandbox` của project, nên một block không khai
báo host không thể khiến thao tác nào trỏ về `localhost`. Nó xuất hiện trong bảng
remote bất cứ khi nào container đó trả lời, và không phụ thuộc project có cấu
hình block hay không. Đó là điều kiện kích hoạt mà `govard remote list` cũng dùng,
chứ không phải cùng một quy tắc về dòng: bảng của app bỏ qua sandbox mà nó không
resolve được, trong khi `remote list` luôn in một dòng và đặt trạng thái vào cột
HOST. Khi block đã được cấu
hình mà không có container đang chạy, dòng đó bị bỏ khỏi bảng và lý do trả về
dưới dạng warning trong bảng; một project không có block và cũng không có
container thì không có dòng nào và không có warning nào. Xem thêm
[Cấu hình remote sandbox](/vi/workflows/remotes-and-sync#cấu-hình-remote-sandbox).

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

## Sức khỏe: Doctor và Audit (Health)

Header hiển thị một banner cho mỗi nguồn, mỗi banner thuộc về project đang
chọn. Đổi project thì banner của project đó hiện ra; project chưa từng chạy
không có banner. Cả hai banner đều chỉ suy từ kết quả mới nhất, nên chạy lại
mà sạch sẽ xóa banner cũ thay vì để nó treo lại. Hai panel Doctor và Audit nằm
ngay dưới ảnh chụp mesh trong cùng khung hình.

### Banner Doctor

Chạy doctor từ panel Doctor. Banner chỉ xuất hiện khi báo cáo mới nhất còn
check trượt hoặc cảnh báo:

| Banner | Ý nghĩa |
| :--- | :--- |
| Đỏ (error), `Doctor: N issues` | Có ít nhất một check trượt. Khi có ít nhất một check trượt hoặc cảnh báo kèm nút sửa, đoạn `, M fixable below` được thêm vào. |
| Vàng (warning), cùng nội dung | Chỉ có cảnh báo, không có lỗi trượt. |
| Không có banner | Lần chạy mới nhất đạt toàn bộ, hoặc project này chưa từng chạy doctor. |

Nút `Re-run` trên banner chạy lại doctor. Lần chạy dài có thể hủy; lần chạy bị
hủy giữ nguyên banner trước đó.

### Sửa lỗi doctor: mỗi check làm được gì

Mỗi check doctor thuộc một trong ba loại sửa. Nhãn nút cho biết loại nào
trước khi bất cứ thứ gì chạy:

| ID check | Bản sửa làm gì | Cách áp dụng |
| :--- | :--- | :--- |
| `host.govard.home` | Sửa thư mục runtime của Govard (quyền ghi) | Nút `Fix`, một lần bấm |
| `host.compose.spam` | Dọn các file compose thừa | Nút `Fix`, một lần bấm |
| `host.govard.registry` | Sửa đường dẫn registry project bị hỏng (đường dẫn đang là thư mục thì bị từ chối, không bao giờ xóa gì) | Nút `Fix`, một lần bấm |
| `host.search.index_block` | Gỡ kẹt chỉ mục tìm kiếm (ghi vào container search đang chạy, nên không bao giờ một lần bấm) | `Preview fix`, rồi xác nhận |
| `project.runtime.images` | Tải các image runtime còn thiếu (tải qua mạng) | `Preview fix`, rồi xác nhận |
| `project.config.audit` | Chuyển các key cấu hình cũ (ghi cấu hình) | `Preview fix`, rồi xác nhận |
| `project.config.drift` | Đồng bộ `.govard.yml` với profile phát hiện được (ghi cấu hình) | `Preview fix`, rồi xác nhận |
| `project.profile.sync` | Đồng bộ profile mà không ghi đè các cài đặt người dùng đã chọn rõ ràng (ghi cấu hình) | `Preview fix`, rồi xác nhận |
| `docker.daemon`, `docker.compose`, `host.port.80`, `host.port.443`, `host.disk.scratch`, `host.network.outbound`, `host.ssh.agent`, `host.deps.docker`, `host.deps.remote` | Không có đường sửa, làm theo gợi ý hoặc lệnh đề xuất của check | Không có nút sửa |

ID check nào ứng dụng chưa biết sẽ bị từ chối kèm lỗi `unknown doctor
check` thay vì lặng lẽ không đưa gì, để check mới của engine buộc phải được
xét duyệt allowlist thay vì rơi nhầm nút.

Các bản sửa thuộc project (mọi check `project.*` cộng `host.search.index_block`)
đòi hỏi phải chọn project trước; chưa chọn gì thì cả xem trước lẫn áp dụng đều
bị từ chối kèm hướng dẫn. Các bản sửa một lần bấm thuộc về máy (host-global)
chạy được mà không cần project.

### Xem trước và xác nhận, nói dễ hiểu

Bản sửa rủi ro luôn mở xem trước. Bản xem trước là chạy thử: liệt kê những gì
sẽ đổi mà không đổi gì cả. Bấm xác nhận (`Confirm and apply`) áp dụng đúng kế
hoạch đã xem, rồi doctor chạy lại để các lỗi đã sửa biến khỏi kết quả mới.

Mỗi lần xem trước tạo một lượt duyệt dùng một lần, hiệu lực 15 phút, chỉ cho
check đó và đúng kế hoạch đó. Lượt duyệt dùng một lần: xác nhận thì hết, đóng
hộp thoại thì mất, đổi project thì mất. Nếu có gì đổi khác giữa lúc xem và lúc
xác nhận, lệnh áp dụng bị từ chối và yêu cầu xem lại thay vì áp dụng kế hoạch
cũ. Bản thân lượt duyệt không bao giờ hiện lên màn hình; nó chỉ nằm trong bộ
nhớ, nên khởi động lại ứng dụng là các lần xem trước đang dở hết hiệu lực. Các
bản sửa một lần bấm bỏ qua toàn bộ bước này và áp dụng ngay.

### Phạm vi audit và banner

Nút `Run audit` trong panel Audit chỉ chạy kiểm tra toàn vẹn (integrity) không
cần container, đúng các bộ phân tích của `govard audit run --checks integrity`.
Nó không bao giờ cần Docker và không bao giờ chạm vào container.

Kết quả liệt kê ba dòng. Dòng integrity chạy thật; hai dòng còn lại vẫn hiện
nhưng bị tắt, không bao giờ giấu đi:

| Dòng | Trạng thái | Lý do hiển thị |
| :--- | :--- | :--- |
| `integrity` | Chạy | Không cần container |
| `lint` | Tắt | Đòi Docker nên không chạy từ desktop; chạy `govard audit run --checks lint` từ CLI |
| `profiler` | Tắt | Đòi Docker nên không chạy từ desktop; chạy `govard audit run --checks profiler` từ CLI |

Phiên không có lần chạy integrity nào hiển thị dòng integrity ở trạng thái tắt
với lý do `Session has no integrity run` thay vì nhận đã chạy.

Banner audit màu vàng (warning) với nội dung `Audit: N findings` và nút `Run
audit`. Không có banner khi project này chưa từng chạy audit, hoặc lần chạy
mới nhất đạt và không có finding nào. Hủy audit thì kết quả đang chạy bị bỏ:
quá trình phân tích có thể chạy nốt trong nền, nhưng kết quả của nó không bao
giờ được ghi nhận và banner trước đó giữ nguyên. Nút `Cancel` vẫn dùng được kể
cả khi lần chạy bắt đầu trước lúc panel mở, nhờ tra cứu thao tác đang chạy khi
gắn panel.

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