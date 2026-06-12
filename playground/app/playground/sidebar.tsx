"use client"

import * as React from "react"
import {
  Sidebar as SidebarPrimitive,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import {
  BotIcon,
  GlobeIcon,
  MapIcon,
  MoonIcon,
  SearchIcon,
  SunIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { useTheme } from "next-themes"
import type { Endpoint } from "@/lib/api-types"

interface AppSidebarProps {
  activeEndpoint: Endpoint
  onEndpointChange: (endpoint: Endpoint) => void
  health: { status: string } | null
}

const endpoints: {
  id: Endpoint
  label: string
  description: string
  icon: typeof BotIcon
}[] = [
  {
    id: "scrape",
    label: "Scrape",
    description: "Single-page extraction",
    icon: BotIcon,
  },
  {
    id: "crawl",
    label: "Crawl",
    description: "Multi-page traversal",
    icon: GlobeIcon,
  },
  {
    id: "map",
    label: "Map",
    description: "Discover URLs",
    icon: MapIcon,
  },
  {
    id: "search",
    label: "Search",
    description: "Search and scrape results",
    icon: SearchIcon,
  },
]

export function AppSidebar({
  activeEndpoint,
  onEndpointChange,
  health,
}: AppSidebarProps) {
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = React.useState(false)

  React.useEffect(() => {
    setMounted(true)
  }, [])

  const logoSrc = !mounted
    ? "/qc.svg"
    : resolvedTheme === "dark"
      ? "/qc-dark.svg"
      : "/qc.svg"

  return (
    <SidebarPrimitive collapsible="icon" className="border-r-2 border-border">
      <SidebarHeader className="border-b-2 border-border bg-secondary-background">
        <div className="flex items-center gap-3 px-3 py-3">
          <img
            src={logoSrc}
            alt="QuickCrawl"
            className="h-10 w-auto object-contain"
          />
        </div>
      </SidebarHeader>

      <SidebarContent className="px-2 pt-2">
        <SidebarGroup className="gap-4 border-b-0">
          <SidebarGroupContent>
            <SidebarMenu className="gap-3 px-2 pt-3 pb-4">
              {endpoints.map(({ id, label, description, icon: Icon }) => (
                <SidebarMenuItem key={id}>
                  <SidebarMenuButton
                    isActive={activeEndpoint === id}
                    size="lg"
                    onClick={(e) => {
                      e.stopPropagation()
                      onEndpointChange(id)
                    }}
                    tooltip={label}
                    className="h-auto min-h-14 justify-start gap-3 rounded-base border-2 border-border bg-background px-3 py-3 text-left shadow-shadow transition-all data-[active=true]:translate-x-boxShadowX data-[active=true]:translate-y-boxShadowY data-[active=true]:shadow-none data-[active=true]:[&_.sidebar-subtitle]:text-main-foreground/80"
                  >
                    <span className="flex h-8 w-8 items-center justify-center rounded-base border-2 border-border bg-secondary-background text-foreground">
                      <Icon className="h-4 w-4" />
                    </span>
                    <span className="flex min-w-0 flex-1 flex-col items-start">
                      <span className="truncate text-sm font-bold font-heading">
                        {label}
                      </span>
                      <span className="sidebar-subtitle text-muted-foreground truncate text-xs">
                        {description}
                      </span>
                    </span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter className="border-t-2 border-border bg-secondary-background">
        <div className="flex flex-col gap-4 p-3">
          <div className="flex items-center gap-2 rounded-base border-2 border-border bg-background px-3 py-2 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
            <div
              className={`h-2.5 w-2.5 shrink-0 rounded-full ${
                health ? "bg-green-500" : "bg-red-500"
              }`}
            />
            <span className="text-xs text-foreground/70 group-data-[collapsible=icon]:hidden">
              {health ? "Server OK" : "Offline"}
            </span>
          </div>
          <Button
            variant="noShadow"
            size="sm"
            className="w-full justify-center border-2 border-border shadow-shadow group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:p-0"
            onClick={() =>
              setTheme(resolvedTheme === "dark" ? "light" : "dark")
            }
          >
            {mounted && resolvedTheme === "dark" ? (
              <SunIcon className="mr-2 h-4 w-4 group-data-[collapsible=icon]:mr-0" />
            ) : (
              <MoonIcon className="mr-2 h-4 w-4 group-data-[collapsible=icon]:mr-0" />
            )}
            <span className="group-data-[collapsible=icon]:hidden">
              {mounted && resolvedTheme === "dark" ? "Light Mode" : "Dark Mode"}
            </span>
          </Button>
          <Button
            variant="noShadow"
            size="sm"
            className="w-full justify-center border-2 border-border shadow-shadow group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:p-0"
            asChild
          >
            <a
              href="https://github.com/MabudAlam/quickcrawl"
              target="_blank"
              rel="noopener noreferrer"
            >
              <svg
                className="mr-2 h-4 w-4 group-data-[collapsible=icon]:mr-0"
                viewBox="0 0 24 24"
                fill="currentColor"
              >
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
              </svg>
              <span className="group-data-[collapsible=icon]:hidden">
                GitHub
              </span>
            </a>
          </Button>
        </div>
      </SidebarFooter>
    </SidebarPrimitive>
  )
}