"use client";

import * as React from "react";
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
} from "@/components/ui/sidebar";
import { BotIcon, GlobeIcon, MapIcon, MoonIcon, SearchIcon, SunIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTheme } from "next-themes";
import type { Endpoint } from "@/lib/api-types";

interface AppSidebarProps {
  activeEndpoint: Endpoint;
  onEndpointChange: (endpoint: Endpoint) => void;
  health: { status: string } | null;
}

const endpoints: { id: Endpoint; label: string; icon: typeof BotIcon }[] = [
  { id: "scrape", label: "Scrape", icon: BotIcon },
  { id: "crawl", label: "Crawl", icon: GlobeIcon },
  { id: "map", label: "Map", icon: MapIcon },
  { id: "search", label: "Search", icon: SearchIcon },
];

export function AppSidebar({ activeEndpoint, onEndpointChange, health }: AppSidebarProps) {
  const { resolvedTheme, setTheme } = useTheme();
  const [mounted, setMounted] = React.useState(false);

  React.useEffect(() => {
    setMounted(true);
  }, []);

  if (!mounted) {
    return (
      <SidebarPrimitive collapsible="icon" className="border-r-2 border-border">
        <SidebarHeader className="border-b-2 border-border">
          <div className="flex items-center gap-2 px-2 py-3">
            <div className="flex items-center justify-center w-8 h-8 rounded-base bg-main text-main-foreground shrink-0">
              <BotIcon className="w-5 h-5" />
            </div>
            <div className="flex flex-col group-data-[collapsible=icon]:hidden">
              <span className="text-sm font-heading font-bold leading-tight">quickcrawl</span>
              <span className="text-xs text-muted-foreground leading-tight">Playground</span>
            </div>
          </div>
        </SidebarHeader>
      </SidebarPrimitive>
    );
  }

  return (
    <SidebarPrimitive collapsible="icon" className="border-r-2 border-border">
      <SidebarHeader className="border-b-2 border-border">
        <div className="flex items-center gap-2 px-2 py-3">
          <div className="flex items-center justify-center w-8 h-8 rounded-base bg-main text-main-foreground shrink-0">
            <BotIcon className="w-5 h-5" />
          </div>
          <div className="flex flex-col group-data-[collapsible=icon]:hidden">
            <span className="text-sm font-heading font-bold leading-tight">quickcrawl</span>
            <span className="text-xs text-muted-foreground leading-tight">Playground</span>
          </div>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {endpoints.map(({ id, label, icon: Icon }) => (
                <SidebarMenuItem key={id}>
                  <SidebarMenuButton
                    isActive={activeEndpoint === id}
                    onClick={(e) => { e.stopPropagation(); onEndpointChange(id); }}
                    tooltip={label}
                  >
                    <Icon className="w-4 h-4" />
                    <span>{label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <div className="flex flex-col gap-2 p-2">
          <div className="flex items-center gap-2 px-2 py-1.5 rounded-base bg-secondary-background group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
            <div
              className={`w-2 h-2 rounded-full shrink-0 ${health ? "bg-green-500" : "bg-red-500"}`}
            />
            <span className="text-xs text-foreground/70 group-data-[collapsible=icon]:hidden">
              {health ? "Server OK" : "Offline"}
            </span>
          </div>
          <Button
            variant="noShadow"
            size="sm"
            className="w-full justify-center group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:p-0"
            onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
          >
            {resolvedTheme === "dark" ? (
              <SunIcon className="w-4 h-4 group-data-[collapsible=icon]:mr-0 mr-2" />
            ) : (
              <MoonIcon className="w-4 h-4 group-data-[collapsible=icon]:mr-0 mr-2" />
            )}
            <span className="group-data-[collapsible=icon]:hidden">
              {resolvedTheme === "dark" ? "Light Mode" : "Dark Mode"}
            </span>
          </Button>
          <Button
            variant="noShadow"
            size="sm"
            className="w-full justify-center group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:p-0"
            asChild
          >
            <a
              href="https://github.com/MabudAlam/quickcrawl"
              target="_blank"
              rel="noopener noreferrer"
            >
              <svg className="w-4 h-4 group-data-[collapsible=icon]:mr-0 mr-2" viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
              </svg>
              <span className="group-data-[collapsible=icon]:hidden">GitHub</span>
            </a>
          </Button>
        </div>
      </SidebarFooter>
    </SidebarPrimitive>
  );
}