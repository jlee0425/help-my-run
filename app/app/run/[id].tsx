import React from 'react';
import { View, Text, ScrollView, Pressable, StyleSheet } from 'react-native';
import { useLocalSearchParams } from 'expo-router';
import { useActivityAnalysis, useFetchStream } from '../../src/api/hooks';
import type { ZoneTime } from '../../src/api/types';

// Plain-language read for the decoupling number (spec §8).
function decouplingRead(pct: number | null): string {
  if (pct == null) return 'No decoupling — needs HR over a longer run.';
  if (pct < 5) return 'Under 5% on an easy long run = good aerobic durability.';
  if (pct < 10) return 'Mild drift — durability is developing.';
  return 'High drift — HR climbed relative to pace; aerobic base still building.';
}

function ZoneBar({ z }: { z: ZoneTime }) {
  return (
    <View testID={`zone-bar-${z.zone}`} style={styles.zoneRow}>
      <Text style={styles.zoneLabel}>Z{z.zone}</Text>
      <View style={styles.zoneTrack}>
        <View style={[styles.zoneFill, { width: `${z.pct}%` }]} />
      </View>
      <Text style={styles.zoneVal}>
        {Math.round(z.seconds / 60)} min · {z.pct.toFixed(0)}%
      </Text>
    </View>
  );
}

export default function RunDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const activityId = Number(id);
  const analysis = useActivityAnalysis(activityId);
  const fetch = useFetchStream(activityId);

  const a = analysis.data;

  if (analysis.isPending) {
    return (
      <ScrollView contentContainerStyle={styles.container}>
        <Text style={styles.heading}>Run detail</Text>
        <Text testID="run-loading" style={styles.loading}>Loading…</Text>
      </ScrollView>
    );
  }

  const decoupling = a?.decoupling_pct ?? null;
  const decouplingImproving = decoupling != null && decoupling < 5;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Run detail</Text>

      {a && !a.has_stream ? (
        <View style={styles.section}>
          <Text style={styles.empty}>No stream fetched for this run yet.</Text>
          <Pressable
            testID="btn-fetch-stream"
            style={styles.button}
            disabled={fetch.isPending}
            onPress={() => fetch.mutate()}
          >
            <Text style={styles.buttonText}>
              {fetch.isPending ? 'Fetching…' : 'Fetch stream'}
            </Text>
          </Pressable>
        </View>
      ) : null}

      {a && a.has_stream && !a.has_hr ? (
        <Text testID="run-no-hr" style={styles.empty}>
          No HR in this stream — time-in-zone and decoupling need heart rate.
        </Text>
      ) : null}

      {a && a.has_stream && a.has_hr ? (
        <View style={styles.section}>
          {a.source === 'garmin' ? (
            <Text testID="source-badge" style={styles.sourceBadge}>HR via Garmin .FIT</Text>
          ) : null}
          <Text style={styles.subheading}>Time in zone</Text>
          {a.time_in_zone.map((z) => (
            <ZoneBar key={z.zone} z={z} />
          ))}
        </View>
      ) : null}

      <View style={styles.section}>
        <Text style={styles.subheading}>Decoupling (Pa:HR drift)</Text>
        <Text
          testID="decoupling-value"
          style={[
            styles.decouplingValue,
            { color: decoupling == null ? '#999' : decouplingImproving ? '#1b8a3a' : '#c0392b' },
          ]}
        >
          {decoupling == null ? '—' : `${decoupling.toFixed(1)}%`}
        </Text>
        <Text style={styles.decouplingRead}>{decouplingRead(decoupling)}</Text>
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 6 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 8 },
  subheading: { fontSize: 16, fontWeight: '600', color: '#222', marginTop: 12 },
  loading: { fontSize: 14, color: '#666' },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8 },
  section: { gap: 4 },
  sourceBadge: {
    alignSelf: 'flex-start', fontSize: 12, fontWeight: '600', color: '#fc4c02',
    backgroundColor: '#fff0e8', borderRadius: 6, paddingHorizontal: 8, paddingVertical: 3, marginTop: 8,
  },
  zoneRow: { flexDirection: 'row', alignItems: 'center', gap: 8, paddingVertical: 4 },
  zoneLabel: { width: 28, fontSize: 13, color: '#222', fontWeight: '600' },
  zoneTrack: { flex: 1, height: 14, backgroundColor: '#eee', borderRadius: 7, overflow: 'hidden' },
  zoneFill: { height: '100%', backgroundColor: '#fc4c02', borderRadius: 7 },
  zoneVal: { width: 96, fontSize: 12, color: '#666', textAlign: 'right' },
  decouplingValue: { fontSize: 24, fontWeight: '700' },
  decouplingRead: { fontSize: 13, color: '#444', fontStyle: 'italic' },
  button: {
    alignSelf: 'flex-start', backgroundColor: '#fc4c02', borderRadius: 8,
    paddingHorizontal: 16, paddingVertical: 10, marginTop: 4,
  },
  buttonText: { color: '#fff', fontSize: 15, fontWeight: '600' },
});
