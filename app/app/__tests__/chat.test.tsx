import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { ChatHistory, ChatMessage } from '../../src/api/types';

const history: ChatHistory = {
  messages: [
    { role: 'user', content: 'How is my Zone 2 pace trending?', created_at: '2026-06-22T09:13:00Z' },
    { role: 'assistant', content: 'Your pace at Z2 dropped ~8 s/km over 12 weeks.', created_at: '2026-06-22T09:14:00Z' },
  ],
};

const mockSend = jest.fn();
const mockClear = jest.fn();

// Mutable mock state so loading / error / empty cases swap return values
// without jest.resetModules() (which forks React under jest-expo and breaks
// the reconciler). Default = happy path.
const mockHookState: {
  chat: { data: ChatHistory | undefined; isPending: boolean; isError: boolean };
  send: { mutate: jest.Mock; data: ChatMessage | undefined; isPending: boolean; isError: boolean; error: unknown };
  clear: { mutate: jest.Mock; isPending: boolean };
} = {
  chat: { data: history, isPending: false, isError: false },
  send: { mutate: mockSend, data: undefined, isPending: false, isError: false, error: null },
  clear: { mutate: mockClear, isPending: false },
};

jest.mock('expo-router', () => {
  const { Text: RNText } = require('react-native');
  return {
    Link: ({ children }: { children: React.ReactNode }) => <RNText>{children}</RNText>,
    Stack: { Screen: () => null },
    useLocalSearchParams: () => ({}),
  };
});

jest.mock('../../src/api/hooks', () => ({
  useChatHistory: () => mockHookState.chat,
  useSendChat: () => mockHookState.send,
  useClearChat: () => mockHookState.clear,
}));

import ChatScreen from '../chat';

afterEach(() => {
  jest.clearAllMocks();
  mockHookState.chat = { data: history, isPending: false, isError: false };
  mockHookState.send = { mutate: mockSend, data: undefined, isPending: false, isError: false, error: null };
  mockHookState.clear = { mutate: mockClear, isPending: false };
});

describe('ChatScreen', () => {
  it('loads history on open and renders user + assistant bubbles', async () => {
    const { getByText, getAllByTestId } = await render(<ChatScreen />);
    expect(getByText('How is my Zone 2 pace trending?')).toBeTruthy();
    expect(getByText('Your pace at Z2 dropped ~8 s/km over 12 weeks.')).toBeTruthy();
    expect(getAllByTestId('bubble-user').length).toBe(1);
    expect(getAllByTestId('bubble-assistant').length).toBe(1);
  });

  it('sends the typed message when Send is pressed', async () => {
    const { getByTestId } = await render(<ChatScreen />);
    await act(async () => {
      fireEvent.changeText(getByTestId('chat-input'), 'What about my HRV?');
    });
    await act(async () => {
      fireEvent.press(getByTestId('btn-send-chat'));
    });
    expect(mockSend).toHaveBeenCalledTimes(1);
    expect(mockSend).toHaveBeenCalledWith({ message: 'What about my HRV?' });
  });

  it('does NOT send a blank/whitespace-only message', async () => {
    const { getByTestId } = await render(<ChatScreen />);
    await act(async () => {
      fireEvent.changeText(getByTestId('chat-input'), '   ');
    });
    await act(async () => {
      fireEvent.press(getByTestId('btn-send-chat'));
    });
    expect(mockSend).not.toHaveBeenCalled();
  });

  it('shows a loading indicator while the answer is awaited', async () => {
    mockHookState.send = { mutate: mockSend, data: undefined, isPending: true, isError: false, error: null };
    const { getByTestId } = await render(<ChatScreen />);
    expect(getByTestId('chat-loading')).toBeTruthy();
  });

  it('shows an error state on send failure (no fabricated bubble)', async () => {
    mockHookState.send = {
      mutate: mockSend, data: undefined, isPending: false, isError: true,
      error: new Error('POST /api/chat failed: 502'),
    };
    const { getByTestId, getAllByTestId } = await render(<ChatScreen />);
    expect(getByTestId('chat-error')).toBeTruthy();
    // history bubbles only; no extra assistant bubble fabricated from the error.
    expect(getAllByTestId('bubble-assistant').length).toBe(1);
  });

  it('clears the chat when the clear action is pressed', async () => {
    const { getByTestId } = await render(<ChatScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-clear-chat'));
    });
    expect(mockClear).toHaveBeenCalledTimes(1);
  });

  it('renders an empty hint when there is no history', async () => {
    mockHookState.chat = { data: { messages: [] }, isPending: false, isError: false };
    const { getByTestId, queryByTestId } = await render(<ChatScreen />);
    expect(getByTestId('chat-empty')).toBeTruthy();
    expect(queryByTestId('bubble-user')).toBeNull();
  });
});
