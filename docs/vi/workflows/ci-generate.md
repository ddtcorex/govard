---
title: Sinh pipeline CI
description: Render pipeline GitLab chạy được từ .govard.yml bằng govard ci generate — một nguồn sự thật duy nhất, kiểm tra drift, và kiểu ship một job.
---

# Sinh pipeline CI

`govard ci generate` render một pipeline GitLab chạy được từ `.govard.yml` của
dự án. Config là nguồn sự thật duy nhất: remote, branch và `mage_mode` chảy
thẳng vào stage, rule và lệnh ship, nên đổi tên branch hay thêm môi trường chỉ
cần regenerate — không bao giờ sửa tay file đã sinh.

```bash
govard ci generate --provider gitlab --output .gitlab-ci.yml
govard ci generate --provider gitlab --check --output .gitlab-ci.yml
```

Quá trình sinh chỉ đọc config: không container, không mạng, không chạm target.
Chạy ở đâu cũng được — laptop, CI, máy chưa cài gì.

## Khi nào sinh, khi nào viết tay

Sinh pipeline khi dự án đã mô tả môi trường trong `.govard.yml` — tức mọi dự
án deploy bằng govard. Chỉ viết tay hai thứ không phải dữ liệu dự án nên nằm
ngoài file đã sinh:

- `CI_GOVARD_IMAGE`: biến CI/CD trỏ tới image duy nhất chạy job (PHP +
  Composer + Node + rsync + govard), ghim bằng digest. Đường dẫn registry là
  riêng tư của công ty nên nằm trong GitLab, không nằm trong file public này.
- một job `ci-check` nhỏ (xem dưới) làm đỏ pipeline khi YAML đã commit lệch
  khỏi `.govard.yml`.

## Pipeline đã sinh gồm những gì

Bốn stage: `integrity` → `lint` → `ship` → `rollback`.

| Job | Khi nào | Làm gì |
| --- | --- | --- |
| `integrity` | luôn chạy | `govard audit run --checks integrity --format json` — không cần container, rớt nhanh |
| `lint:quick` | merge request | phpcs trên các file PHP/PHTML đổi trong MR cộng phpstan theo config repo, kèm báo cáo codequality |
| `lint:custom` | chạy tay + theo lịch | cùng bộ công cụ trên scope lint của recipe — tầng sâu mà cửa MR bỏ qua |
| `ship:<remote>` | đúng branch của nó | build rồi deploy trong cùng một workspace (xem dưới), mỗi remote một job, `resource_group` riêng từng môi trường |
| `rollback:<remote>` | chạy tay | `govard deploy rollback <remote> --yes` |

Secret chỉ xuất hiện bằng tên (`$SSH_KEY`, `$SSH_KNOWN_HOSTS`); giá trị không
bao giờ chạm vào file. Cache Composer và npm khóa theo lock file, và block
`variables` ghim `GOVARD_VERSION` theo đúng binary đã render file — binary dev
ghi `dev`, binary release ghi tag — nên file luôn nói rõ image job phải đóng
gói Govard nào.

Đầu vào lint thuộc về framework, không hard-code: hai setting trung tính mà
recipe của framework đặt mặc định và dự án có thể ghi đè dưới `deploy.settings`
(hoặc theo từng remote):

| Setting | Mặc định recipe Magento | Khi không đặt |
| --- | --- | --- |
| `ci_lint_phpcs_standard` | `Magento2` | `PSR-12`, mặc định của ngôn ngữ |
| `ci_lint_paths` | `app/code app/design` | dòng phpcs theo scope bị bỏ (phpstan vẫn chạy) |

Quá trình sinh xếp default của recipe dưới setting dự án — cùng chiều mà lệnh
deploy resolve — nên những gì bạn thấy là những gì framework khai báo. Framework
nào chưa khai báo scope thì deep lint chỉ chạy phpstan cho tới khi nó khai báo.

## Kiểu ship một job

Mỗi job `ship:<remote>` build rồi deploy trong cùng workspace thay vì kiểu cổ
điển chia job build / tải artifact / job deploy. Lần diễn tập đã đo lý do: một
artifact production là 101.733 file / 772 MiB, và chuyển nó giữa các job (chưa
kể tải về) là chi phí thuần túy khi runner tự làm được cả hai nửa. Không có gì
trôi giữa các job nên cũng không có gì lệch nhau được.

Hình dạng theo `mage_mode` hiệu dụng (ghi đè remote thắng từng key trước block
dự án, đúng phép merge mà lệnh deploy dùng):

- `developer` → build trên server: `govard deploy --remote <name> --revision "$CI_COMMIT_SHA" --yes`
- còn lại → `govard deploy build <name> --output "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA"`,
  rồi `govard deploy <name> --artifact-dir "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA" --yes`

Số đo trên dự án tham chiếu (Magento 2.4.9 + Hyva): build server ở production
với ma trận đủ 3×3 theme×locale hết 11m48s, deploy artifact 8m18s, ma trận gọn
2×1 hết 5m36s, cửa sổ bảo trì ~1 phút ở mọi kiểu. Mục tiêu: dev từ push tới
xanh ≤ ~4 phút, production ≤ ~9 phút ma trận đủ. Ma trận — chứ không phải kiểu
build — mới là thứ dời được phút: 9 combo static tốn ~7 phút, 2 combo tốn
~1m19s.

## Kiểm tra drift trong CI

Mỗi push nên chứng minh pipeline đã commit còn khớp config. Thêm một job viết
tay nhỏ (nó không sinh được — nó canh file đã sinh):

```yaml
ci-check:
  stage: integrity
  script:
    - govard ci generate --provider gitlab --check --output .gitlab-ci.yml
```

Nó hoạt động như `gofmt -l`: file khớp thì exit 0, lệch thì exit 1 kèm 20 dòng
lệch đầu tiên. Cách sửa luôn là regenerate, không bao giờ sửa tay dưới dòng
`DO NOT EDIT`.

## Một image, output cho người đọc

Mọi job chạy trong cùng một image nội bộ nên PHP, Composer, Node, rsync và
govard giống hệt nhau từ `integrity` tới `rollback`. Ghim bằng digest, không
bao giờ dùng tag trôi.

Job đỏ báo cáo như người, không như trace: hỏng gì, log chữ, kèm ngay lệnh tái
hiện (`govard deploy releases <remote>`, `govard deploy status`) trong log.
Lỗi chặn pipeline; warning (warning phpcs, nhiễu phpstan dưới baseline của
repo) không bao giờ chặn.

- Kiểu artifact hai job mà cách này thay thế, và artifact mang được gì:
  [Triển khai](/vi/workflows/deployment#mô-hình-ci-hai-job)
- Từng bước chạy ở đâu với từng dạng dự án:
  [Case study triển khai](/vi/workflows/deploy-case-studies)
