import { defineConfig } from 'vitepress'

export default defineConfig({
  base: '/KubeNova/',
  title: 'KubeNova',
  description: 'Single API surface for clusters, tenants, projects, and apps',
  themeConfig: {
    nav: [
      { text: 'Step by Step', link: '/step-by-step' },
      { text: 'Troubleshooting', link: '/troubleshooting' }
    ],
    sidebar: [
      { text: 'Step by Step', link: '/step-by-step' },
      { text: 'Troubleshooting', link: '/troubleshooting' }
    ]
  }
})
