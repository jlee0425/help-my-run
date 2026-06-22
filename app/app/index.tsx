import React from 'react';
import { View, Text, FlatList, ScrollView, StyleSheet, Pressable } from 'react-native';
import { Link } from 'expo-router';
import { useStatus, useActivities, useRecovery, useToday, useUndoToday } from '../src/api/hooks';
import { currentMonday } from '../src/lib/week';
import type { Activity, RecoveryDay, PlanDay } from '../src/api/types';

function fmtKm(distanceM: number): string {
  return (distanceM / 1000).toFixed(2) + ' km';
}

function fmtSyncTime(iso: string | null): string {
  return iso ?? 'never';
}

const READINESS_BG: Record<string, string> = { green: '#1b8a3a', amber: '#d39000', red: '#c0392b' };

function fmtSession(s: PlanDay | null): string {
  if (!s) return 'rest';
  return `${s.run_type} ${s.distance_km}km @ ${s.pace_target}`;
}

function TodayCard() {
  const today = useToday();
  const undo = useUndoToday();
  const b = today.data;
  if (today.isPending) {
    return (
      <View style={styles.todayCard}><Text style={styles.todayLoading}>Loading today…</Text></View>
    );
  }
  if (!b) {
    return (
      <View style={styles.todayCard}><Text style={styles.empty}>No briefing for today yet</Text></View>
    );
  }
  const changed = b.action !== 'STAND' && b.action !== 'REST_DAY' && b.original_session != null && b.effective_session != null;
  return (
    <View style={styles.todayCard}>
      <View style={styles.todayHeaderRow}>
        <Text testID="today-readiness" style={[styles.todayPill, { backgroundColor: READINESS_BG[b.readiness_color] ?? '#666' }]}>
          {b.readiness_color}
        </Text>
        <Text style={styles.todayAction}>{b.action}</Text>
        {b.stale ? <Text testID="today-stale" style={styles.todayStale}>stale data</Text> : null}
      </View>
      {b.reasons.length > 0 ? (
        <View style={styles.todayReasons}>
          {b.reasons.map((r) => (
            <View key={r} style={styles.todayReasonRow}>
              <Text style={styles.todayReasonBullet}>•</Text>
              <Text style={styles.todayReason}>{r}</Text>
            </View>
          ))}
        </View>
      ) : null}
      <Text testID="today-original" style={styles.todayLine}>Original: {fmtSession(b.original_session)}</Text>
      <Text testID="today-effective" style={styles.todayLineStrong}>Today: {fmtSession(b.effective_session)}</Text>
      <Text testID="today-rationale" style={styles.todayRationale}>{b.rationale}</Text>
      {changed ? (
        <Pressable testID="btn-today-undo" style={styles.undoButton} disabled={undo.isPending} onPress={() => undo.mutate()}>
          <Text style={styles.undoButtonText}>{undo.isPending ? 'Reverting…' : 'Undo (revert to original)'}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

export default function HomeScreen() {
  const status = useStatus();
  const activities = useActivities();
  const recovery = useRecovery();

  const strava = status.data?.strava;
  const garmin = status.data?.garmin;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Today</Text>
      <TodayCard />

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
      <Link href="/progress" style={styles.link}>
        Progress
      </Link>

      <Text style={styles.heading}>Recent runs</Text>
      <FlatList
        scrollEnabled={false}
        data={activities.data?.activities ?? []}
        keyExtractor={(item: Activity) => String(item.strava_id)}
        ListEmptyComponent={<Text style={styles.empty}>No runs yet</Text>}
        renderItem={({ item }: { item: Activity }) => (
          <Link href={{ pathname: '/run/[id]', params: { id: String(item.strava_id) } }} asChild>
            <Pressable testID={`run-row-${item.strava_id}`} style={styles.row}>
              <Text style={styles.rowTitle}>{item.name}</Text>
              <Text style={styles.rowSub}>
                {fmtKm(item.distance_m)} · {Math.round(item.moving_time_s / 60)} min
                {item.avg_hr != null ? ` · ${Math.round(item.avg_hr)} bpm` : ''}
              </Text>
            </Pressable>
          </Link>
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
  todayCard: { borderWidth: StyleSheet.hairlineWidth, borderColor: '#ddd', borderRadius: 10, padding: 12, gap: 6, backgroundColor: '#fafafa' },
  todayLoading: { fontSize: 14, color: '#666' },
  todayHeaderRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  todayPill: { color: '#fff', fontWeight: '700', textTransform: 'uppercase', fontSize: 12, paddingHorizontal: 10, paddingVertical: 4, borderRadius: 12, overflow: 'hidden' },
  todayAction: { fontSize: 15, fontWeight: '600', color: '#222' },
  todayStale: { fontSize: 12, color: '#c0392b', fontStyle: 'italic' },
  todayReasons: { gap: 2 },
  todayReasonRow: { flexDirection: 'row', gap: 6 },
  todayReasonBullet: { fontSize: 13, color: '#555' },
  todayReason: { fontSize: 13, color: '#555', flexShrink: 1 },
  todayLine: { fontSize: 14, color: '#666' },
  todayLineStrong: { fontSize: 15, fontWeight: '600', color: '#222' },
  todayRationale: { fontSize: 13, color: '#444', fontStyle: 'italic' },
  undoButton: { marginTop: 6, alignSelf: 'flex-start', borderWidth: StyleSheet.hairlineWidth, borderColor: '#fc4c02', borderRadius: 8, paddingHorizontal: 12, paddingVertical: 8 },
  undoButtonText: { fontSize: 14, color: '#fc4c02', fontWeight: '600' },
});
