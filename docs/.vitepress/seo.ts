// docs/.vitepress/seo.ts
import fs from 'node:fs'
import path from 'node:path'

export const SITE_HOSTNAME = 'https://govard.ddtcorex.com'

export const SITE_TITLE = 'Govard'
const MAX_TITLE_LENGTH = 60

/**
 * VitePress appends ` | <site title>` unless the template is false or `:title`,
 * and its own template option accepts a string or a boolean only. So the dedupe
 * is decided per page: drop the suffix when the title already names the brand
 * (case-insensitive) or when the suffix would push the title past the length a
 * search result shows.
 */
export function resolveTitleTemplate(title: string, siteTitle: string = SITE_TITLE): string | undefined {
  const suffix = ` | ${siteTitle}`
  if (title.toLowerCase().includes(siteTitle.toLowerCase())) return ':title'
  if (`${title}${suffix}`.length > MAX_TITLE_LENGTH) return ':title'
  return undefined
}

/** The title exactly as the document will publish it. */
export function renderTitle(title: string, template: string | undefined, siteTitle: string = SITE_TITLE): string {
  return template === ':title' ? title : `${title} | ${siteTitle}`
}

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
  'workflows/audit': { changefreq: 'monthly', priority: 0.8 },
  'workflows/try': { changefreq: 'monthly', priority: 0.9 },
  'workflows/benchmark': { changefreq: 'weekly', priority: 0.8 },
  'workflows/lock': { changefreq: 'yearly', priority: 0.6 },
  'workflows/tunnel': { changefreq: 'monthly', priority: 0.7 },
  'developer/architecture': { changefreq: 'yearly', priority: 0.6 },
  'developer/adding-a-framework': { changefreq: 'yearly', priority: 0.6 },
  'developer/contributing': { changefreq: 'yearly', priority: 0.6 },
  'compare/ddev': { changefreq: 'monthly', priority: 0.8 },
  'more/windows': { changefreq: 'yearly', priority: 0.6 },
  'more/faq': { changefreq: 'monthly', priority: 0.7 },
  'more/changelog': { changefreq: 'weekly', priority: 0.7 },
}

const DEFAULT_SITEMAP_ENTRY = { changefreq: 'monthly', priority: 0.5 }

export function transformSitemapItems(items: any[]): any[] {
  return items.map((item) => {
    const canonicalKey = item.url.replace(/^vi\//, '')
    const meta = SITEMAP_PRIORITIES[canonicalKey] ?? DEFAULT_SITEMAP_ENTRY
    // VitePress fills `links` only for a locale group of two or more, so a lone
    // page gets its own English entry plus x-default here. Both methods are
    // emitted on every page, so they have to agree.
    const selfLang = item.url.startsWith('vi/') ? 'vi-VN' : 'en-US'
    const links = item.links?.length ? [...item.links] : [{ url: item.url, lang: selfLang }]
    const englishUrl = links.find((link: any) => link.lang === 'en-US')?.url ?? item.url
    const alternates = links.filter((link: any) => link.lang !== 'x-default')
    return {
      ...item,
      changefreq: meta.changefreq,
      priority: meta.priority,
      links: [...alternates, { url: englishUrl, lang: 'x-default' }],
    }
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

/**
 * A two level breadcrumb: the site root, then the page. Sections have no index
 * page, so a middle crumb would have to point at a URL that answers with a
 * redirect, which does not belong inside structured data.
 */
export function breadcrumbLd(page: string, title: string, siteTitle: string = SITE_TITLE): any | null {
  const url = relativePathToUrl(page)
  if (url === '/' || url === '/vi/') return null
  const isVi = page.startsWith('vi/')
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: [
      { '@type': 'ListItem', position: 1, name: siteTitle, item: `${SITE_HOSTNAME}${isVi ? '/vi/' : '/'}` },
      { '@type': 'ListItem', position: 2, name: title, item: `${SITE_HOSTNAME}${url}` },
    ],
  }
}

export function transformPageDataSeo(pageData: any, ctx: { siteConfig: any }): void {
  const relativePath: string = pageData.relativePath
  const isVi = relativePath.startsWith('vi/')
  const counterpartRelativePath = isVi ? relativePath.slice('vi/'.length) : `vi/${relativePath}`
  const counterpartExists = fs.existsSync(path.join(ctx.siteConfig.srcDir, counterpartRelativePath))
  const siteTitle = ctx.siteConfig?.site?.title ?? SITE_TITLE

  const url = relativePathToUrl(relativePath)
  const head = pageData.frontmatter.head ?? (pageData.frontmatter.head = [])

  // A page may pin its own template in frontmatter; only fill the gap.
  if (pageData.titleTemplate === undefined) {
    pageData.titleTemplate = resolveTitleTemplate(pageData.title ?? siteTitle, siteTitle)
  }

  head.push(['link', { rel: 'canonical', href: `${SITE_HOSTNAME}${url}` }])
  head.push(['link', { rel: 'alternate', hreflang: isVi ? 'vi-VN' : 'en-US', href: `${SITE_HOSTNAME}${url}` }])

  if (counterpartExists) {
    const counterpartUrl = relativePathToUrl(counterpartRelativePath)
    head.push(['link', { rel: 'alternate', hreflang: isVi ? 'en-US' : 'vi-VN', href: `${SITE_HOSTNAME}${counterpartUrl}` }])
  }

  // x-default designates the fallback page for a language we do not serve: the
  // English member of the pair, or this page when it has no counterpart.
  const englishUrl = isVi && counterpartExists ? relativePathToUrl(counterpartRelativePath) : url
  head.push(['link', { rel: 'alternate', hreflang: 'x-default', href: `${SITE_HOSTNAME}${englishUrl}` }])

  const breadcrumb = breadcrumbLd(relativePath, pageData.title ?? SITE_TITLE, siteTitle)
  if (breadcrumb) head.push(['script', { type: 'application/ld+json' }, JSON.stringify(breadcrumb)])
}
