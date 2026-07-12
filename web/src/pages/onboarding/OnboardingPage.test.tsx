import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import { OnboardingPage } from './OnboardingPage';
import { wizardProfilePatch, INITIAL_WIZARD } from './steps';

const mocks = vi.hoisted(() => ({
  setup: vi.fn(async () => ({ api_token: 'hmr_first_token' })),
  apiPost: vi.fn(),
  updateMutate: vi.fn(),
  authed: false,
  syncError: null as Error | null,
}));

vi.mock('../../api/auth', () => ({ setup: mocks.setup }));

vi.mock('../../api/client', async (orig) => ({
  ...(await orig<typeof import('../../api/client')>()),
  apiPost: mocks.apiPost,
}));

vi.mock('../../lib/push', () => ({
  pushSupported: () => false,
  subscribePush: vi.fn(),
}));

vi.mock('../../api/hooks', () => ({
  useAuthState: () => ({ data: { setup_required: !mocks.authed, authed: mocks.authed } }),
  useFitness: () => ({ data: { easy_pace: '5:31' } }),
  useProfile: () => ({ data: null }),
  useStatus: () => ({
    data: {
      garmin: { connected: false, last_synced_at: null, last_run_at: null, status: 'never', error: null },
      counts: { activities: 0, recovery_days: 0 },
      agent_next_run: '2026-07-10T06:00:00Z',
      manual_sync: false,
      agent_enabled: true,
    },
  }),
  useSync: () => ({
    mutate: vi.fn(),
    isError: mocks.syncError !== null,
    error: mocks.syncError,
    isPending: false,
  }),
  useUpdateProfile: () => ({ mutate: mocks.updateMutate, isPending: false }),
}));

function renderWizard(initialEntry = '/onboarding') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0 }, mutations: { retry: 0 } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <OnboardingPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('wizardProfilePatch', () => {
  it('serializes the wizard answers into profile JSON fields', () => {
    const patch = wizardProfilePatch({
      ...INITIAL_WIZARD,
      markers: { zone2: '148', lthr: '168', maxhr: '' },
      runsPerWeek: 5,
      restDay: 'sunday',
      rules: { ...INITIAL_WIZARD.rules, hrv_backoff: false },
    });
    expect(JSON.parse(patch.goals_json)).toEqual(['crossfit', 'fitness']);
    expect(JSON.parse(patch.week_json)).toEqual({
      runs_per_week: 5,
      crossfit_days: 3,
      rest_day: 'sunday',
    });
    expect(JSON.parse(patch.guardrails_json).hrv_backoff).toBe(false);
    expect(patch.zone2_ceiling_bpm).toBe(148);
    expect(patch.threshold_bpm).toBe(168);
    expect(patch.max_hr_bpm).toBeNull();
  });
});

describe('OnboardingPage', () => {
  it('walks WELCOME → SECURE and blocks until the password is set', async () => {
    renderWizard();
    expect(screen.getByText('Your coach reads Garmin while you sleep.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Get started' }));

    expect(screen.getByText('Secure this instance')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled();

    await userEvent.type(screen.getByLabelText('Password'), 'a strong password');
    await userEvent.type(screen.getByLabelText('Confirm password'), 'a strong password');
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));

    await waitFor(() => expect(mocks.setup).toHaveBeenCalledWith('a strong password'));
    expect(await screen.findByText('hmr_first_token')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled();
  });

  it('shows mismatch error without calling setup', async () => {
    renderWizard();
    await userEvent.click(screen.getByRole('button', { name: 'Get started' }));
    await userEvent.type(screen.getByLabelText('Password'), 'a strong password');
    await userEvent.type(screen.getByLabelText('Confirm password'), 'different password');
    mocks.setup.mockClear();
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
    expect(await screen.findByText('Passwords don’t match.')).toBeInTheDocument();
    expect(mocks.setup).not.toHaveBeenCalled();
  });

  it('Garmin step handles the MFA branch', async () => {
    mocks.apiPost
      .mockResolvedValueOnce({ status: 'mfa_required', login_id: 'lg1' }) // login
      .mockResolvedValueOnce({ status: 'ok' }); // mfa
    renderWizard();
    await userEvent.click(screen.getByRole('button', { name: 'Get started' }));
    // Secure quickly
    await userEvent.type(screen.getByLabelText('Password'), 'a strong password');
    await userEvent.type(screen.getByLabelText('Confirm password'), 'a strong password');
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
    await screen.findByText('hmr_first_token');
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }));

    expect(screen.getByText('Connect Garmin')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled();

    await userEvent.type(screen.getByLabelText('Garmin email'), 'you@example.com');
    await userEvent.type(screen.getByLabelText('Garmin password'), 'garmin-pw');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in to Garmin Connect' }));

    expect(await screen.findByText('Enter the code Garmin sent you.')).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('MFA code'), '424242');
    await userEvent.click(screen.getByRole('button', { name: 'Verify code' }));

    expect(await screen.findByText('Connected to Garmin')).toBeInTheDocument();
    expect(mocks.apiPost).toHaveBeenCalledWith('/api/garmin/login', {
      email: 'you@example.com',
      password: 'garmin-pw',
    });
    expect(mocks.apiPost).toHaveBeenCalledWith('/api/garmin/login/mfa', {
      login_id: 'lg1',
      code: '424242',
    });
    expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled();
  });

  it('surfaces a failed first sync instead of pulsing forever', async () => {
    mocks.syncError = new Error(
      'worker exit 1: fetch failed: API call client error (400): API Error 400 - requested date range is too big.',
    );
    mocks.apiPost.mockResolvedValueOnce({ status: 'ok' }); // garmin login without MFA
    renderWizard();
    await userEvent.click(screen.getByRole('button', { name: 'Get started' }));
    await userEvent.type(screen.getByLabelText('Password'), 'a strong password');
    await userEvent.type(screen.getByLabelText('Confirm password'), 'a strong password');
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
    await screen.findByText('hmr_first_token');
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }));
    await userEvent.type(screen.getByLabelText('Garmin email'), 'e@x.com');
    await userEvent.type(screen.getByLabelText('Garmin password'), 'pw');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in to Garmin Connect' }));
    await screen.findByText('Connected to Garmin');

    expect(screen.getByText(/First sync failed:.*date range is too big/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry sync' })).toBeInTheDocument();
    expect(screen.queryByText(/CONTINUE WHENEVER/)).not.toBeInTheDocument();
    // Login itself succeeded, so the wizard still allows Continue.
    expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled();
    mocks.syncError = null;
  });

  it('finishes with one profile PUT carrying the wizard payload', async () => {
    mocks.apiPost.mockResolvedValueOnce({ status: 'ok' }); // garmin login without MFA
    renderWizard();
    await userEvent.click(screen.getByRole('button', { name: 'Get started' }));
    await userEvent.type(screen.getByLabelText('Password'), 'a strong password');
    await userEvent.type(screen.getByLabelText('Confirm password'), 'a strong password');
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
    await screen.findByText('hmr_first_token');
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }));

    await userEvent.type(screen.getByLabelText('Garmin email'), 'e@x.com');
    await userEvent.type(screen.getByLabelText('Garmin password'), 'pw');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in to Garmin Connect' }));
    await screen.findByText('Connected to Garmin');
    await userEvent.click(screen.getByRole('button', { name: 'Continue' })); // -> GOAL

    expect(screen.getByText('What is running for?')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Continue' })); // -> MARKERS
    expect(screen.getByText('Your numbers')).toBeInTheDocument();
    expect(screen.getByText('5:31')).toBeInTheDocument(); // detected easy pace
    await userEvent.click(screen.getByRole('button', { name: 'Continue' })); // -> RHYTHM
    await userEvent.click(screen.getByRole('button', { name: /increase Runs/ })); // 4 -> 5
    await userEvent.click(screen.getByRole('button', { name: 'Continue' })); // -> GUARDRAILS
    expect(screen.getByText('Coach guardrails')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Continue' })); // -> READY
    expect(screen.getByText('You’re set.')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Open today' }));
    expect(mocks.updateMutate).toHaveBeenCalledOnce();
    const payload = mocks.updateMutate.mock.calls[0][0];
    expect(JSON.parse(payload.week_json)).toEqual({
      runs_per_week: 5,
      crossfit_days: 3,
      rest_day: 'monday',
    });
    expect(JSON.parse(payload.goals_json)).toEqual(['crossfit', 'fitness']);
    expect(payload.agent_enabled).toBe(true);
  });
});
