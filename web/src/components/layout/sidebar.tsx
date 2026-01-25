import { Link, useRouterState } from "@tanstack/react-router";
import type { LucideIcon } from "lucide-react";
import {
  LayoutDashboard,
  PlayCircle,
  Server,
  Settings,
  Zap,
  GitBranch,
  AlertCircle,
  Shuffle,
  TrendingUp,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { ScrollArea } from "@/components/ui/scroll-area";
import { NavItem } from "./nav-item";

interface NavigationSection {
  title?: string;
  items: NavigationItem[];
}

interface NavigationItem {
  name: string;
  href: string;
  icon: LucideIcon;
}

const navigationSections: NavigationSection[] = [
  {
    items: [
      {
        name: "Dashboard",
        href: "/",
        icon: LayoutDashboard,
      },
      {
        name: "Test Runs",
        href: "/test-runs",
        icon: PlayCircle,
      },
      {
        name: "Services",
        href: "/services",
        icon: GitBranch,
      },
      {
        name: "Agents",
        href: "/agents",
        icon: Server,
      },
    ],
  },
  {
    title: "Analytics",
    items: [
      {
        name: "Failure Analysis",
        href: "/failure-analysis",
        icon: AlertCircle,
      },
      {
        name: "Flaky Tests",
        href: "/flaky-tests",
        icon: Shuffle,
      },
      {
        name: "Trends",
        href: "/trends",
        icon: TrendingUp,
      },
    ],
  },
  {
    items: [
      {
        name: "Settings",
        href: "/settings",
        icon: Settings,
      },
    ],
  },
];

export function Sidebar() {
  const router = useRouterState();
  const currentPath = router.location.pathname;

  return (
    <aside className="hidden w-64 flex-shrink-0 border-r bg-card lg:block">
      <div className="flex h-full flex-col">
        {/* Logo */}
        <div className="flex h-16 items-center gap-2 border-b px-6">
          <Link to="/" className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary">
              <Zap className="h-5 w-5 text-primary-foreground" />
            </div>
            <span className="text-xl font-bold">Conductor</span>
          </Link>
        </div>

        {/* Navigation */}
        <ScrollArea className="flex-1 px-3 py-4">
          <nav className="space-y-6">
            {navigationSections.map((section, sectionIndex) => (
              <div key={sectionIndex}>
                {section.title && (
                  <h3 className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    {section.title}
                  </h3>
                )}
                <div className="space-y-1">
                  {section.items.map((item) => {
                    const isActive =
                      item.href === "/"
                        ? currentPath === "/"
                        : currentPath.startsWith(item.href);

                    return (
                      <NavItem
                        key={item.name}
                        href={item.href}
                        icon={item.icon}
                        isActive={isActive}
                      >
                        {item.name}
                      </NavItem>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>
        </ScrollArea>

        {/* Footer */}
        <div className="border-t p-4">
          <div className="rounded-lg bg-muted/50 p-3">
            <div className="flex items-center gap-2">
              <div className={cn("h-2 w-2 rounded-full bg-success")} />
              <span className="text-sm text-muted-foreground">
                System healthy
              </span>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
}
