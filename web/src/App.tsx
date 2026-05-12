import { useEffect, useState } from 'react';
import { Routes, Route, useLocation } from 'react-router';
import Home from './pages/Home';
import Library from './pages/Library';
import BookDetail from './pages/BookDetail';
import Reader from './pages/Reader';
import MyRequests from './pages/MyRequests';
import Submit from './pages/Submit';
import Collections from './pages/Collections';
import Apps from './pages/Apps';
import Admin from './pages/Admin';
import Search from './pages/Search';
import Placeholder from './pages/Placeholder';
import Layout from './components/Layout';
import { Toaster } from '@/components/ui/sonner';
import { loadIdentity } from '@/lib/identity';

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
        <Route element={<Layout />}>
          <Route index element={<Home />} />
          <Route path="/library" element={<Library />} />
          <Route path="/series" element={<Placeholder name="Series" />} />
          <Route path="/authors" element={<Placeholder name="Authors" />} />
          <Route path="/genres" element={<Placeholder name="Genres" />} />
          <Route path="/collections" element={<Collections />} />
          <Route path="/me/requests" element={<MyRequests />} />
          <Route path="/me/submit" element={<Submit />} />
          <Route path="/apps" element={<Apps />} />
          <Route path="/admin" element={<Admin />} />
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
    window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
  }, [pathname]);
  return null;
}
