import React from 'react';
import { View, Text, ScrollView, Pressable, StyleSheet } from 'react-native';
import { useLocalSearchParams } from 'expo-router';
import { useProgress, useAnalyzeProgress } from '../src/api/hooks';
import { sparkline } from '../src/lib/sparkline';
import type { TrendSummary } from '../src/api/types';

// Map raw value-direction + lower_is_better to a glyph the user reads as
// improving/worsening. The arrow reflects the VALUE movement (↑ value up,
// ↓ value down, → flat); color reflects improvement.
function arrowGlyph(s: TrendSummary): string {
  if (s.direction === 'flat') return '→';
  return s.direction === 'up' ? '↑' : '↓';
}

// improving = value moved in the better direction for this signal.
function isImproving(s: TrendSummary): boolean {
  if (s.direction === 'flat') return false;
  const valueWentUp = s.direction === 'up';
  return s.lower_is_better ? !valueWentUp : valueWentUp;
}

function fmtNum(v: number | null): string {
  return v == null ? '—' : String(v);
}

function fmtDelta(v: number | null): string {
  if (v == null) return '—';
  return v > 0 ? `+${v}` : String(v);
}

function TrendCard({ signal }: { signal: TrendSummary }) {
  const improving = isImproving(signal);
  return (
    <View testID={`progress-card-${signal.key}`} style={styles.card}>
      <View style={styles.cardHeaderRow}>
        <Text style={styles.cardLabel}>{signal.label}</Text>
        <Text
          testID={`progress-arrow-${signal.key}`}
          style={[styles.arrow, { color: improving ? '#1b8a3a' : '#c0392b' }]}
        >
          {arrowGlyph(signal)}
        </Text>
      </View>
      <Text style={styles.cardCurrent}>
        {fmtNum(signal.current)} {signal.unit}
      </Text>
      <Text style={styles.cardDelta}>
        {fmtDelta(signal.delta_abs)} {signal.unit} vs {fmtNum(signal.baseline)} (start)
      </Text>
      <Text testID={`progress-spark-${signal.key}`} style={styles.spark}>
        {sparkline(signal.series)}
      </Text>
    </View>
  );
}

export default function ProgressScreen() {
  const params = useLocalSearchParams<{ weeks?: string }>();
  const weeks = typeof params.weeks === 'string' ? Number(params.weeks) : 12;
  const progress = useProgress(weeks);
  const analyze = useAnalyzeProgress();

  const report = progress.data;
  const enoughData = report?.enough_data ?? false;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Progress</Text>
      {progress.isPending ? <Text style={styles.loading}>Loading…</Text> : null}

      {report && !enoughData ? (
        <Text testID="progress-empty" style={styles.empty}>
          Not enough data yet — keep logging runs and syncing Garmin.
        </Text>
      ) : null}

      {enoughData
        ? report!.signals.map((s) => <TrendCard key={s.key} signal={s} />)
        : null}

      <Pressable
        testID="btn-analyze-progress"
        style={styles.button}
        disabled={analyze.isPending}
        onPress={() => analyze.mutate({ weeks })}
      >
        <Text style={styles.buttonText}>
          {analyze.isPending ? 'Analyzing…' : 'Analyze progress'}
        </Text>
      </Pressable>

      <Text style={styles.heading}>Coach read</Text>
      <Text testID="progress-read" style={styles.read}>
        {analyze.data?.text ?? '—'}
      </Text>
      {analyze.data?.source ? (
        <Text testID="progress-read-source" style={styles.readSource}>
          source: {analyze.data.source}
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 6 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  loading: { fontSize: 14, color: '#666' },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8 },
  card: {
    borderWidth: StyleSheet.hairlineWidth, borderColor: '#ddd', borderRadius: 10,
    padding: 12, gap: 4, backgroundColor: '#fafafa', marginTop: 8,
  },
  cardHeaderRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  cardLabel: { fontSize: 16, fontWeight: '600', color: '#222' },
  arrow: { fontSize: 18, fontWeight: '700' },
  cardCurrent: { fontSize: 20, fontWeight: '700', color: '#222' },
  cardDelta: { fontSize: 13, color: '#666' },
  spark: { fontFamily: 'monospace', fontSize: 16, letterSpacing: 1, color: '#fc4c02' },
  button: {
    backgroundColor: '#fc4c02', borderRadius: 8, paddingVertical: 12,
    alignItems: 'center', marginTop: 16,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  read: { fontSize: 14, color: '#444', fontStyle: 'italic' },
  readSource: { fontSize: 12, color: '#999' },
});
