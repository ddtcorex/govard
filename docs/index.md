---
layout: home
title: Govard — DDEV & Warden Alternative for Magento, Laravel, Symfony, WordPress
description: Govard is a native Go local dev orchestrator — a faster, more stable alternative to DDEV, Warden, and bash-based tools. Auto-detects Magento, Laravel, Symfony, WordPress. Built-in HTTPS, Docker SDK, safe remote sync.

hero:
  name: Govard
  text: Go-based Versatile Runtime & Development
  tagline: Professional-grade local development orchestrator engineered in Go — replaces legacy bash-based tools with a high-performance native binary that manages complex containerized environments with a focus on stability, speed, and a premium developer experience.
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started/installation
    - theme: alt
      text: View on GitHub
      link: https://github.com/ddtcorex/govard

features:
  - title: Docker SDK Orchestration
    details: Direct Docker SDK integration for predictable lifecycle behavior — no shell-script glue, just native Go orchestration.
  - title: Framework Auto-Detection
    details: Automatically detects Magento 1/2, Laravel, Next.js, Drupal, Symfony, Shopware, CakePHP, PrestaShop, WordPress, and more from your project files.
  - title: Local HTTPS & DNS
    details: Built-in Caddy proxy + dnsmasq + Root CA auto-trust for *.test domains — zero-config secure local development.
  - title: Remote Management
    details: Named remotes with scoped capabilities, SSH, sync, prod write blocking, and audit logs for safe remote operations.
  - title: First-Class Deployment
    details: Publish a git revision over SSH + rsync with a framework recipe, an atomic symlink swap or in-place publish, verification, backup and rollback — rehearse the whole pipeline against a local sandbox container first, and keep the CI deploy job toolchain-free with artifact mode.
  - title: Database Tools
    details: Dump, import, query, live monitoring (db top), and privacy filters (--no-pii, --no-noise) for complete DB workflows.
  - title: CLI + Desktop Parity
    details: Same core engine in both CLI and Wails Desktop app — live logs, quick actions, and shell launcher in a polished GUI.
  - title: Code Audit & Profiler
    details: phpcs + phpstan lint with a pub/media webshell guard, diff-scoped runs for PR review, and a one-shot Magento profiler capture. Every run is kept as an immutable session.
---
