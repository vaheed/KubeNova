import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'KubeNova',
  description: 'Manager + Agent for Capsule + KubeVela',
  themeConfig: {
    nav: [
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'How it Works', link: '/how-it-works' },
      { text: 'Operations', link: '/operations' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'E2E', link: '/e2e' },
    ],
    sidebar: [
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'How it Works', link: '/how-it-works' },
      { text: 'Operations', link: '/operations' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'E2E', link: '/e2e' },
    ]
  }
})
