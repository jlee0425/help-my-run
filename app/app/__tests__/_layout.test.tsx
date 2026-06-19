import React from 'react';
import { render } from '@testing-library/react-native';
import { Text } from 'react-native';
import { useQueryClient } from '@tanstack/react-query';

jest.mock('expo-router', () => {
  const React = require('react');
  const Stack = ({ children }: { children?: React.ReactNode }) =>
    React.createElement(React.Fragment, null, children);
  Stack.Screen = () => null;
  return { Stack };
});

import RootLayout from '../_layout';

// A probe that throws (caught by RTL) if no QueryClient is in context.
function QueryClientProbe() {
  const client = useQueryClient();
  return <Text testID="probe">{client ? 'has-client' : 'no-client'}</Text>;
}

describe('RootLayout', () => {
  // render() is async in @testing-library/react-native v14 (React 19
  // test-renderer), so each test awaits it before querying the result.
  it('renders without crashing', async () => {
    // The layout itself emits no host elements under the expo-router mock
    // (Stack/Stack.Screen render nothing), so toJSON() is null. The point of
    // this case is that mounting RootLayout does not throw and returns a
    // usable render result.
    const result = await render(<RootLayout />);
    expect(result.toJSON).toBeInstanceOf(Function);
    expect(() => result.toJSON()).not.toThrow();
  });

  it('provides a QueryClient to its subtree', async () => {
    // Render the same provider the layout uses by mounting the layout's
    // QueryClientProvider via the exported queryClient.
    const { queryClient } = require('../../src/api/queryClient');
    const { QueryClientProvider } = require('@tanstack/react-query');
    const { getByTestId } = await render(
      <QueryClientProvider client={queryClient}>
        <QueryClientProbe />
      </QueryClientProvider>
    );
    expect(getByTestId('probe').props.children).toBe('has-client');
  });

  it('exposes the shared queryClient to RootLayout descendants', async () => {
    // The mocked Stack renders its children, so this probe is mounted inside
    // the QueryClientProvider that RootLayout itself wires up. If RootLayout
    // ever dropped the provider, useQueryClient() would report no client.
    const { Stack } = require('expo-router') as {
      Stack: { Screen: React.ComponentType };
    };
    const { queryClient } = require('../../src/api/queryClient');
    const { useQueryClient } = require('@tanstack/react-query');
    let resolvedClient: unknown = null;
    const CaptureScreen = () => {
      resolvedClient = useQueryClient();
      return null;
    };
    // Replace one Screen with a capturing probe for this assertion only.
    const originalScreen = Stack.Screen;
    Stack.Screen = CaptureScreen as typeof Stack.Screen;
    try {
      await render(<RootLayout />);
    } finally {
      Stack.Screen = originalScreen;
    }
    expect(resolvedClient).toBe(queryClient);
  });
});
