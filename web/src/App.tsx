import { useEffect, useState } from "react";
import { Routes, Route, useLocation } from "react-router";
import Home from "./pages/Home";
import Library from "./pages/Library";
import BookDetail from "./pages/BookDetail";
import Reader from "./pages/Reader";
import MyRequests from "./pages/MyRequests";
import RequestDetail from "./pages/RequestDetail";
import Submit from "./pages/Submit";
import Collections from "./pages/Collections";
import CollectionDetail from "./pages/CollectionDetail";
import Apps from "./pages/Apps";
import Admin from "./pages/Admin";
import Search from "./pages/Search";
import Authors from "./pages/Authors";
import SeriesPage from "./pages/Series";
import Genres from "./pages/Genres";
import Layout from "./components/Layout";
import { Toaster } from "@/components/ui/sonner";
import { loadIdentity } from "@/lib/identity";

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
      <Routes>
        <Route
          path="/admin"
          element={
            <main className="min-h-[100dvh] bg-background text-foreground">
              <div className="mx-auto max-w-[1600px] space-y-6 px-4 py-6 md:px-6 lg:px-8">
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
