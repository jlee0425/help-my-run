import React from 'react';
import { View, Text, FlatList, ScrollView, StyleSheet } from 'react-native';
import { Link } from 'expo-router';
import { useStatus, useActivities, useRecovery } from '../src/api/hooks';
import { currentMonday } from '../src/lib/week';
import type { Activity, RecoveryDay } from '../src/api/types';

function fmtKm(distanceM: number): string {
  return (distanceM / 1000).toFixed(2) + ' km';
}

function fmtSyncTime(iso: string | null): string {
  return iso ?? 'never';
}

export default function HomeScreen() {
  const status = useStatus();
  const activities = useActivities();
  const recovery = useRecovery();

  const strava = status.data?.strava;
  const garmin = status.data?.garmin;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Connection</Text>
      <Text testID="home-strava-status" style={styles.statusLine}>
        Strava: {strava?.connected ? 'Connected' : 'Not connected'} · last sync{' '}
        {fmtSyncTime(strava?.last_synced_at ?? null)}
      </Text>
      <Text testID="home-garmin-status" style={styles.statusLine}>
        Garmin: {garmin?.connected ? 'Connected' : 'Not connected'} · last sync{' '}
        {fmtSyncTime(garmin?.last_synced_at ?? null)}
      </Text>
      <Text testID="count-activities" style={styles.statusLine}>
        Activities: {status.data?.counts.activities ?? 0}
      </Text>
      <Text testID="count-recovery" style={styles.statusLine}>
        Recovery days: {status.data?.counts.recovery_days ?? 0}
      </Text>
      <Link href="/settings" style={styles.link}>
        Settings
      </Link>
      <Link href="/plan" style={styles.link}>
        Plan my week
      </Link>
      <Link href={`/plan-view?week=${currentMonday()}`} style={styles.link}>
        This week's plan
      </Link>
      <Link href="/profile" style={styles.link}>
        Profile
      </Link>

      <Text style={styles.heading}>Recent runs</Text>
      <FlatList
        scrollEnabled={false}
        data={activities.data?.activities ?? []}
        keyExtractor={(item: Activity) => String(item.strava_id)}
        ListEmptyComponent={<Text style={styles.empty}>No runs yet</Text>}
        renderItem={({ item }: { item: Activity }) => (
          <View style={styles.row}>
            <Text style={styles.rowTitle}>{item.name}</Text>
            <Text style={styles.rowSub}>
              {fmtKm(item.distance_m)} · {Math.round(item.moving_time_s / 60)} min
              {item.avg_hr != null ? ` · ${Math.round(item.avg_hr)} bpm` : ''}
            </Text>
          </View>
        )}
      />

      <Text style={styles.heading}>Recent recovery</Text>
      <FlatList
        scrollEnabled={false}
        data={recovery.data?.recovery ?? []}
        keyExtractor={(item: RecoveryDay) => item.date}
        ListEmptyComponent={<Text style={styles.empty}>No recovery data yet</Text>}
        renderItem={({ item }: { item: RecoveryDay }) => (
          <View style={styles.row}>
            <Text style={styles.rowTitle}>{item.date}</Text>
            <Text style={styles.rowSub}>
              {item.sleep?.score != null ? `Sleep ${item.sleep.score}` : 'Sleep —'} ·{' '}
              {item.hrv?.last_night_avg_ms != null ? `HRV ${item.hrv.last_night_avg_ms}ms` : 'HRV —'}{' '}
              · {item.rhr?.resting_hr != null ? `RHR ${item.rhr.resting_hr}` : 'RHR —'}
            </Text>
          </View>
        )}
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 6 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  statusLine: { fontSize: 15, color: '#222' },
  link: { fontSize: 15, color: '#fc4c02', marginTop: 8 },
  row: { paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: '#ddd' },
  rowTitle: { fontSize: 16, fontWeight: '500' },
  rowSub: { fontSize: 13, color: '#666' },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8 },
});
