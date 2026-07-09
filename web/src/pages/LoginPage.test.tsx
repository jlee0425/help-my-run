import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { LoginPage } from './LoginPage';

const loginFn = vi.hoisted(() => vi.fn());
vi.mock('../api/auth', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/auth')>()),
  login: loginFn,
}));

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0 }, mutations: { retry: 0 } } });
  return render(
    <QueryClientProvider client={qc}>
      <LoginPage />
    </QueryClientProvider>,
  );
}

describe('LoginPage', () => {
  it('submits the password', async () => {
    loginFn.mockResolvedValueOnce(undefined);
    renderPage();
    await userEvent.type(screen.getByLabelText(/PASSWORD/i), 'hunter22again');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));
    await waitFor(() => expect(loginFn).toHaveBeenCalledWith('hunter22again'));
  });

  it('shows an error line on wrong password', async () => {
    const { ApiError } = await import('../api/client');
    loginFn.mockRejectedValueOnce(new ApiError(401, 'unauthorized'));
    renderPage();
    await userEvent.type(screen.getByLabelText(/PASSWORD/i), 'nope-nope');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));
    expect(await screen.findByText('Wrong password.')).toBeInTheDocument();
  });

  it('shows throttle message on 429', async () => {
    const { ApiError } = await import('../api/client');
    loginFn.mockRejectedValueOnce(new ApiError(429, 'throttled'));
    renderPage();
    await userEvent.type(screen.getByLabelText(/PASSWORD/i), 'whatever1');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));
    expect(await screen.findByText(/Too many attempts/)).toBeInTheDocument();
  });
});
