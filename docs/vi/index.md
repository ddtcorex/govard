---
layout: home
title: Govard — Giải pháp thay thế DDEV & Warden cho Magento, Laravel, Symfony, WordPress
description: Govard là công cụ điều phối môi trường dev cục bộ viết bằng Go — nhanh và ổn định hơn DDEV, Warden. Tự động nhận diện Magento, Laravel, Symfony, WordPress. HTTPS tích hợp sẵn, Docker SDK, đồng bộ remote an toàn.

hero:
  name: Govard
  text: Môi trường phát triển linh hoạt dựa trên Go
  tagline: Bộ điều phối phát triển local chuyên nghiệp được viết bằng Go — thay thế các công cụ dùng bash script cũ bằng một binary native hiệu năng cao giúp quản lý các môi trường container phức tạp, tập trung vào sự ổn định, tốc độ và trải nghiệm developer cao cấp.
  actions:
    - theme: brand
      text: Bắt đầu
      link: /vi/getting-started/installation
    - theme: alt
      text: Xem trên GitHub
      link: https://github.com/ddtcorex/govard

features:
  - title: Docker SDK Orchestration
    details: Tích hợp trực tiếp Docker SDK giúp hành vi lifecycle có thể dự đoán — không dùng shell-script chắp vá, chỉ có orchestration Go native.
  - title: Framework Auto-Detection
    details: Tự động nhận diện Magento 1/2, Laravel, Next.js, Drupal, Symfony, Shopware, CakePHP, PrestaShop, WordPress và nhiều framework khác từ project files.
  - title: Local HTTPS & DNS
    details: Tích hợp sẵn Caddy proxy + dnsmasq + Root CA auto-trust cho các domain *.test — phát triển local an toàn với zero-config.
  - title: Remote Management
    details: Quản lý remote được định danh với scoped capabilities, SSH, sync, chặn ghi đè trên prod và audit logs cho các thao tác remote an toàn.
  - title: Database Tools
    details: Hỗ trợ dump, import, query, giám sát real-time (db top) và bộ lọc bảo mật (--no-pii, --no-noise) cho toàn bộ quy trình làm việc với DB.
  - title: CLI + Desktop Parity
    details: Sử dụng chung một core engine cho cả CLI và ứng dụng Wails Desktop — live logs, quick actions và trình chạy shell trong một giao diện GUI đẹp mắt.
---