import { Stack } from 'expo-router';
import { QueryClientProvider } from '@tanstack/react-query';
import { queryClient } from '../src/api/queryClient';

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <Stack>
        <Stack.Screen name="index" options={{ title: 'help-my-run' }} />
        <Stack.Screen name="settings" options={{ title: 'Settings' }} />
      </Stack>
    </QueryClientProvider>
  );
}
