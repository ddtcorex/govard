// docs/.vitepress/seo.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { relativePathToUrl, transformSitemapItems, SITEMAP_PRIORITIES, transformPageDataSeo, transformHeadSeo } from './seo.ts'

test('relativePathToUrl handles home, locale home, and nested pages', () => {
  assert.equal(relativePathToUrl('index.md'), '/')
  assert.equal(relativePathToUrl('vi/index.md'), '/vi/')
  assert.equal(relativePathToUrl('getting-started/installation.md'), '/getting-started/installation')
  assert.equal(relativePathToUrl('vi/getting-started/installation.md'), '/vi/getting-started/installation')
})

test('transformSitemapItems applies the same priority to a page and its vi/ counterpart', () => {
  const items = [
    { url: 'getting-started/installation' },
    { url: 'vi/getting-started/installation' },
    { url: 'some/unmapped-future-page' },
  ]
  const result = transformSitemapItems(items)
  assert.deepEqual(result[0], { url: 'getting-started/installation', ...SITEMAP_PRIORITIES['getting-started/installation'] })
  assert.deepEqual(result[1], { url: 'vi/getting-started/installation', ...SITEMAP_PRIORITIES['getting-started/installation'] })
  assert.equal(result[2].changefreq, 'monthly')
  assert.equal(result[2].priority, 0.5)
})

test('transformPageDataSeo adds canonical + reciprocal hreflang when a vi/ counterpart exists', () => {
  const pageData = { relativePath: 'reference/cli-commands.md', frontmatter: {} }
  const siteConfig = { srcDir: new URL('.', import.meta.url).pathname.replace(/\.vitepress\/$/, '') }
  transformPageDataSeo(pageData, { siteConfig })
  const head = pageData.frontmatter.head
  assert.deepEqual(head[0], ['link', { rel: 'canonical', href: 'https://govard.ddtcorex.com/reference/cli-commands' }])
  assert.deepEqual(head[1], ['link', { rel: 'alternate', hreflang: 'en-US', href: 'https://govard.ddtcorex.com/reference/cli-commands' }])
  assert.deepEqual(head[2], ['link', { rel: 'alternate', hreflang: 'vi-VN', href: 'https://govard.ddtcorex.com/vi/reference/cli-commands' }])
})

test('transformPageDataSeo skips the reciprocal hreflang when no counterpart file exists', () => {
  const pageData = { relativePath: 'nonexistent-page.md', frontmatter: {} }
  const siteConfig = { srcDir: new URL('.', import.meta.url).pathname.replace(/\.vitepress\/$/, '') }
  transformPageDataSeo(pageData, { siteConfig })
  assert.equal(pageData.frontmatter.head.length, 2)
})

test('transformHeadSeo emits OG/Twitter tags using the resolved page title and description', () => {
  const tags = transformHeadSeo({ page: 'reference/cli-commands.md', title: 'Govard CLI Commands Reference | Govard', description: 'Complete reference for Govard CLI commands.' })
  assert.deepEqual(tags[0], ['meta', { property: 'og:title', content: 'Govard CLI Commands Reference | Govard' }])
  assert.deepEqual(tags[4], ['meta', { property: 'og:image', content: 'https://govard.ddtcorex.com/og-image.png' }])
  assert.equal(tags.some((t) => t[0] === 'script'), false)
})

test('transformHeadSeo adds SoftwareApplication + WebSite JSON-LD only on the homepage of each locale', () => {
  const en = transformHeadSeo({ page: 'index.md', title: 'Govard', description: 'x' })
  const vi = transformHeadSeo({ page: 'vi/index.md', title: 'Govard', description: 'x' })
  const other = transformHeadSeo({ page: 'more/faq.md', title: 'FAQ', description: 'x' })
  const enScripts = en.filter((t) => t[0] === 'script')
  const viScripts = vi.filter((t) => t[0] === 'script')
  assert.equal(enScripts.length, 2)
  assert.equal(viScripts.length, 2)
  assert.equal(other.some((t) => t[0] === 'script'), false)
  const enTypes = enScripts.map((t) => JSON.parse(t[2])['@type']).sort()
  assert.deepEqual(enTypes, ['SoftwareApplication', 'WebSite'])
  const viTypes = viScripts.map((t) => JSON.parse(t[2])['@type']).sort()
  assert.deepEqual(viTypes, ['SoftwareApplication', 'WebSite'])
})

// New pages are easy to publish without the metadata the rest of the site
// carries, and nothing else in CI notices. Walk the docs tree so the next page
// that skips its frontmatter fails here instead of shipping without a
// description or a canonical tag.
test('every docs page carries a title and a description', async () => {
  const { readdir, readFile } = await import('node:fs/promises')
  const { join } = await import('node:path')
  const { fileURLToPath } = await import('node:url')
  const root = fileURLToPath(new URL('..', import.meta.url))

  const walk = async (dir) => {
    const entries = await readdir(dir, { withFileTypes: true })
    const files = []
    for (const entry of entries) {
      if (entry.name === 'node_modules' || entry.name === 'superpowers' || entry.name === 'public') continue
      const full = join(dir, entry.name)
      if (entry.isDirectory()) files.push(...(await walk(full)))
      else if (entry.name.endsWith('.md')) files.push(full)
    }
    return files
  }

  const missing = []
  for (const file of await walk(root)) {
    const body = await readFile(file, 'utf8')
    const match = body.match(/^---\n([\s\S]*?)\n---/)
    if (!match) {
      missing.push(`${file}: no frontmatter`)
      continue
    }
    for (const key of ['title', 'description']) {
      if (!new RegExp(`^${key}:`, 'm').test(match[1])) missing.push(`${file}: no ${key}`)
    }
  }

  assert.deepEqual(missing, [], `pages missing SEO frontmatter:\n${missing.join('\n')}`)
})
