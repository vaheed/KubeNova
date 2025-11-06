import { defineConfig } from 'vitepress'

// Resolve base for GitHub Pages project sites, defaulting to /KubeNova/
function computeBase() {
  const fromEnv = process.env.SITE_BASE
  if (fromEnv) return ensureSlash(fromEnv)
  const repo = (process.env.GITHUB_REPOSITORY || '').split('/').pop()
  if (repo) return ensureSlash(`/${repo}/`)
  return '/KubeNova/'
}

function ensureSlash(v) {
  if (!v.startsWith('/')) v = '/' + v
  if (!v.endsWith('/')) v = v + '/'
  return v
}

export default defineConfig({
  base: computeBase(),
  title: 'KubeNova',
  description: 'Single API surface for clusters, tenants, projects, and apps',
  cleanUrls: true,
  themeConfig: {
    nav: [
      { text: 'Step by Step', link: '/' }    ],
    sidebar: [
      { text: 'Step by Step', link: '/' }    ]
  }
})
