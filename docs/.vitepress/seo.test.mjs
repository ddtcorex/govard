// docs/.vitepress/seo.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { relativePathToUrl, transformSitemapItems, SITEMAP_PRIORITIES, transformPageDataSeo, transformHeadSeo, resolveTitleTemplate, renderTitle, breadcrumbLd } from './seo.ts'

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
  // Assert the priority contract, not the whole object: every item also carries
  // `links` since x-default landed, and that shape has its own test.
  for (const index of [0, 1]) {
    assert.equal(result[index].changefreq, SITEMAP_PRIORITIES['getting-started/installation'].changefreq)
    assert.equal(result[index].priority, SITEMAP_PRIORITIES['getting-started/installation'].priority)
  }
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
  const links = pageData.frontmatter.head.filter((t) => t[0] === 'link')
  assert.equal(links.length, 3)
  assert.deepEqual(links[2], ['link', { rel: 'alternate', hreflang: 'x-default', href: 'https://govard.ddtcorex.com/nonexistent-page' }])
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

test('transformPageDataSeo declares x-default pointing at the English page of a pair', () => {
  const siteConfig = { srcDir: new URL('.', import.meta.url).pathname.replace(/\.vitepress\/$/, '') }
  const en = { relativePath: 'reference/cli-commands.md', frontmatter: {} }
  transformPageDataSeo(en, { siteConfig })
  assert.deepEqual(en.frontmatter.head[3], ['link', { rel: 'alternate', hreflang: 'x-default', href: 'https://govard.ddtcorex.com/reference/cli-commands' }])
  const vi = { relativePath: 'vi/reference/cli-commands.md', frontmatter: {} }
  transformPageDataSeo(vi, { siteConfig })
  assert.deepEqual(vi.frontmatter.head[3], ['link', { rel: 'alternate', hreflang: 'x-default', href: 'https://govard.ddtcorex.com/reference/cli-commands' }])
})

test('transformPageDataSeo declares x-default for itself on a page with no counterpart', () => {
  const siteConfig = { srcDir: new URL('.', import.meta.url).pathname.replace(/\.vitepress\/$/, '') }
  const page = { relativePath: 'workflows/ci-integration.md', frontmatter: {} }
  transformPageDataSeo(page, { siteConfig })
  const links = page.frontmatter.head.filter((t) => t[0] === 'link')
  assert.equal(links.length, 3)
  assert.deepEqual(links[2], ['link', { rel: 'alternate', hreflang: 'x-default', href: 'https://govard.ddtcorex.com/workflows/ci-integration' }])
})

test('transformSitemapItems adds x-default to a locale pair and to a lone page', () => {
  const items = [
    { url: 'reference/cli-commands', links: [{ url: 'reference/cli-commands', lang: 'en-US' }, { url: 'vi/reference/cli-commands', lang: 'vi-VN' }] },
    { url: 'workflows/ci-integration' },
  ]
  const out = transformSitemapItems(items)
  assert.deepEqual(out[0].links.map((l) => l.lang), ['en-US', 'vi-VN', 'x-default'])
  assert.equal(out[0].links[2].url, 'reference/cli-commands')
  assert.deepEqual(out[1].links, [{ url: 'workflows/ci-integration', lang: 'en-US' }, { url: 'workflows/ci-integration', lang: 'x-default' }])
})

test('resolveTitleTemplate drops the suffix when the title already carries the brand, any case', () => {
  assert.equal(resolveTitleTemplate('Govard Architecture'), ':title')
  assert.equal(resolveTitleTemplate('why govard is fast'), ':title')
})

test('resolveTitleTemplate keeps the suffix only while the rendered title fits 60 characters', () => {
  assert.equal(resolveTitleTemplate('Deployment'), undefined)
  assert.equal(resolveTitleTemplate('x'.repeat(51)), undefined)
  assert.equal(resolveTitleTemplate('x'.repeat(52)), ':title')
})

test('renderTitle applies the template exactly', () => {
  assert.equal(renderTitle('Deployment', undefined), 'Deployment | Govard')
  assert.equal(renderTitle('Deployment', ':title'), 'Deployment')
})

test('transformPageDataSeo sets titleTemplate for a page whose title carries the brand', () => {
  const siteConfig = { srcDir: new URL('.', import.meta.url).pathname.replace(/\.vitepress\/$/, '') }
  const page = { relativePath: 'developer/architecture.md', title: 'Govard Architecture', frontmatter: {} }
  transformPageDataSeo(page, { siteConfig })
  assert.equal(page.titleTemplate, ':title')
})

test('every English page renders a title of 60 characters or fewer with no doubled brand', async () => {
  const { readdir, readFile } = await import('node:fs/promises')
  const { join, relative } = await import('node:path')
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

  const offences = []
  for (const file of await walk(root)) {
    if (relative(root, file).startsWith('vi/')) continue
    const body = await readFile(file, 'utf8')
    const match = body.match(/^---\n([\s\S]*?)\n---/)
    // YAML quotes are not part of the value, so measure the scalar, not the line.
    const title = match?.[1].match(/^title:\s*(.*)$/m)?.[1]?.trim().replace(/^["']|["']$/g, '')
    if (!title) { offences.push(`${relative(root, file)}: no title`); continue }
    const rendered = renderTitle(title, resolveTitleTemplate(title))
    if (rendered.length > 60) offences.push(`${relative(root, file)}: ${rendered.length} characters`)
    if (rendered.split('Govard').length - 1 > 1) offences.push(`${relative(root, file)}: brand twice`)
    if (title.includes(' | Govard')) offences.push(`${relative(root, file)}: frontmatter carries the suffix`)
  }
  assert.deepEqual(offences, [], `English titles that break the rule:\n${offences.join('\n')}`)
})

test('every English page description fits in a search result; the Vietnamese tree is exempt by path', async () => {
  const { readdir, readFile } = await import('node:fs/promises')
  const { join, relative } = await import('node:path')
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

  const offences = []
  for (const file of await walk(root)) {
    if (relative(root, file).startsWith('vi/')) continue
    const body = await readFile(file, 'utf8')
    const match = body.match(/^---\n([\s\S]*?)\n---/)
    const raw = match?.[1].match(/^description:\s*(.*)$/m)?.[1]?.trim()
    const description = raw?.replace(/^["']|["']$/g, '') ?? ''
    if (!description) offences.push(`${relative(root, file)}: no description`)
    else if (description.length > 160) offences.push(`${relative(root, file)}: ${description.length} characters`)
  }
  assert.deepEqual(offences, [], `English descriptions that break the rule:\n${offences.join('\n')}`)
})

test('breadcrumbLd builds a two level chain ending on the page', () => {
  const ld = breadcrumbLd('workflows/tunnel.md', 'Public Tunnel')
  assert.equal(ld['@type'], 'BreadcrumbList')
  assert.deepEqual(ld.itemListElement.map((i) => i.position), [1, 2])
  assert.equal(ld.itemListElement[0].item, 'https://govard.ddtcorex.com/')
  assert.equal(ld.itemListElement[1].item, 'https://govard.ddtcorex.com/workflows/tunnel')
  assert.equal(ld.itemListElement[1].name, 'Public Tunnel')
})

test('breadcrumbLd uses the Vietnamese root for a Vietnamese page and returns null on a homepage', () => {
  const vi = breadcrumbLd('vi/workflows/tunnel.md', 'Tunnel công khai')
  assert.equal(vi.itemListElement[0].item, 'https://govard.ddtcorex.com/vi/')
  assert.equal(vi.itemListElement[1].item, 'https://govard.ddtcorex.com/vi/workflows/tunnel')
  assert.equal(breadcrumbLd('index.md', 'Govard'), null)
  assert.equal(breadcrumbLd('vi/index.md', 'Govard'), null)
})

test('transformPageDataSeo pushes the breadcrumb script on a nested page and not on a homepage', () => {
  const siteConfig = { srcDir: new URL('.', import.meta.url).pathname.replace(/\.vitepress\/$/, '') }
  const page = { relativePath: 'workflows/tunnel.md', title: 'Public Tunnel', frontmatter: {} }
  transformPageDataSeo(page, { siteConfig })
  const scripts = page.frontmatter.head.filter((t) => t[0] === 'script')
  assert.equal(scripts.length, 1)
  assert.equal(JSON.parse(scripts[0][2])['@type'], 'BreadcrumbList')
  const home = { relativePath: 'index.md', title: 'Govard', frontmatter: {} }
  transformPageDataSeo(home, { siteConfig })
  assert.equal(home.frontmatter.head.some((t) => t[0] === 'script'), false)
})

test('every section root redirects to a page that exists', async () => {
  const { readFile, access } = await import('node:fs/promises')
  const { join } = await import('node:path')
  const { fileURLToPath } = await import('node:url')
  const root = fileURLToPath(new URL('..', import.meta.url))
  const body = await readFile(join(root, 'public/_redirects'), 'utf8')

  const rules = body
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'))
    .map((line) => line.split(/\s+/))

  for (const rule of rules) {
    assert.equal(rule.length, 3, `rule needs source, target and code: ${rule.join(' ')}`)
    assert.equal(rule[2], '301', `rule must be a permanent redirect: ${rule.join(' ')}`)
  }

  const sources = rules.map((rule) => rule[0])
  const sectionRoots = ['workflows', 'developer', 'reference', 'getting-started', 'compare', 'more']
  for (const section of sectionRoots) {
    assert.ok(sources.includes(`/${section}/`), `missing redirect for /${section}/`)
    assert.ok(sources.includes(`/vi/${section}/`), `missing redirect for /vi/${section}/`)
  }

  const favicon = rules.find((rule) => rule[0] === '/favicon.ico')
  assert.deepEqual(favicon, ['/favicon.ico', '/favicon.svg', '301'])

  for (const [, target] of rules) {
    const relative = target.replace(/^\//, '')
    const candidates = [join(root, `${relative}.md`), join(root, relative, 'index.md'), join(root, 'public', relative.replace(/^public\//, ''))]
    const exists = await Promise.all(candidates.map((file) => access(file).then(() => true, () => false)))
    assert.ok(exists.some(Boolean), `redirect target has no page: ${target}`)
  }
})

test('_headers caches HTML at the edge, detaches Cache-Control for assets, and de-indexes the pages.dev hosts', async () => {
  const { readFile } = await import('node:fs/promises')
  const { join } = await import('node:path')
  const { fileURLToPath } = await import('node:url')
  const root = fileURLToPath(new URL('..', import.meta.url))
  const body = await readFile(join(root, 'public/_headers'), 'utf8')

  const blocks = new Map()
  let current = null
  for (const raw of body.split('\n')) {
    if (!raw.trim() || raw.trim().startsWith('#')) continue
    if (!/^\s/.test(raw)) {
      current = raw.trim()
      blocks.set(current, [])
    } else if (current) {
      blocks.get(current).push(raw.trim())
    }
  }

  // Cloudflare applies header rules in file order and a detach only removes what
  // earlier rules added, so the narrow block must follow the broad one. A Map
  // throws order away, so assert on the raw text.
  const broadAt = body.indexOf('\n/*\n')
  const narrowAt = body.indexOf('\n/assets/*')
  assert.ok(broadAt >= 0 && narrowAt >= 0, 'missing the /* or /assets/* block')
  assert.ok(broadAt < narrowAt, 'the /* block must come before /assets/* or the detach does nothing')

  const html = blocks.get('/*')
  assert.ok(html, 'missing the /* block')
  const htmlCache = html.filter((line) => line.toLowerCase().startsWith('cache-control:'))
  assert.equal(htmlCache.length, 1, 'the /* block must set Cache-Control exactly once')
  assert.match(htmlCache[0], /s-maxage=600/)

  const assets = blocks.get('/assets/*')
  assert.ok(assets, 'missing the /assets/* block')
  const detachAt = assets.findIndex((line) => line === '! Cache-Control')
  const cacheAt = assets.findIndex((line) => line.toLowerCase().startsWith('cache-control:'))
  assert.ok(detachAt >= 0, 'the asset block must detach Cache-Control: Pages comma-joins it with the /* value')
  assert.ok(cacheAt > detachAt, 'the detach must come before the value it sets')
  assert.match(assets[cacheAt], /immutable/)

  for (const host of ['https://:project.pages.dev/*', 'https://:version.:project.pages.dev/*']) {
    const block = blocks.get(host)
    assert.ok(block, `missing the ${host} block`)
    assert.ok(block.some((line) => /^x-robots-tag:\s*noindex$/i.test(line)), `${host} must be noindexed`)
  }
})

test('frontmatter title and description are valid YAML scalars when they contain a colon', async () => {
  const { readdir, readFile } = await import('node:fs/promises')
  const { join, relative } = await import('node:path')
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

  const offences = []
  for (const file of await walk(root)) {
    const body = await readFile(file, 'utf8')
    const match = body.match(/^---\n([\s\S]*?)\n---/)
    if (!match) continue
    for (const key of ['title', 'description']) {
      const value = match[1].match(new RegExp(`^${key}:(.*)$`, 'm'))?.[1]?.trim()
      if (!value) continue
      const quoted = /^(['"]).*\1$/.test(value)
      // An unquoted YAML plain scalar may not contain ": ", it parses as a mapping.
      if (!quoted && /:\s/.test(value)) offences.push(`${relative(root, file)}: unquoted ${key} with a colon`)
    }
  }
  assert.deepEqual(offences, [], `frontmatter that breaks the YAML parse:\n${offences.join('\n')}`)
})
