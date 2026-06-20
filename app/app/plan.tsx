import React, { useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useRouter } from 'expo-router';
import { useParseCrossfit, useGeneratePlan } from '../src/api/hooks';
import type { CrossFitWeek, CrossFitDay, Load } from '../src/api/types';
import {
  pickFromLibrary,
  takePhoto,
  toUploadFile,
} from '../src/lib/imagePicker';
import { currentMonday, isValidWeekStart } from '../src/lib/week';
import type { ImagePickerAsset } from 'expo-image-picker';

const LOADS: Load[] = ['low', 'med', 'high'];

export default function PlanScreen() {
  const router = useRouter();
  const parse = useParseCrossfit();
  const generate = useGeneratePlan();
  const [week, setWeek] = useState<CrossFitWeek | null>(null);
  const [weekStart, setWeekStart] = useState<string>(() => currentMonday());

  const weekStartValid = isValidWeekStart(weekStart);

  const onPicked = async (asset: ImagePickerAsset | null) => {
    if (!asset) return;
    const file = toUploadFile(asset);
    const parsed = await parse.mutateAsync({ file, weekStart });
    setWeek(parsed);
  };

  const editDay = (i: number, patch: Partial<CrossFitDay>) =>
    setWeek((w) =>
      w ? { ...w, days: w.days.map((d, j) => (j === i ? { ...d, ...patch } : d)) } : w,
    );

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.label}>Week start (Monday, YYYY-MM-DD)</Text>
      <TextInput
        testID="input-week-start"
        style={[styles.input, !weekStartValid && styles.inputError]}
        value={weekStart}
        onChangeText={setWeekStart}
        autoCapitalize="none"
        autoCorrect={false}
        placeholder="YYYY-MM-DD"
      />
      {!weekStartValid ? (
        <Text testID="week-start-error" style={styles.errorLine}>
          Enter a valid date as YYYY-MM-DD.
        </Text>
      ) : null}

      <Pressable
        testID="btn-pick-photo"
        style={[styles.button, (parse.isPending || !weekStartValid) && styles.buttonDisabled]}
        disabled={parse.isPending || !weekStartValid}
        onPress={async () => onPicked(await pickFromLibrary())}
      >
        <Text style={styles.buttonText}>
          {parse.isPending ? 'Parsing…' : 'Pick schedule photo'}
        </Text>
      </Pressable>
      <Pressable
        testID="btn-take-photo"
        style={[styles.button, (parse.isPending || !weekStartValid) && styles.buttonDisabled]}
        disabled={parse.isPending || !weekStartValid}
        onPress={async () => onPicked(await takePhoto())}
      >
        <Text style={styles.buttonText}>Take photo</Text>
      </Pressable>

      {week ? <Text style={styles.heading}>Week of {week.week_start}</Text> : null}

      {week?.days.map((day, i) => (
        <View key={day.date} testID={`cf-day-${day.date}`} style={styles.card}>
          <Text style={styles.rowTitle}>
            {day.dow} · {day.date}
          </Text>
          <Text style={styles.label}>Focus</Text>
          <TextInput
            testID={`cf-focus-${day.date}`}
            style={styles.input}
            value={day.focus}
            onChangeText={(t) => editDay(i, { focus: t })}
          />
          <Text style={styles.label}>Notes</Text>
          <TextInput
            testID={`cf-notes-${day.date}`}
            style={styles.input}
            value={day.notes}
            onChangeText={(t) => editDay(i, { notes: t })}
          />
          <Pressable
            testID={`cf-hascf-${day.date}`}
            style={[styles.chip, day.has_crossfit && styles.chipOn]}
            onPress={() => editDay(i, { has_crossfit: !day.has_crossfit })}
          >
            <Text style={day.has_crossfit ? styles.chipTextOn : styles.chipText}>
              {day.has_crossfit ? 'CrossFit' : 'Rest'}
            </Text>
          </Pressable>
          <Text style={styles.label}>CNS load</Text>
          <View style={styles.chips}>
            {LOADS.map((lv) => (
              <Pressable
                key={lv}
                testID={`cf-cns-${day.date}-${lv}`}
                style={[styles.chip, day.cns_load === lv && styles.chipOn]}
                onPress={() => editDay(i, { cns_load: lv })}
              >
                <Text style={day.cns_load === lv ? styles.chipTextOn : styles.chipText}>{lv}</Text>
              </Pressable>
            ))}
          </View>
          <Text style={styles.label}>Leg load</Text>
          <View style={styles.chips}>
            {LOADS.map((lv) => (
              <Pressable
                key={lv}
                testID={`cf-leg-${day.date}-${lv}`}
                style={[styles.chip, day.leg_load === lv && styles.chipOn]}
                onPress={() => editDay(i, { leg_load: lv })}
              >
                <Text style={day.leg_load === lv ? styles.chipTextOn : styles.chipText}>{lv}</Text>
              </Pressable>
            ))}
          </View>
        </View>
      ))}

      {week ? (
        <Pressable
          testID="btn-generate"
          style={[styles.button, generate.isPending && styles.buttonDisabled]}
          disabled={generate.isPending}
          onPress={() =>
            generate.mutate(
              { week_start: weekStart, crossfit_week: week },
              {
                onSuccess: (plan) =>
                  router.push(`/plan-view?week=${plan.week_start}`),
              },
            )
          }
        >
          <Text style={styles.buttonText}>
            {generate.isPending ? 'Generating…' : 'Generate plan'}
          </Text>
        </Pressable>
      ) : null}

      {generate.data ? (
        <Text testID="plan-generated" style={styles.statusLine}>
          Plan generated for {generate.data.week_start}
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 8 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  label: { fontSize: 14, color: '#444', marginTop: 8 },
  rowTitle: { fontSize: 16, fontWeight: '500' },
  statusLine: { fontSize: 15, color: '#222' },
  input: {
    borderWidth: 1,
    borderColor: '#ccc',
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 16,
  },
  inputError: { borderColor: '#c0392b' },
  errorLine: { color: '#c0392b', fontSize: 14 },
  button: {
    backgroundColor: '#fc4c02',
    borderRadius: 8,
    paddingVertical: 12,
    alignItems: 'center',
    marginTop: 8,
  },
  buttonDisabled: { opacity: 0.5 },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  card: {
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 8,
    padding: 12,
    marginTop: 8,
    gap: 4,
  },
  chips: { flexDirection: 'row', gap: 8, marginTop: 4 },
  chip: {
    borderWidth: 1,
    borderColor: '#ccc',
    borderRadius: 16,
    paddingHorizontal: 14,
    paddingVertical: 6,
    marginTop: 4,
    alignSelf: 'flex-start',
  },
  chipOn: { backgroundColor: '#fc4c02', borderColor: '#fc4c02' },
  chipText: { color: '#444', fontSize: 14 },
  chipTextOn: { color: '#fff', fontSize: 14, fontWeight: '600' },
});
