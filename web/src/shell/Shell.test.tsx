import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import { Shell } from './Shell';

const mocks = vi.hoisted(() => ({
  authState: { data: { setup_required: false, authed: true, demo: false } },
}));

vi.mock('../api/hooks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/hooks')>()),
  useAuthState: () => mocks.authState,
  useStatus: () => ({
    data: {
      garmin: { connected: true, last_synced_at: null, last_run_at: null, status: 'ok', error: null },
      counts: { activities: 0, recovery_days: 0 },
      agent_next_run: '2026-07-10T06:00:00Z',
      manual_sync: false,
      agent_enabled: true,
    },
  }),
}));

vi.mock('../api/today', () => ({
  useToday: () => ({
    data: {
      date: '2026-07-09',
      readiness_color: 'amber',
      drivers: {},
      reasons: [],
      action: 'SOFTEN',
      original_session: null,
      effective_session: null,
      rationale: '',
      source: 'ai',
      stale: false,
    },
  }),
}));

function setDesktop(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

function renderShell() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0 } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/']}>
        <Shell>
          <div>page-content</div>
        </Shell>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('Shell', () => {
  it('renders mobile tab bar below 1024px', () => {
    setDesktop(false);
    renderShell();
    expect(screen.getByText('TODAY')).toBeInTheDocument();
    expect(screen.getByText('TRENDS')).toBeInTheDocument();
    expect(screen.getByText('COACH')).toBeInTheDocument();
    expect(screen.getByText('page-content')).toBeInTheDocument();
    expect(screen.queryByText('HELP MY RUN')).not.toBeInTheDocument();
  });

  it('renders desktop rail + readiness pill at ≥1024px', () => {
    setDesktop(true);
    renderShell();
    expect(screen.getByText('HELP MY RUN')).toBeInTheDocument();
    expect(screen.getByText('AMBER')).toBeInTheDocument();
    expect(screen.getByText(/Next run/)).toBeInTheDocument();
    expect(screen.getByText('page-content')).toBeInTheDocument();
  });

  it('shows the DEMO badge in demo mode on mobile and desktop, never otherwise', () => {
    mocks.authState.data = { setup_required: false, authed: true, demo: true };
    setDesktop(false);
    const { unmount } = renderShell();
    expect(screen.getByText(/DEMO/)).toBeInTheDocument();
    unmount();

    setDesktop(true);
    const { unmount: unmount2 } = renderShell();
    expect(screen.getByText(/DEMO/)).toBeInTheDocument();
    unmount2();

    mocks.authState.data = { setup_required: false, authed: true, demo: false };
    setDesktop(true);
    renderShell();
    expect(screen.queryByText(/DEMO/)).not.toBeInTheDocument();
  });
});
