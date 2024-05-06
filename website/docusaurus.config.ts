import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";
import type * as Redocusaurus from "redocusaurus";

// Constants
const organizationName = "mahendrapaipuri";
const projectName = "ceems";

const config: Config = {
  title: "Compute Energy & Emissions Monitoring Stack (CEEMS)",
  tagline:
    "Monitor the energy consumption and equivalent emissions of your workloads in realtime",
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

  onBrokenLinks: "throw",
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
          editUrl: `https://github.com/${organizationName}/${projectName}/tree/main/`,
        },
        blog: {
          showReadingTime: true,
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl: `https://github.com/${organizationName}/${projectName}/tree/main/`,
        },
        theme: {
          customCss: "./src/css/custom.css",
        },
      } satisfies Preset.Options,
    ],
    // Redocusaurus config
    [
      "redocusaurus",
      {
        // Plugin Options for loading OpenAPI files
        specs: [
          {
            // Redocusaurus will automatically bundle your spec into a single file during the build
            spec: "../pkg/api/http/docs/swagger.yaml",
            route: "/api/",
          },
        ],
        // Theme Options for modifying how redoc renders them
        theme: {
          // Change with your site colors
          primaryColor: "#3cc9beff",
        },
      },
    ] satisfies Redocusaurus.PresetEntry,
  ],

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
          type: "docSidebar",
          sidebarId: "ceemsSidebar",
          position: "left",
          label: "Documentation",
        },
        { to: "/api", label: "API", position: "left" },
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
              label: "Docs",
              to: "/docs/",
            },
            {
              label: "Objectives",
              to: "/docs/category/objectives",
            },
            {
              label: "Installation",
              to: "/docs/category/installation",
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
              label: "GitHub",
              href: `https://github.com/${organizationName}/${projectName}`,
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
      ],
      copyright: `Copyright © ${new Date().getFullYear()}. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
