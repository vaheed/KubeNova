import { defineConfig } from 'vitepress'

export default defineConfig({
  base: '/',
  title: 'KubeNova',
  description: 'Single API surface for clusters, tenants, projects, and apps',
  themeConfig: {
    nav: [
      { text: 'Step by Step', link: '/index' }    ],
    sidebar: [
      { text: 'Step by Step', link: '/index' }    ]
  }
})
