import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
import type { ScalarOptions } from '@scalar/docusaurus'

const config: Config = {
  title: 'QuickCrawl',
  tagline: 'Web Scraping API for AI Agents — Scrape, crawl, and map websites with a single binary.',
  favicon: 'img/favicon.ico',

  future: {
    v4: true,
  },

  url: 'https://quickcrawl.dev',
  baseUrl: '/',

  organizationName: 'MabudAlam',
  projectName: 'QuickCrawl',

  onBrokenLinks: 'ignore',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/MabudAlam/QuickCrawl/tree/main/docs/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    [
      '@scalar/docusaurus',
      {
        label: 'API Docs',
        route: '/api-docs',
        showNavLink: true,
        configuration: {
          spec: {
            url: '/swagger.json',
          },
          theme: 'bluePlanet',
          layout: 'modern',
          hideSearch: true,
          hideModels: true,
          showOperationId: true,
          hideClientButton: false,
          showSidebar: true,
          showDeveloperTools: 'localhost',
          showToolbar: 'localhost',
          operationTitleSource: 'summary',
          persistAuth: false,
          telemetry: true,
          isEditable: false,
          documentDownloadType: 'both',
          hideTestRequestButton: false,
          hideDarkModeToggle: true,
          withDefaultFonts: true,
          defaultOpenFirstTag: true,
          defaultOpenAllTags: false,
          expandAllModelSections: false,
          expandAllResponses: false,
          expandAllSchemaProperties: false,
          orderSchemaPropertiesBy: 'alpha',
          orderRequiredPropertiesFirst: true,
          modelsSectionLabel: 'Models',
          externalUrls: {
            dashboardUrl: 'https://dashboard.scalar.com',
            registryUrl: 'https://registry.scalar.com',
            proxyUrl: 'https://proxy.scalar.com',
            apiBaseUrl: 'https://api.scalar.com',
          },
        },
      } as ScalarOptions,
    ],
  ],

  themeConfig: {
    image: 'img/logo.svg',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'QuickCrawl',
      logo: {
        alt: 'QuickCrawl Logo',
        src: 'img/logo.svg',
        
        srcDark: 'img/logo-dark.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs',
        },
        {
          href: 'https://github.com/MabudAlam/QuickCrawl',
          label: 'GitHub',
          position: 'right',
        },
        // {
        //   href: '/api-docs',
        //   label: 'API Reference',
        //   position: 'right',
        // },

      ],
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;