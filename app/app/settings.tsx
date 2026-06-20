import React, { useEffect, useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet, Switch } from 'react-native';
import { useSettings } from '../src/api/settings';
import { useStatus, useSync, useConnectStrava, useProfile, useUpdateProfile } from '../src/api/hooks';
import type { AthleteProfile } from '../src/api/types';

export default function SettingsScreen() {
  const settings = useSettings();
  const status = useStatus();
  const sync = useSync();
  const connectStrava = useConnectStrava();
  const profile = useProfile();
  const updateProfile = useUpdateProfile();

  const [baseUrl, setBaseUrl] = useState('');
  const [token, setToken] = useState('');
  const [dailyRunTime, setDailyRunTime] = useState('');
  const [timezone, setTimezone] = useState('');
  const [agentEnabled, setAgentEnabled] = useState(true);

  useEffect(() => {
    if (!settings.loading) {
      setBaseUrl(settings.baseUrl);
      setToken(settings.token);
    }
  }, [settings.loading, settings.baseUrl, settings.token]);

  const loadedProfile = profile.data;
  useEffect(() => {
    if (loadedProfile) {
      setDailyRunTime(loadedProfile.daily_run_time);
      setTimezone(loadedProfile.timezone);
      setAgentEnabled(loadedProfile.agent_enabled);
    }
  }, [loadedProfile]);

  const onSaveAgent = () => {
    if (!loadedProfile) return;
    const body: AthleteProfile = {
      ...loadedProfile,
      daily_run_time: dailyRunTime,
      timezone,
      agent_enabled: agentEnabled,
    };
    updateProfile.mutate(body);
  };

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

      <Text style={styles.heading}>Daily coach</Text>
      <Text style={styles.label}>Daily run time (HH:MM, 24h local)</Text>
      <TextInput testID="input-daily-run-time" style={styles.input} autoCapitalize="none" autoCorrect={false}
        placeholder="05:30" value={dailyRunTime} onChangeText={setDailyRunTime} />
      <Text style={styles.label}>Timezone (IANA)</Text>
      <TextInput testID="input-timezone" style={styles.input} autoCapitalize="none" autoCorrect={false}
        placeholder="Asia/Seoul" value={timezone} onChangeText={setTimezone} />
      <View style={styles.toggleRow}>
        <Text style={styles.label}>Agent enabled</Text>
        <Switch testID="toggle-agent-enabled" value={agentEnabled} onValueChange={setAgentEnabled} />
      </View>
      <Pressable testID="btn-save-agent" style={styles.button} disabled={updateProfile.isPending} onPress={onSaveAgent}>
        <Text style={styles.buttonText}>{updateProfile.isPending ? 'Saving…' : 'Save daily coach'}</Text>
      </Pressable>
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
  toggleRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: 8 },
});
