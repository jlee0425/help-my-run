import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '../api/client';
import type { ChatMessage } from '../api/types';
import { CoachPage } from './CoachPage';

const mocks = vi.hoisted(() => ({
  history: [] as ChatMessage[],
  sendMutate: vi.fn(),
  sendPending: false,
  sendError: null as Error | null,
  clearMutate: vi.fn(),
}));

vi.mock('../api/hooks', () => ({
  useChatHistory: () => ({ data: mocks.history, isLoading: false }),
  useSendChat: () => ({
    mutate: mocks.sendMutate,
    isPending: mocks.sendPending,
    error: mocks.sendError,
    variables: 'Why easy today?',
  }),
  useClearChat: () => ({ mutate: mocks.clearMutate }),
}));

function renderPage() {
  return render(
    <MemoryRouter>
      <CoachPage />
    </MemoryRouter>,
  );
}

describe('CoachPage', () => {
  it('renders both roles from history', () => {
    mocks.history = [
      { role: 'assistant', content: 'Softened today to easy.', created_at: 'x' },
      { role: 'user', content: 'Why easy today?', created_at: 'y' },
    ];
    mocks.sendError = null;
    renderPage();
    expect(screen.getByText('Softened today to easy.')).toBeInTheDocument();
    // appears as a suggestion chip AND as the user's history bubble
    expect(screen.getAllByText('Why easy today?').length).toBeGreaterThanOrEqual(2);
  });

  it('sends typed input and clears the draft', async () => {
    mocks.history = [];
    mocks.sendError = null;
    renderPage();
    const input = screen.getByLabelText('Ask about your training');
    await userEvent.type(input, 'Am I overtraining?');
    await userEvent.click(screen.getByLabelText('Send'));
    expect(mocks.sendMutate).toHaveBeenCalledWith('Am I overtraining?');
    expect((input as HTMLInputElement).value).toBe('');
  });

  it('chips send their literal question', async () => {
    mocks.history = [];
    mocks.sendError = null;
    renderPage();
    await userEvent.click(screen.getByRole('button', { name: 'Running × CrossFit?' }));
    expect(mocks.sendMutate).toHaveBeenCalledWith('Running × CrossFit?');
  });

  it('surfaces 502 engine errors with the server message + retry', async () => {
    mocks.history = [];
    mocks.sendError = new ApiError(502, 'Claude not logged in — run `claude auth login`.');
    renderPage();
    expect(screen.getByText(/claude auth login/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'retry' }));
    expect(mocks.sendMutate).toHaveBeenCalledWith('Why easy today?');
  });

  it('clear asks for confirmation then clears', async () => {
    mocks.history = [{ role: 'assistant', content: 'hello', created_at: 'x' }];
    mocks.sendError = null;
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderPage();
    await userEvent.click(screen.getByRole('button', { name: 'CLEAR' }));
    expect(mocks.clearMutate).toHaveBeenCalledOnce();
  });
});
