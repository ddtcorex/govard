// docs/.vitepress/seo.ts
import fs from 'node:fs'
import path from 'node:path'

export const SITE_HOSTNAME = 'https://govard.ddtcorex.com'

/**
 * Mirrors VitePress's own internal sitemap URL derivation (see
 * generateSitemap in vitepress/dist/node) so canonical/hreflang links
 * always agree with what actually ships in sitemap.xml.
 */
export function relativePathToUrl(relativePath: string): string {
  let url = relativePath.replace(/(^|\/)index\.md$/, '$1')
  url = url.replace(/\.md$/, '')
  return `/${url}`
}

// Keyed by the English-relative sitemap url (no locale prefix, no leading
// slash — matches the `item.url` shape VitePress's sitemap generator
// produces). The same entry is applied to both the English page and its
// `vi/` counterpart in transformSitemapItems below.
export const SITEMAP_PRIORITIES: Record<string, { changefreq: string; priority: number }> = {
  '': { changefreq: 'weekly', priority: 1.0 },
  'getting-started/installation': { changefreq: 'monthly', priority: 0.9 },
  'getting-started/getting-started': { changefreq: 'monthly', priority: 0.8 },
  'getting-started/migration': { changefreq: 'yearly', priority: 0.7 },
  'reference/cli-commands': { changefreq: 'monthly', priority: 0.9 },
  'reference/configuration': { changefreq: 'monthly', priority: 0.8 },
  'reference/frameworks': { changefreq: 'monthly', priority: 0.7 },
  'workflows/remotes-and-sync': { changefreq: 'monthly', priority: 0.8 },
  'workflows/ssl-and-domains': { changefreq: 'yearly', priority: 0.7 },
  'workflows/global-services': { changefreq: 'monthly', priority: 0.7 },
  'workflows/desktop-app': { changefreq: 'yearly', priority: 0.7 },
  'developer/architecture': { changefreq: 'yearly', priority: 0.6 },
  'developer/adding-a-framework': { changefreq: 'yearly', priority: 0.6 },
  'developer/contributing': { changefreq: 'yearly', priority: 0.6 },
  'more/faq': { changefreq: 'monthly', priority: 0.7 },
  'more/changelog': { changefreq: 'weekly', priority: 0.7 },
}

const DEFAULT_SITEMAP_ENTRY = { changefreq: 'monthly', priority: 0.5 }

export function transformSitemapItems(items: any[]): any[] {
  return items.map((item) => {
    const canonicalKey = item.url.replace(/^vi\//, '')
    const meta = SITEMAP_PRIORITIES[canonicalKey] ?? DEFAULT_SITEMAP_ENTRY
    return { ...item, changefreq: meta.changefreq, priority: meta.priority }
  })
}

export function transformHeadSeo(ctx: {
  page: string
  title: string
  description: string
}): [string, Record<string, string>, string?][] {
  const isVi = ctx.page.startsWith('vi/')
  const url = relativePathToUrl(ctx.page)
  const canonicalUrl = `${SITE_HOSTNAME}${url}`
  const ogImage = `${SITE_HOSTNAME}/og-image.png`

  const tags: [string, Record<string, string>, string?][] = [
    ['meta', { property: 'og:title', content: ctx.title }],
    ['meta', { property: 'og:description', content: ctx.description }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:url', content: canonicalUrl }],
    ['meta', { property: 'og:image', content: ogImage }],
    ['meta', { property: 'og:locale', content: isVi ? 'vi_VN' : 'en_US' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'twitter:title', content: ctx.title }],
    ['meta', { name: 'twitter:description', content: ctx.description }],
    ['meta', { name: 'twitter:image', content: ogImage }],
  ]

  if (ctx.page === 'index.md' || ctx.page === 'vi/index.md') {
    tags.push([
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'SoftwareApplication',
        name: 'Govard',
        applicationCategory: 'DeveloperApplication',
        operatingSystem: 'Linux, macOS',
        description: ctx.description,
        url: canonicalUrl,
        license: 'https://github.com/ddtcorex/govard/blob/master/LICENSE',
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
      }),
    ])
    tags.push([
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'WebSite',
        name: 'Govard',
        url: canonicalUrl,
      }),
    ])
  }

  return tags
}

export function transformPageDataSeo(pageData: any, ctx: { siteConfig: any }): void {
  const relativePath: string = pageData.relativePath
  const isVi = relativePath.startsWith('vi/')
  const counterpartRelativePath = isVi ? relativePath.slice('vi/'.length) : `vi/${relativePath}`
  const counterpartExists = fs.existsSync(path.join(ctx.siteConfig.srcDir, counterpartRelativePath))

  const url = relativePathToUrl(relativePath)
  const head = pageData.frontmatter.head ?? (pageData.frontmatter.head = [])

  head.push(['link', { rel: 'canonical', href: `${SITE_HOSTNAME}${url}` }])
  head.push(['link', { rel: 'alternate', hreflang: isVi ? 'vi-VN' : 'en-US', href: `${SITE_HOSTNAME}${url}` }])

  if (counterpartExists) {
    const counterpartUrl = relativePathToUrl(counterpartRelativePath)
    head.push(['link', { rel: 'alternate', hreflang: isVi ? 'en-US' : 'vi-VN', href: `${SITE_HOSTNAME}${counterpartUrl}` }])
  }
}
