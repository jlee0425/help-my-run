import React, { useEffect, useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useSettings } from '../src/api/settings';
import { useStatus, useSync, useConnectStrava } from '../src/api/hooks';

export default function SettingsScreen() {
  const settings = useSettings();
  const status = useStatus();
  const sync = useSync();
  const connectStrava = useConnectStrava();

  const [baseUrl, setBaseUrl] = useState('');
  const [token, setToken] = useState('');

  useEffect(() => {
    if (!settings.loading) {
      setBaseUrl(settings.baseUrl);
      setToken(settings.token);
    }
  }, [settings.loading, settings.baseUrl, settings.token]);

  const stravaConnected = status.data?.strava.connected ?? false;
  const garminConnected = status.data?.garmin.connected ?? false;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Backend</Text>
      <Text style={styles.label}>Backend URL</Text>
      <TextInput
        testID="input-base-url"
        style={styles.input}
        autoCapitalize="none"
        autoCorrect={false}
        placeholder="http://localhost:8080"
        value={baseUrl}
        onChangeText={setBaseUrl}
      />
      <Text style={styles.label}>API token</Text>
      <TextInput
        testID="input-token"
        style={styles.input}
        autoCapitalize="none"
        autoCorrect={false}
        secureTextEntry
        placeholder="API_TOKEN"
        value={token}
        onChangeText={setToken}
      />
      <Pressable
        testID="btn-save"
        style={styles.button}
        onPress={() => settings.save(baseUrl, token)}
      >
        <Text style={styles.buttonText}>Save</Text>
      </Pressable>

      <Text style={styles.heading}>Strava</Text>
      <Text testID="strava-status" style={styles.statusLine}>
        {stravaConnected ? 'Connected' : 'Not connected'}
      </Text>
      <Pressable
        testID="btn-strava-connect"
        style={styles.button}
        disabled={connectStrava.isPending}
        onPress={() => connectStrava.mutate()}
      >
        <Text style={styles.buttonText}>
          {connectStrava.isPending ? 'Connecting…' : 'Strava Connect'}
        </Text>
      </Pressable>

      <Text style={styles.heading}>Garmin</Text>
      <Text testID="garmin-status" style={styles.statusLine}>
        {garminConnected ? 'Connected' : 'Not connected'}
      </Text>

      <Text style={styles.heading}>Sync</Text>
      <Pressable
        testID="btn-sync"
        style={styles.button}
        disabled={sync.isPending}
        onPress={() => sync.mutate()}
      >
        <Text style={styles.buttonText}>{sync.isPending ? 'Syncing…' : 'Sync now'}</Text>
      </Pressable>
      {sync.data ? (
        <Text testID="sync-result" style={styles.statusLine}>
          Strava: {sync.data.strava.status} ({sync.data.strava.synced}) · Garmin:{' '}
          {sync.data.garmin.status} ({sync.data.garmin.synced})
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 8 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  label: { fontSize: 14, color: '#444', marginTop: 8 },
  input: {
    borderWidth: 1,
    borderColor: '#ccc',
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 16,
  },
  button: {
    backgroundColor: '#fc4c02',
    borderRadius: 8,
    paddingVertical: 12,
    alignItems: 'center',
    marginTop: 8,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  statusLine: { fontSize: 15, color: '#222' },
});
