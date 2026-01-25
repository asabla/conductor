/**
 * User menu dropdown component
 */

import { Link } from "@tanstack/react-router";
import { User as UserIcon, LogOut, Settings, Shield, Eye, Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Badge } from "@/components/ui/badge";
import { useAuth } from "@/hooks/use-auth";
import type { UserRole } from "@/types/auth";
import { cn } from "@/lib/utils";

/**
 * Role badge styling and icons
 */
const roleConfig: Record<UserRole, { label: string; variant: "default" | "secondary" | "outline"; icon: typeof Shield }> = {
  admin: {
    label: "Admin",
    variant: "default",
    icon: Shield,
  },
  operator: {
    label: "Operator",
    variant: "secondary",
    icon: Wrench,
  },
  viewer: {
    label: "Viewer",
    variant: "outline",
    icon: Eye,
  },
};

/**
 * Get the highest role from user's roles (admin > operator > viewer)
 */
function getHighestRole(roles: UserRole[]): UserRole {
  if (roles.includes("admin")) return "admin";
  if (roles.includes("operator")) return "operator";
  return "viewer";
}

interface UserMenuProps {
  /** Additional class names */
  className?: string;
}

/**
 * User dropdown menu showing user info and logout option
 */
export function UserMenu({ className }: UserMenuProps) {
  const { user, isAuthenticated, logout } = useAuth();

  if (!isAuthenticated || !user) {
    return (
      <Button variant="ghost" size="sm" asChild className={className}>
        <Link to="/login">Sign in</Link>
      </Button>
    );
  }

  const highestRole = getHighestRole(user.roles);
  const roleInfo = roleConfig[highestRole];
  const RoleIcon = roleInfo.icon;

  // Get user initials for avatar
  const initials = user.name
    .split(" ")
    .map((n) => n[0])
    .join("")
    .toUpperCase()
    .slice(0, 2);

  const handleLogout = () => {
    logout().catch((error) => {
      console.error("Logout failed:", error);
    });
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          className={cn("flex items-center gap-2 px-2", className)}
          aria-label="User menu"
        >
          {/* Avatar */}
          {user.picture ? (
            <img
              src={user.picture}
              alt={user.name}
              className="h-8 w-8 rounded-full object-cover"
            />
          ) : (
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
              {initials}
            </div>
          )}

          {/* Name and role - hidden on small screens */}
          <div className="hidden flex-col items-start md:flex">
            <span className="text-sm font-medium">{user.name}</span>
            <span className="text-xs text-muted-foreground">{user.email}</span>
          </div>
        </Button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="end" className="w-64">
        {/* User info header */}
        <DropdownMenuLabel className="font-normal">
          <div className="flex flex-col space-y-2">
            <div className="flex items-center gap-3">
              {user.picture ? (
                <img
                  src={user.picture}
                  alt={user.name}
                  className="h-10 w-10 rounded-full object-cover"
                />
              ) : (
                <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
                  {initials}
                </div>
              )}
              <div className="flex flex-col">
                <span className="font-medium">{user.name}</span>
                <span className="text-xs text-muted-foreground">{user.email}</span>
              </div>
            </div>

            {/* Role badge */}
            <div className="flex items-center gap-2">
              <Badge variant={roleInfo.variant} className="gap-1">
                <RoleIcon className="h-3 w-3" />
                {roleInfo.label}
              </Badge>
              {user.roles.length > 1 && (
                <span className="text-xs text-muted-foreground">
                  +{user.roles.length - 1} more
                </span>
              )}
            </div>
          </div>
        </DropdownMenuLabel>

        <DropdownMenuSeparator />

        {/* Menu items */}
        <DropdownMenuItem asChild>
          <Link to="/settings" className="flex items-center">
            <UserIcon className="mr-2 h-4 w-4" />
            Profile
          </Link>
        </DropdownMenuItem>

        <DropdownMenuItem asChild>
          <Link to="/settings" className="flex items-center">
            <Settings className="mr-2 h-4 w-4" />
            Settings
          </Link>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        {/* Logout */}
        <DropdownMenuItem
          onClick={handleLogout}
          className="text-destructive focus:text-destructive"
        >
          <LogOut className="mr-2 h-4 w-4" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
