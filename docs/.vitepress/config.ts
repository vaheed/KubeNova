import { defineConfig } from 'vitepress'

export default defineConfig({
  lang: 'en-US',
  title: 'KubeNova',
  description: 'Unified multi-datacenter control plane for Kubernetes',
  lastUpdated: true,
  themeConfig: {
    nav: [
      { text: 'Getting Started', link: '/getting-started/quickstart' },
      { text: 'Architecture', link: '/architecture/overview' },
      { text: 'Development', link: '/development/workflow' },
      { text: 'Deployment', link: '/deployment/overview' },
      { text: 'Operations', link: '/operations/kind-e2e' },
      { text: 'Reference', link: '/reference/configuration' },
      { text: 'Roadmap', link: '/roadmap' }
    ],
    sidebar: {
      '/getting-started/': [
        { text: 'Quickstart', link: '/getting-started/quickstart' },
        { text: 'API Lifecycle Walkthrough', link: '/getting-started/api-playbook' }
      ],
      '/architecture/': [
        { text: 'Overview', link: '/architecture/overview' },
        { text: 'Multi-Tenancy Model', link: '/architecture/multi-tenancy' }
      ],
      '/development/': [
        { text: 'Local Development', link: '/development/workflow' }
      ],
      '/deployment/': [
        { text: 'Local & Helm Deployment', link: '/deployment/overview' }
      ],
      '/operations/': [
        { text: 'kind E2E Test Setup', link: '/operations/kind-e2e' },
        { text: 'Upgrades & Validations', link: '/operations/upgrade' },
        { text: 'Observability', link: '/operations/observability' }
      ],
      '/reference/': [
        { text: 'Configuration', link: '/reference/configuration' },
        { text: 'API & OpenAPI', link: '/reference/api' },
        { text: 'Architecture Decisions (ADRs)', link: '/reference/adr' }
      ],
      '/roadmap': [
        { text: 'Roadmap', link: '/roadmap' }
      ]
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/vaheed/kubenova' }
    ]
  }
})
