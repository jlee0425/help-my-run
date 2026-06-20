import React from 'react';
import { View, Text, FlatList, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useLocalSearchParams } from 'expo-router';
import { usePlan, useGeneratePlan } from '../src/api/hooks';
import type { PlanDay } from '../src/api/types';

export default function PlanViewScreen() {
  const params = useLocalSearchParams<{ week?: string }>();
  const week = typeof params.week === 'string' ? params.week : '';
  const plan = usePlan(week);
  const generate = useGeneratePlan();

  // Cold start: Regenerate needs a previously-parsed CrossFit week for this week.
  // The backend returns 404 ("no crossfit week for that week") in that case, so
  // surface a specific, friendly hint instead of a generic failure.
  const regenError = generate.isError
    ? /404|no crossfit week/i.test(String(generate.error?.message ?? ''))
      ? 'No CrossFit week for this week — parse a photo first.'
      : 'Could not regenerate the plan. Please try again.'
    : '';

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Fitness</Text>
      <Text testID="plan-fitness-summary" style={styles.statusLine}>
        {plan.data?.fitness_summary ?? '—'}
      </Text>
      <Text testID="plan-weekly-target" style={styles.statusLine}>
        Weekly target: {plan.data?.weekly_target_km ?? 0} km
      </Text>

      <Text style={styles.heading}>Plan</Text>
      <FlatList
        scrollEnabled={false}
        data={plan.data?.days ?? []}
        keyExtractor={(d: PlanDay) => d.date}
        ListEmptyComponent={<Text style={styles.empty}>No plan yet</Text>}
        renderItem={({ item }: { item: PlanDay }) => (
          <View testID={`plan-day-${item.date}`} style={styles.row}>
            <Text testID={`plan-day-${item.date}-title`} style={styles.rowTitle}>
              {item.dow} · {item.run_type}
              {item.optional_if_cns ? ' (optional)' : ''}
            </Text>
            <Text testID={`plan-day-${item.date}-detail`} style={styles.rowSub}>
              {item.distance_km} km · {item.pace_target || '—'} · {item.time_note || '—'}
            </Text>
            <Text style={styles.rowSub}>{item.rationale}</Text>
          </View>
        )}
      />

      <Text style={styles.heading}>Why this week</Text>
      <Text testID="plan-week-rationale" style={styles.statusLine}>
        {plan.data?.week_rationale ?? '—'}
      </Text>
      <Text style={styles.heading}>Flag</Text>
      <Text testID="plan-one-flag" style={styles.statusLine}>
        {plan.data?.one_flag ?? '—'}
      </Text>

      <Pressable
        testID="btn-regenerate"
        style={styles.button}
        disabled={generate.isPending || !week}
        onPress={() => generate.mutate({ week_start: week })}
      >
        <Text style={styles.buttonText}>
          {generate.isPending ? 'Regenerating…' : 'Regenerate'}
        </Text>
      </Pressable>
      {regenError ? (
        <Text testID="plan-regenerate-error" style={styles.errorLine}>
          {regenError}
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 6 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  statusLine: { fontSize: 15, color: '#222' },
  row: { paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: '#ddd' },
  rowTitle: { fontSize: 16, fontWeight: '500' },
  rowSub: { fontSize: 13, color: '#666' },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8 },
  button: {
    backgroundColor: '#fc4c02',
    borderRadius: 8,
    paddingVertical: 12,
    alignItems: 'center',
    marginTop: 16,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  errorLine: { color: '#c0392b', fontSize: 14, marginTop: 8 },
});
