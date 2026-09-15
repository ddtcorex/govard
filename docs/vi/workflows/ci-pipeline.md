---
title: Pipeline CI
description: Mẫu pipeline GitLab viết tay cho dự án govard — integrity, lint, ship một job và rollback tay bằng các lệnh đã có.
---

# Pipeline CI

Govard không kèm công cụ sinh pipeline: file `.gitlab-ci.yml` của dự án được
viết tay theo mẫu dưới đây, chỉ dùng các lệnh đã có. `.govard.yml` vẫn là nguồn
sự thật duy nhất cho môi trường — khi remote, branch hay `mage_mode` đổi ở đó,
cập nhật `rules` và lệnh ship tương ứng ở đây.

## Hình dạng

Bốn stage: `integrity` → `lint` → `ship` → `rollback`.

| Job | Khi nào | Làm gì |
| --- | --- | --- |
| `integrity` | luôn chạy | `govard audit run --checks integrity --format json` — không cần container, rớt nhanh |
| `lint:quick` | merge request | phpcs trên các file PHP/PHTML đổi trong MR cộng phpstan theo config repo, kèm báo cáo codequality |
| `lint:custom` | chạy tay + theo lịch | cùng bộ công cụ trên toàn bộ cây code custom — tầng sâu mà cửa MR bỏ qua |
| `ship:<remote>` | đúng branch của nó | build rồi deploy trong cùng một workspace (xem dưới), mỗi remote một job, `resource_group` riêng từng môi trường |
| `rollback:<remote>` | chạy tay | `govard deploy rollback <remote> --yes` |

## Image chạy job

Mọi job chạy trong một image nội bộ duy nhất: PHP + Composer + Node + rsync +
govard, nên toolchain giống hệt nhau từ `integrity` tới `rollback`. Khai báo một
lần bằng biến CI/CD, ghim digest — đường dẫn registry là riêng tư của công ty
nên nằm trong GitLab, không bao giờ nằm trong file public:

```yaml
image: $CI_GOVARD_IMAGE

variables:
  COMPOSER_CACHE_DIR: "$CI_PROJECT_DIR/.composer-cache"
  NPM_CONFIG_CACHE: "$CI_PROJECT_DIR/.npm-cache"
  # Cờ tốc độ đã chứng minh trên dự án tham chiếu: xử lý zip nhanh và nén
  # tối thiểu cho cache và artifact.
  FF_USE_FASTZIP: "true"
  CACHE_COMPRESSION_LEVEL: "fastest"
  ARTIFACT_COMPRESSION_LEVEL: "fastest"
```

## SSH và cache

Secret chỉ xuất hiện bằng tên (`$SSH_KEY`, `$SSH_KNOWN_HOSTS`); giá trị không
bao giờ chạm vào file. Cache Composer khóa theo lock file nên đổi lock là xả
cache chứ không phình dần:

```yaml
.ssh_setup: &ssh_setup
  - "command -v ssh-agent >/dev/null || ( apt-get update -y && apt-get install openssh-client -y )"
  - eval $(ssh-agent -s)
  - mkdir -p ~/.ssh
  - touch ~/.ssh/known_hosts
  - echo "$SSH_KEY" | tr -d '\r' | ssh-add -
  - chmod 700 ~/.ssh
  - echo "$SSH_KNOWN_HOSTS" > ~/.ssh/known_hosts

.composer_cache: &composer_cache
  key:
    files:
      - composer.lock
    prefix: ${CI_PROJECT_PATH_SLUG}
  fallback_keys:
    - ${CI_PROJECT_PATH_SLUG}-composer-default
  paths:
    - .composer-cache/
  policy: pull-push
```

## Lint

`lint:quick` chỉ chạy trên merge request: phpcs trên các file MR đã đổi
(warning không bao giờ làm đỏ job — `--runtime-set ignore_warnings_on_exit 1`)
cộng phpstan theo config repo kèm báo cáo codequality:

```yaml
lint:quick:
  stage: lint
  cache: *composer_cache
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  before_script:
    - composer install --prefer-dist --no-progress --no-interaction --no-dev
  script:
    - |
      files=$(git diff --name-only --diff-filter=ACMRT "$CI_MERGE_REQUEST_DIFF_BASE_SHA...$CI_COMMIT_SHA" -- '*.php' '*.phtml' | grep -v -e '^vendor/' -e '^generated/' -e '^var/' -e '^pub/media/' || true)
      if [ -n "$files" ]; then echo "$files" | xargs phpcs --standard=Magento2 --runtime-set ignore_warnings_on_exit 1 -p; else echo 'No PHP files changed.'; fi
    - phpstan analyse --memory-limit=2G --no-progress --error-format=gitlab > phpstan-gitlab.json
  artifacts:
    when: always
    reports:
      codequality: phpstan-gitlab.json
```

`lint:custom` là cùng bộ công cụ trên toàn bộ cây custom, chạy theo lịch và chạy
tay. Standard và scope dưới đây là giá trị Magento — framework khác thay bằng
của mình (PSR-12 là standard trung tính):

```yaml
lint:custom:
  stage: lint
  cache: *composer_cache
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - when: manual
  before_script:
    - composer install --prefer-dist --no-progress --no-interaction --no-dev
  script:
    - phpcs --standard=Magento2 --runtime-set ignore_warnings_on_exit 1 -p app/code app/design
    - phpstan analyse --memory-limit=2G --no-progress --error-format=gitlab > phpstan-gitlab.json
  artifacts:
    when: always
    reports:
      codequality: phpstan-gitlab.json
```

## Ship

Mỗi job `ship:<remote>` build rồi deploy trong cùng workspace thay vì kiểu cổ
điển chia job build / tải artifact / job deploy. Lần diễn tập đã đo lý do: một
artifact production là 101.733 file / 772 MiB, và chuyển nó giữa các job (chưa
kể tải về) là chi phí thuần túy khi runner tự làm được cả hai nửa. Không có gì
trôi giữa các job nên cũng không có gì lệch nhau được.

Hình dạng theo `mage_mode` của remote trong `.govard.yml`:

- `developer` → build trên server, mọi thứ diễn ra trên target:
  `govard deploy --remote <name> --revision "$CI_COMMIT_SHA" --yes`
- còn lại → build artifact rồi deploy trong cùng workspace:

```yaml
ship:production:
  stage: ship
  resource_group: production
  rules:
    - if: $CI_COMMIT_BRANCH == "master"
  cache: *composer_cache
  before_script:
    - *ssh_setup
  script:
    - govard version
    - govard deploy build production --output "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA"
    - govard deploy production --artifact-dir "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA" --yes
  environment:
    name: production
```

```yaml
ship:dev1:
  stage: ship
  resource_group: dev1
  rules:
    - if: $CI_COMMIT_BRANCH == "develop"
  before_script:
    - *ssh_setup
  script:
    - govard version
    - govard deploy --remote dev1 --revision "$CI_COMMIT_SHA" --yes
  environment:
    name: dev1
```

Số đo trên dự án tham chiếu (Magento 2.4.9 + Hyva): build server ở production
với ma trận đủ 3×3 theme×locale hết 11m48s, deploy artifact 8m18s, ma trận gọn
2×1 hết 5m36s, cửa sổ bảo trì ~1 phút ở mọi kiểu. Mục tiêu: dev từ push tới
xanh ≤ ~4 phút, production ≤ ~9 phút ma trận đủ. Ma trận — chứ không phải kiểu
build — mới là thứ dời được phút: 9 combo static tốn ~7 phút, 2 combo tốn
~1m19s.

## Rollback

```yaml
rollback:production:
  stage: rollback
  resource_group: production
  rules:
    - if: $CI_COMMIT_BRANCH == "master"
      when: manual
  before_script:
    - *ssh_setup
  script:
    - govard deploy rollback production --yes
```

Mỗi remote một job, cùng rule branch, luôn chạy tay.

## Output cho người đọc

Job đỏ báo cáo như người, không như trace: hỏng gì, log chữ, kèm ngay lệnh tái
hiện (`govard deploy releases <remote>`, `govard deploy status`) trong log.
Lỗi chặn pipeline; warning (warning phpcs, nhiễu phpstan dưới baseline của
repo) không bao giờ chặn. Ghim image job bằng digest, không bao giờ dùng tag
trôi.

- Kiểu artifact hai job mà cách này thay thế, và artifact mang được gì:
  [Triển khai](/vi/workflows/deployment#mô-hình-ci-hai-job)
- Từng bước chạy ở đâu với từng dạng dự án:
  [Case study triển khai](/vi/workflows/deploy-case-studies)
