import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';

const mocks = vi.hoisted(() => ({
  changePassword: vi.fn(async () => undefined),
  regenerateToken: vi.fn(async () => ({ api_token: 'hmr_new_token_shown_once' })),
  logoutMutate: vi.fn(),
  syncMutate: vi.fn(),
  updateMutate: vi.fn(),
  claudeTokenSet: vi.fn(),
  garminDisconnect: vi.fn(),
}));

vi.mock('../api/auth', () => ({
  changePassword: mocks.changePassword,
  regenerateToken: mocks.regenerateToken,
}));

vi.mock('../lib/push', () => ({
  pushSupported: () => false,
  pushEnabled: async () => false,
  subscribePush: vi.fn(),
  unsubscribePush: vi.fn(),
  sendTestPush: vi.fn(),
}));

vi.mock('../api/hooks', () => ({
  useGarminStatus: () => ({
    data: { connected: true, last_synced_at: '2026-07-09T05:30:00Z' },
    isError: false,
  }),
  useGarminDisconnect: () => ({ mutate: mocks.garminDisconnect, isPending: false }),
  useClaudeStatus: () => ({
    data: {
      binary_found: true,
      authenticated: true,
      model: 'claude-opus-4-8',
      detail: '',
      checked_at: 'x',
    },
    isError: false,
    refetch: vi.fn(),
  }),
  useClaudeTokenSet: () => ({ mutate: mocks.claudeTokenSet, isPending: false, error: null }),
  useClaudeTokenDelete: () => ({ mutate: vi.fn(), isPending: false }),
  useStatus: () => ({
    data: {
      garmin: { connected: true, last_synced_at: '2026-07-09T05:30:00Z', last_run_at: null, status: 'ok', error: null },
      counts: { activities: 46, recovery_days: 89 },
      agent_next_run: '2026-07-10T06:00:00Z',
      agent_enabled: true,
    },
  }),
  useSync: () => ({ mutate: mocks.syncMutate, isPending: false, error: null }),
  useProfile: () => ({
    data: {
      target_weekly_km: 25,
      progression_mode: 'build',
      zone2_ceiling_bpm: 148,
      threshold_bpm: 168,
      max_hr_bpm: 186,
      run_constraints_json: '{}',
      goal_text: 'engine',
      daily_run_time: '06:00',
      timezone: 'UTC',
      agent_enabled: true,
      goals_json: '[]',
      week_json: '{}',
      guardrails_json: '{}',
    },
  }),
  useUpdateProfile: () => ({ mutate: mocks.updateMutate, error: null }),
  useLogout: () => ({ mutate: mocks.logoutMutate, isPending: false }),
}));

function renderPage() {
  return render(
    <MemoryRouter>
      <SettingsPage />
    </MemoryRouter>,
  );
}

describe('SettingsPage', () => {
  it('renders all five cards with live data', () => {
    renderPage();
    expect(screen.getByText('// GARMIN')).toBeInTheDocument();
    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.getByText('// CLAUDE')).toBeInTheDocument();
    expect(screen.getByText(/Subscription active · claude-opus-4-8/)).toBeInTheDocument();
    expect(screen.getByText('// NOTIFICATIONS')).toBeInTheDocument();
    expect(screen.getByText(/doesn’t support Web Push/)).toBeInTheDocument();
    expect(screen.getByText(/46 ACTIVITIES · 89/)).toBeInTheDocument();
    expect(screen.getByText('// SECURITY')).toBeInTheDocument();
  });

  it('changes the password through the auth api', async () => {
    renderPage();
    await userEvent.type(screen.getByLabelText('Current password'), 'old-password');
    await userEvent.type(screen.getByLabelText('New password'), 'new-password-1');
    await userEvent.click(screen.getByRole('button', { name: 'Change password' }));
    await waitFor(() =>
      expect(mocks.changePassword).toHaveBeenCalledWith('old-password', 'new-password-1'),
    );
    expect(await screen.findByText('Password changed.')).toBeInTheDocument();
  });

  it('regenerates the API token and shows it once', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderPage();
    await userEvent.click(screen.getByRole('button', { name: 'Regenerate token' }));
    expect(await screen.findByText('hmr_new_token_shown_once')).toBeInTheDocument();
  });

  it('saves a pasted claude setup-token', async () => {
    renderPage();
    await userEvent.type(screen.getByLabelText('Claude setup token'), 'sk-ant-oat01-abc');
    await userEvent.click(screen.getByRole('button', { name: 'Save token' }));
    expect(mocks.claudeTokenSet).toHaveBeenCalledWith('sk-ant-oat01-abc', expect.anything());
  });

  it('sync now and logout fire their mutations', async () => {
    renderPage();
    await userEvent.click(screen.getByRole('button', { name: 'Sync now' }));
    expect(mocks.syncMutate).toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Log out' }));
    expect(mocks.logoutMutate).toHaveBeenCalled();
  });
});
