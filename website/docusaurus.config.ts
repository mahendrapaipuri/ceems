import { exec } from  "child_process";
import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";
import type * as OpenApiPlugin from "docusaurus-plugin-openapi-docs";
import { myCustomApiMdGenerator } from "./customMdGenerators";

// Constants
const organizationName = "ceems-dev";
const projectName = "ceems";

// Global variables that will be replaced
// in markdown files
let globalVars = {
  goVersion: "1.24.x", // Everytime Go version is updated, this needs to be updated as well
  ceemsVersion: "",
  ceemsOrg: organizationName,
  ceemsRepo: projectName,
}

// Remove API sidebar items
function removeApiSidebarItems(items) {
  return items.filter(item => !(item.label === 'api'));
}

// Setup versions
exec('git tag --sort=committerdate | tail -1', (error, stdout) => {
  if (error) {
    console.error(`git tag exec error: ${error}`);
    return;
  }

  globalVars.ceemsVersion = stdout.trim().split("v")[1];
});

const config: Config = {
  title: "Compute Energy & Emissions Monitoring Stack (CEEMS)",
  tagline:
    "Monitor the energy consumption and carbon footprint of your workloads in realtime",
  favicon: "img/favicon.ico",

  // Set the production url of your site here
  url: `https://${organizationName}.github.io`,
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: `/${projectName}/`,

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: `${organizationName}`, // Usually your GitHub org/user name.
  projectName: `${projectName}`, // Usually your repo name.

  // NOTE: Set it to throw once docs are "release" ready
  onBrokenLinks: "warn",
  onBrokenMarkdownLinks: "warn",

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  presets: [
    [
      "classic",
      {
        docs: {
          sidebarPath: "./sidebars.ts",
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl: `https://github.com/${organizationName}/${projectName}/tree/main/website`,
          docItemComponent: "@theme/ApiItem", // Derived from docusaurus-theme-openapi
          async sidebarItemsGenerator({defaultSidebarItemsGenerator, ...args}) {
            const sidebarItems = await defaultSidebarItemsGenerator(args);
            return removeApiSidebarItems(sidebarItems);
          },
        },
        blog: {
          showReadingTime: true,
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl: `https://github.com/${organizationName}/${projectName}/tree/main/website`,
        },
        theme: {
          customCss: "./src/css/custom.css",
        },
        // pages: {
        //   include: [
        //     "**/*.{js,jsx,ts,tsx,md,mdx}",
        //     "**/*.{yaml,yml}",
        //   ]
        // },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    // // Custom plugin to modify Webpack config
    // function myCustomPlugin(context, options) {
    //   return {
    //     name: 'custom-webpack-plugin',
    //     configureWebpack(config, isServer, utils, content) {
    //       return {
    //         module: {
    //          rules: [
    //            {
    //              test: /\.yaml$/,
    //              use: 'yaml-loader'
    //            }
    //          ]
    //         },
    //       };
    //     },
    //   };
    // },
    [
      'docusaurus-plugin-openapi-docs',
      {
        id: "api", // plugin id
        docsPluginId: "classic", // configured for preset-classic
        config: {
          ceems: {
            specPath: "../pkg/api/http/docs/swagger.yaml",
            outputDir: "docs/api",
            sidebarOptions: {
              groupPathsBy: "tag",
              categoryLinkSource: "tag",
            },
            template: "api.mustache", // Customize API MDX with mustache template
            // showSchemas: true,
            markdownGenerators: { createApiPageMD: myCustomApiMdGenerator }, // customize MDX with markdown generator
          } satisfies OpenApiPlugin.Options,
        }
      },
    ]
  ],

  markdown: {
    preprocessor: ({ filePath, fileContent }) => {
      let content = fileContent;

      // Replace globalVars in files
      for (const [key, value] of Object.entries(globalVars)) {
        content = content.replaceAll(
          "@" + key + "@",
          value
        );
      }

      return content;
    },
  },

  themes: ["docusaurus-theme-openapi-docs"], // export theme components

  themeConfig: {
    // Replace with your project's social card
    image: "img/docusaurus-social-card.jpg",
    docs: {
      sidebar: {
        // Make sidebar hideable
        hideable: true,
        // Collapse all sibling categories when expanding one category
        autoCollapseCategories: true,
      },
    },
    navbar: {
      title: "CEEMS",
      logo: {
        alt: "CEEMS Logo",
        src: "img/logo.svg",
      },
      items: [
        {
          type: "doc",
          docId: "intro",
          position: "left",
          label: "Documentation",
        },
        {
          type: "docSidebar",
          sidebarId: "ceemsApiSidebar",
          position: "left",
          label: "API",
        },
        {
          href: `https://ceems-demo.myaddr.tools`,
          label: "Demo",
          position: "right",
        },
        {
          href: `https://github.com/${organizationName}/${projectName}`,
          label: "GitHub",
          position: "right",
        },
      ],
    },
    footer: {
      style: "dark",
      links: [
        {
          title: "Docs",
          items: [
            {
              label: "CEEMS Docs",
              to: "/docs/",
            },
            {
              label: "Prometheus Docs",
              href: `https://prometheus.io/docs/introduction/overview/`,
            },
            {
              label: "Grafana Docs",
              href: `https://grafana.com/docs/`,
            },
          ],
        },
        {
          title: "Repository",
          items: [
            {
              label: "Issues",
              href: `https://github.com/${organizationName}/${projectName}/issues`,
            },
            {
              label: "Pull Requests",
              href: `https://github.com/${organizationName}/${projectName}/pulls`,
            },
            {
              label: "Discussions",
              href: `https://github.com/${organizationName}/${projectName}/discussions`,
            },
          ],
        },

        {
          title: "More",
          items: [
            {
              label: "Demo",
              href: `https://ceems-demo.myaddr.tools`,
            },
            {
              label: "GitHub",
              href: `https://github.com/${organizationName}/${projectName}`,
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()}. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: [
        "ruby",
        "csharp",
        "php",
        "java",
        "powershell",
        "json",
        "bash",
        "dart",
        "objectivec",
        "r",
      ],
    },
    languageTabs: [
      {
        highlight: "python",
        language: "python",
        logoClass: "python",
      },
      {
        highlight: "bash",
        language: "curl",
        logoClass: "curl",
      },
      {
        highlight: "csharp",
        language: "csharp",
        logoClass: "csharp",
      },
      {
        highlight: "go",
        language: "go",
        logoClass: "go",
      },
      {
        highlight: "javascript",
        language: "nodejs",
        logoClass: "nodejs",
      },
      {
        highlight: "ruby",
        language: "ruby",
        logoClass: "ruby",
      },
      {
        highlight: "php",
        language: "php",
        logoClass: "php",
      },
      {
        highlight: "java",
        language: "java",
        logoClass: "java",
        variant: "unirest",
      },
      {
        highlight: "powershell",
        language: "powershell",
        logoClass: "powershell",
      },
      {
        highlight: "dart",
        language: "dart",
        logoClass: "dart",
      },
      {
        highlight: "javascript",
        language: "javascript",
        logoClass: "javascript",
      },
      {
        highlight: "c",
        language: "c",
        logoClass: "c",
      },
      {
        highlight: "objective-c",
        language: "objective-c",
        logoClass: "objective-c",
      },
      {
        highlight: "ocaml",
        language: "ocaml",
        logoClass: "ocaml",
      },
      {
        highlight: "r",
        language: "r",
        logoClass: "r",
      },
      {
        highlight: "swift",
        language: "swift",
        logoClass: "swift",
      },
      {
        highlight: "kotlin",
        language: "kotlin",
        logoClass: "kotlin",
      },
      {
        highlight: "rust",
        language: "rust",
        logoClass: "rust",
      },
    ],
    algolia: {
      // The application ID provided by Algolia
      appId: 'KQ58EXUULN',
      // Public API key: it is safe to commit it
      apiKey: '16e0dc2efb99e9d654683fd9b9602082',
      indexName: 'mahendrapaipuriio',
      // // Optional: Replace parts of the item URLs from Algolia. Useful when using the same search index for multiple deployments using a different baseUrl. You can use regexp or string in the `from` param. For example: localhost:3000 vs myCompany.com/docs
      replaceSearchResultPathname: {
        from: '/ceems/',
        to: '/',
      },
      // We may need to tune `contextualSearch` and `searchParameters` to handle search for versioned docs
      // Optional: see doc section -- https://docusaurus.io/docs/search#contextual-search
      contextualSearch: false,
      // Optional: Algolia search parameters
      // searchParameters: {},
      // Optional: path for search page that enabled by default (`false` to disable it)
      searchPagePath: 'search',
    },

  } satisfies Preset.ThemeConfig,
};

export default config;
