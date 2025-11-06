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
      { text: 'API Quickstart', link: '/api-quickstart' },
      { text: 'Tenants & Projects', link: '/tenants-projects' },
      { text: 'Apps & Deploy', link: '/apps-deploy' },
      { text: 'Policies & Security', link: '/policies-security' },
      { text: 'Kubeconfig (Scoped)', link: '/kubeconfig-scoped' },
      { text: 'Usage', link: '/usage' },
      { text: 'How it Works', link: '/how-it-works' },
      { text: 'API Quick Start', link: '/api-quickstart' },
      { text: 'Provider Setup', link: '/provider-setup' },
      { text: 'Operations', link: '/operations' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'E2E', link: '/e2e' },
    ],
    sidebar: [
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'API Quickstart', link: '/api-quickstart' },
      { text: 'Tenants & Projects', link: '/tenants-projects' },
      { text: 'Apps & Deploy', link: '/apps-deploy' },
      { text: 'Policies & Security', link: '/policies-security' },
      { text: 'Kubeconfig (Scoped)', link: '/kubeconfig-scoped' },
      { text: 'Usage', link: '/usage' },
      { text: 'How it Works', link: '/how-it-works' },
      { text: 'API Quick Start', link: '/api-quickstart' },
      { text: 'Provider Setup', link: '/provider-setup' },
      { text: 'Operations', link: '/operations' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'E2E', link: '/e2e' },
    ]
  }
})
