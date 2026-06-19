import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Architecture',
      items: ['architecture/index', 'architecture/scrape', 'architecture/crawl', 'architecture/map', 'architecture/search'],
    },
    {
      type: 'category',
      label: 'API Reference',
      items: ['api/scrape', 'api/crawl', 'api/map', 'api/search'],
    },
    {
      type: 'category',
      label: 'QuickCrawl Skill',
      items: ['skill/installation'],
    },
    {
      type: 'category',
      label: 'MCP Server',
      items: ['mcp/installation', 'mcp/tools'],
    },
    {
      type: 'category',
      label: 'CLI',
      items: ['cli/installation', 'cli/usage'],
    },
    {
      type: 'category',
      label: 'Python SDK',
      items: ['sdk/python', 'sdk/reference'],
    },
    {
      type: 'category',
      label: 'Environment',
      items: ['environment/configuration'],
    },
    {
      type: 'category',
      label: 'Deployment',
      items: ['deployment/index'],
    },
  ],
};

export default sidebars;