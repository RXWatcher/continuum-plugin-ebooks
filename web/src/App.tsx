import { Suspense, lazy, useEffect, useState } from "react";
import { Routes, Route, useLocation } from "react-router";
import Layout from "./components/Layout";
import { Toaster } from "@/components/ui/sonner";
import { loadIdentity } from "@/lib/identity";

const Home = lazy(() => import("./pages/Home"));
const Library = lazy(() => import("./pages/Library"));
const BookDetail = lazy(() => import("./pages/BookDetail"));
const Reader = lazy(() => import("./pages/Reader"));
const MyRequests = lazy(() => import("./pages/MyRequests"));
const RequestDetail = lazy(() => import("./pages/RequestDetail"));
const Submit = lazy(() => import("./pages/Submit"));
const Collections = lazy(() => import("./pages/Collections"));
const CollectionDetail = lazy(() => import("./pages/CollectionDetail"));
const Apps = lazy(() => import("./pages/Apps"));
const Admin = lazy(() => import("./pages/Admin"));
const Search = lazy(() => import("./pages/Search"));
const Authors = lazy(() => import("./pages/Authors"));
const SeriesPage = lazy(() => import("./pages/Series"));
const Genres = lazy(() => import("./pages/Genres"));

export default function App() {
  // Resolve identity once before mounting routes. is_admin gates Admin link.
  const [ready, setReady] = useState(false);
  useEffect(() => {
    loadIdentity().finally(() => setReady(true));
  }, []);
  if (!ready) return null;
  return (
    <>
      <ScrollToTop />
      <Suspense fallback={null}>
        <Routes>
          <Route
            path="/admin"
            element={
              <main className="min-h-[100dvh] bg-background text-foreground">
                <div className="mx-auto max-w-[1600px] space-y-6 px-4 py-6 md:px-6 lg:px-8">
                  <a
                    href="/admin/plugins"
                    className="text-muted-foreground hover:bg-surface-hover hover:text-foreground inline-flex min-h-9 items-center justify-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors"
                    title="Back to Continuum plugins"
                  >
                    Back to Continuum plugins
                  </a>
                  <Admin />
                </div>
              </main>
            }
          />
          <Route element={<Layout />}>
            <Route index element={<Home />} />
            <Route path="/library" element={<Library />} />
            <Route path="/series" element={<SeriesPage />} />
            <Route path="/authors" element={<Authors />} />
            <Route path="/genres" element={<Genres />} />
            <Route path="/collections" element={<Collections />} />
            <Route path="/collections/:id" element={<CollectionDetail />} />
            <Route path="/me/requests" element={<MyRequests />} />
            <Route path="/me/requests/:id" element={<RequestDetail />} />
            <Route path="/me/submit" element={<Submit />} />
            <Route path="/apps" element={<Apps />} />
            <Route path="/search" element={<Search />} />
            <Route path="/:id" element={<BookDetail />} />
            <Route path="/:id/read" element={<Reader />} />
          </Route>
        </Routes>
      </Suspense>
      <Toaster />
    </>
  );
}

function ScrollToTop() {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo({ top: 0, left: 0, behavior: "auto" });
  }, [pathname]);
  return null;
}
