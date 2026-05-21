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
  Search,
} from "lucide-react";
import { cn } from "@/lib/utils";
import SearchBar from "./SearchBar";
import { CommandPaletteProvider, useCommandPalette } from "./CommandPalette";

export default function Layout() {
  return (
    <CommandPaletteProvider>
      <LayoutInner />
    </CommandPaletteProvider>
  );
}

function LayoutInner() {
  const loc = useLocation();
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
            <CmdKButton />
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
            {/*
              No "Admin" tab here. The user portal is strictly user-facing;
              admins reach the plugin's admin UI via the continuum host
              sidebar (Apps → Books → Ebooks → [admin]). Mixing the two
              surfaces in this nav blurred the audience and is what users
              flagged as "user side has admin functions."
            */}
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

function CmdKButton() {
  const { open } = useCommandPalette();
  return (
    <button
      type="button"
      onClick={open}
      title="Search (Cmd-K)"
      className="text-muted-foreground hover:bg-surface-hover hover:text-foreground inline-flex min-h-9 items-center justify-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
    >
      <Search className="size-4" />
      <kbd className="border-border hidden rounded border px-1.5 py-0.5 text-xs sm:inline">
        ⌘K
      </kbd>
    </button>
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
