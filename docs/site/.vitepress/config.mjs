import { defineConfig } from 'vitepress'

export default defineConfig({
  // GitHub Pages project site requires base to be the repo name
  // e.g. https://<owner>.github.io/KubeNova/ -> base '/KubeNova/'
  base: '/KubeNova/',
  title: 'KubeNova',
  description: 'Manager + Agent for Capsule + KubeVela',
  themeConfig: {
    nav: [
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'How it Works', link: '/how-it-works' },
      { text: 'API Quick Start', link: '/api-quickstart' },
      { text: 'Operations', link: '/operations' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'E2E', link: '/e2e' },
    ],
    sidebar: [
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'How it Works', link: '/how-it-works' },
      { text: 'API Quick Start', link: '/api-quickstart' },
      { text: 'Operations', link: '/operations' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'E2E', link: '/e2e' },
    ]
  }
})
