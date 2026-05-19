import { Outlet, NavLink, Link, useLocation } from "react-router";
import {
  ArrowLeft,
  Compass,
  Library,
  BookOpen,
  Users,
  Tag,
  Bookmark,
  Send,
  Smartphone,
  Shield,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { currentUser } from "@/lib/identity";
import SearchBar from "./SearchBar";

export default function Layout() {
  const loc = useLocation();
  const user = currentUser();
  const isAdminRoute = loc.pathname.startsWith("/admin");
  const continuumHomeHref = isAdminRoute ? "/admin/plugins" : "/";
  const continuumHomeTitle = isAdminRoute
    ? "Back to Continuum plugins"
    : "Back to Continuum";
  return (
    <div className="bg-background relative min-h-[100dvh] overflow-x-hidden text-foreground">
      <div className="from-primary/6 pointer-events-none fixed inset-x-0 top-0 z-0 h-48 bg-gradient-to-b to-transparent blur-3xl" />
      <header className="glass-dark border-border/70 sticky top-0 z-30 mx-3 mt-3 rounded-2xl border px-4 py-3 sm:mx-6 lg:mx-8">
        <div className="flex flex-wrap items-center gap-3">
          <a
            href={continuumHomeHref}
            className="text-muted-foreground hover:bg-surface-hover hover:text-foreground inline-flex min-h-9 min-w-9 items-center justify-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors"
            title={continuumHomeTitle}
          >
            <ArrowLeft className="size-4" />
            <span className="hidden sm:inline">Continuum</span>
          </a>
          <span className="text-border/60" aria-hidden>
            /
          </span>
          <Link
            to="/"
            className="inline-flex min-h-9 items-center text-base font-semibold tracking-tight"
          >
            Ebooks
          </Link>
          <nav className="flex items-center gap-1 flex-wrap">
            <NavTab to="/" end icon={<Compass className="size-4" />}>
              Home
            </NavTab>
            <NavTab to="/library" icon={<Library className="size-4" />}>
              Library
            </NavTab>
            <NavTab to="/series" icon={<BookOpen className="size-4" />}>
              Series
            </NavTab>
            <NavTab to="/authors" icon={<Users className="size-4" />}>
              Authors
            </NavTab>
            <NavTab to="/genres" icon={<Tag className="size-4" />}>
              Genres
            </NavTab>
            <NavTab to="/collections" icon={<Bookmark className="size-4" />}>
              Collections
            </NavTab>
            <NavTab to="/me/requests" icon={<Send className="size-4" />}>
              Requests
            </NavTab>
            <NavTab to="/apps" icon={<Smartphone className="size-4" />}>
              Apps
            </NavTab>
            {user?.is_admin && (
              <NavTab to="/admin" icon={<Shield className="size-4" />}>
                Admin
              </NavTab>
            )}
          </nav>
          <div className="ml-auto w-full max-w-md sm:w-80 md:w-96">
            <SearchBar />
          </div>
        </div>
      </header>
      <main className="relative z-10 pb-12 pt-2">
        <div className="mx-auto max-w-[1600px] space-y-6 px-4 md:px-6 lg:px-8">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

function NavTab({
  to,
  end,
  icon,
  children,
}: {
  to: string;
  end?: boolean;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          "inline-flex min-h-9 items-center justify-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors",
          isActive
            ? "bg-surface text-foreground"
            : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
        )
      }
    >
      {icon}
      {children}
    </NavLink>
  );
}
