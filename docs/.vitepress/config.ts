import { defineConfig } from 'vitepress'
import { SITE_HOSTNAME, transformSitemapItems, transformPageDataSeo, transformHeadSeo } from './seo'

export default defineConfig({
  title: 'Govard',
  description: 'Go-based Versatile Runtime & Development',

  srcExclude: ['plans/**', 'superpowers/**'],

  cleanUrls: true,
  lastUpdated: true,
  appearance: 'dark',

  // sitemap.xml is generated at build time from the resolved page list
  // (siteConfig.pages) plus git-based lastUpdated timestamps — do not
  // hand-edit docs/public/sitemap.xml, it no longer exists as a static file.
  sitemap: {
    hostname: SITE_HOSTNAME,
    transformItems: transformSitemapItems,
  },

  transformPageData: transformPageDataSeo,
  transformHead: transformHeadSeo,

  head: [
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
  ],

  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
      themeConfig: {
        nav: navEn(),
        sidebar: sidebarEn(),
        editLink: {
          pattern: 'https://github.com/ddtcorex/govard/edit/master/docs/:path',
          text: 'Edit this page on GitHub',
        },
        footer: {
          message: 'Released under the MIT License.',
          copyright: 'Copyright © 2026 DDTCoreX',
        },
        docFooter: {
          prev: 'Previous',
          next: 'Next',
        },
        outline: {
          label: 'On this page',
        },
        lastUpdated: {
          text: 'Updated at',
        },
      },
    },
    vi: {
      label: 'Tiếng Việt',
      lang: 'vi-VN',
      link: '/vi/',
      themeConfig: {
        nav: navVi(),
        sidebar: sidebarVi(),
        editLink: {
          pattern: 'https://github.com/ddtcorex/govard/edit/master/docs/vi/:path',
          text: 'Chỉnh sửa trang này trên GitHub',
        },
        footer: {
          message: 'Phát hành theo giấy phép MIT.',
          copyright: 'Bản quyền © 2026 DDTCoreX',
        },
        docFooter: {
          prev: 'Trước',
          next: 'Tiếp',
        },
        outline: {
          label: 'Trang này',
        },
        lastUpdated: {
          text: 'Cập nhật lúc',
        },
      },
    },
  },

  themeConfig: {
    logo: '/favicon.svg',
    socialLinks: [
      { icon: 'github', link: 'https://github.com/ddtcorex/govard' },
    ],
    search: {
      provider: 'local',
      options: {
        locales: {
          vi: {
            translations: {
              button: {
                buttonText: 'Tìm kiếm',
                buttonAriaLabel: 'Tìm kiếm',
              },
              modal: {
                displayDetails: 'Hiển thị chi tiết',
                resetButtonTitle: 'Xóa tìm kiếm',
                backButtonTitle: 'Đóng tìm kiếm',
                noResultsText: 'Không có kết quả',
                footer: {
                  selectText: 'Chọn',
                  navigateText: 'Điều hướng',
                  closeText: 'Đóng',
                },
              },
            },
          },
        },
      },
    },
  },
})

function navEn() {
  return [
    { text: 'Getting Started', link: '/getting-started/installation', activeMatch: '/getting-started/' },
    { text: 'Reference', link: '/reference/cli-commands', activeMatch: '/reference/' },
    { text: 'Workflows', link: '/workflows/remotes-and-sync', activeMatch: '/workflows/' },
    {
      text: 'More',
      items: [
        { text: 'Developer', link: '/developer/architecture' },
        { text: 'FAQ', link: '/more/faq' },
        { text: 'Changelog', link: '/more/changelog' },
      ],
    },
  ]
}

function navVi() {
  return [
    { text: 'Bắt đầu', link: '/vi/getting-started/installation', activeMatch: '/vi/getting-started/' },
    { text: 'Tham khảo', link: '/vi/reference/cli-commands', activeMatch: '/vi/reference/' },
    { text: 'Quy trình', link: '/vi/workflows/remotes-and-sync', activeMatch: '/vi/workflows/' },
    {
      text: 'Thêm',
      items: [
        { text: 'Nhà phát triển', link: '/vi/developer/architecture' },
        { text: 'FAQ', link: '/vi/more/faq' },
        { text: 'Nhật ký thay đổi', link: '/vi/more/changelog' },
      ],
    },
  ]
}

function sidebarEn() {
  return {
    '/getting-started/': [
      {
        text: 'Getting Started',
        items: [
          { text: 'Installation', link: '/getting-started/installation' },
          { text: 'First Project', link: '/getting-started/getting-started' },
          { text: 'Migration Guide', link: '/getting-started/migration' },
        ],
      },
    ],
    '/reference/': [
      {
        text: 'Reference',
        items: [
          { text: 'CLI Commands', link: '/reference/cli-commands' },
          { text: 'Configuration', link: '/reference/configuration' },
          { text: 'Frameworks', link: '/reference/frameworks' },
        ],
      },
    ],
    '/workflows/': [
      {
        text: 'Workflows',
        items: [
          { text: 'Remotes & Sync', link: '/workflows/remotes-and-sync' },
          { text: 'SSL & Domains', link: '/workflows/ssl-and-domains' },
          { text: 'Global Services', link: '/workflows/global-services' },
          { text: 'Desktop App', link: '/workflows/desktop-app' },
        ],
      },
    ],
    '/developer/': [
      {
        text: 'Developer',
        items: [
          { text: 'Architecture', link: '/developer/architecture' },
          { text: 'Adding a Framework', link: '/developer/adding-a-framework' },
          { text: 'Contributing', link: '/developer/contributing' },
        ],
      },
    ],
    '/more/': [
      {
        text: 'More',
        items: [
          { text: 'FAQ & Troubleshooting', link: '/more/faq' },
          { text: 'Changelog', link: '/more/changelog' },
        ],
      },
    ],
  }
}

function sidebarVi() {
  return {
    '/vi/getting-started/': [
      {
        text: 'Bắt đầu',
        items: [
          { text: 'Cài đặt', link: '/vi/getting-started/installation' },
          { text: 'Dự án đầu tiên', link: '/vi/getting-started/getting-started' },
          { text: 'Hướng dẫn di chuyển', link: '/vi/getting-started/migration' },
        ],
      },
    ],
    '/vi/reference/': [
      {
        text: 'Tham khảo',
        items: [
          { text: 'Lệnh CLI', link: '/vi/reference/cli-commands' },
          { text: 'Cấu hình', link: '/vi/reference/configuration' },
          { text: 'Frameworks', link: '/vi/reference/frameworks' },
        ],
      },
    ],
    '/vi/workflows/': [
      {
        text: 'Quy trình',
        items: [
          { text: 'Remote & Đồng bộ', link: '/vi/workflows/remotes-and-sync' },
          { text: 'SSL & Tên miền', link: '/vi/workflows/ssl-and-domains' },
          { text: 'Dịch vụ toàn cục', link: '/vi/workflows/global-services' },
          { text: 'Ứng dụng Desktop', link: '/vi/workflows/desktop-app' },
        ],
      },
    ],
    '/vi/developer/': [
      {
        text: 'Nhà phát triển',
        items: [
          { text: 'Kiến trúc', link: '/vi/developer/architecture' },
          { text: 'Thêm Framework', link: '/vi/developer/adding-a-framework' },
          { text: 'Đóng góp', link: '/vi/developer/contributing' },
        ],
      },
    ],
    '/vi/more/': [
      {
        text: 'Thêm',
        items: [
          { text: 'FAQ & Khắc phục sự cố', link: '/vi/more/faq' },
          { text: 'Nhật ký thay đổi', link: '/vi/more/changelog' },
        ],
      },
    ],
  }
}