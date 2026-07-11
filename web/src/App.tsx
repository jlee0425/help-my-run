import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { BrowserRouter, Route, Routes } from 'react-router';
import { setOnUnauthorized } from './api/client';
import { useAuthState } from './api/hooks';
import { LoginPage } from './pages/LoginPage';
import { OnboardingPage } from './pages/onboarding/OnboardingPage';
import { TodayPage } from './pages/TodayPage';
import { TrendsPage } from './pages/TrendsPage';
import { CoachPage } from './pages/CoachPage';
import { RunDetailPage } from './pages/RunDetailPage';
import { PlanPage } from './pages/PlanPage';
import { SettingsPage } from './pages/SettingsPage';
import { Shell } from './shell/Shell';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 15_000, refetchOnWindowFocus: true },
    mutations: { retry: 0 },
  },
});

function Splash() {
  return (
    <div
      style={{
        minHeight: '100dvh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <span
        className="mono-label mono-label--green pulse-dot"
        style={{
          width: 'auto',
          height: 'auto',
          borderRadius: 0,
          fontSize: 11,
          letterSpacing: '.26em',
        }}
      >
        HELP MY RUN
      </span>
    </div>
  );
}

function Gate() {
  const qc = useQueryClient();
  const { data: auth, isLoading } = useAuthState();

  useEffect(() => {
    setOnUnauthorized(() => {
      void qc.invalidateQueries({ queryKey: ['auth'] });
    });
    return () => setOnUnauthorized(null);
  }, [qc]);

  if (isLoading || !auth) return <Splash />;
  if (auth.setup_required) return <OnboardingPage />;
  if (!auth.authed) return <LoginPage />;

  return (
    <Routes>
      <Route path="/onboarding" element={<OnboardingPage />} />
      <Route
        path="*"
        element={
          <Shell>
            <Routes>
              <Route path="/" element={<TodayPage />} />
              <Route path="/trends" element={<TrendsPage />} />
              <Route path="/coach" element={<CoachPage />} />
              <Route path="/runs/:id" element={<RunDetailPage />} />
              <Route path="/plan" element={<PlanPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="*" element={<TodayPage />} />
            </Routes>
          </Shell>
        }
      />
    </Routes>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Gate />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
