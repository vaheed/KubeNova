import { defineConfig } from 'vitepress'

// Auto-detect base for GitHub Pages project sites
const repo = process.env.GITHUB_REPOSITORY ? process.env.GITHUB_REPOSITORY.split('/')[1] : ''
const inferredBase = repo ? `/${repo}/` : '/'

export default defineConfig({
  // Allow override via env (e.g., SITE_BASE), otherwise infer from repo
  base: process.env.SITE_BASE || inferredBase,
  title: 'KubeNova',
  description: 'Single API surface for clusters, tenants, projects, and apps',
  themeConfig: {
    nav: [
      { text: 'Step by Step', link: '/index' }    ],
    sidebar: [
      { text: 'Step by Step', link: '/index' }    ]
  }
})
