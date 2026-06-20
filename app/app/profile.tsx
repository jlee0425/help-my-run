import React, { useEffect, useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useProfile, useUpdateProfile } from '../src/api/hooks';
import type { AthleteProfile } from '../src/api/types';

type Mode = 'build' | 'hold';

function parseIntOrNull(s: string): number | null {
  const t = s.trim();
  if (t === '') return null;
  const n = parseInt(t, 10);
  return Number.isNaN(n) ? null : n;
}

function parseFloatOr(s: string, fallback: number): number {
  const n = parseFloat(s.trim());
  return Number.isNaN(n) ? fallback : n;
}

export default function ProfileScreen() {
  const profile = useProfile();
  const update = useUpdateProfile();

  const [targetKm, setTargetKm] = useState('');
  const [mode, setMode] = useState<Mode>('build');
  const [zone2, setZone2] = useState('');
  const [thresholdBpm, setThresholdBpm] = useState('');
  const [maxHr, setMaxHr] = useState('');
  const [constraints, setConstraints] = useState('');
  const [goal, setGoal] = useState('');

  const loaded = profile.data;
  useEffect(() => {
    if (loaded) {
      setTargetKm(String(loaded.target_weekly_km));
      setMode(loaded.progression_mode);
      setZone2(loaded.zone2_ceiling_bpm != null ? String(loaded.zone2_ceiling_bpm) : '');
      setThresholdBpm(loaded.threshold_bpm != null ? String(loaded.threshold_bpm) : '');
      setMaxHr(loaded.max_hr_bpm != null ? String(loaded.max_hr_bpm) : '');
      setConstraints(loaded.run_constraints_json);
      setGoal(loaded.goal_text);
    }
  }, [loaded]);

  const onSave = () => {
    const body: AthleteProfile = {
      target_weekly_km: parseFloatOr(targetKm, 20),
      progression_mode: mode,
      zone2_ceiling_bpm: parseIntOrNull(zone2),
      threshold_bpm: parseIntOrNull(thresholdBpm),
      max_hr_bpm: parseIntOrNull(maxHr),
      run_constraints_json: constraints,
      goal_text: goal,
    };
    update.mutate(body);
  };

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Target</Text>
      <Text style={styles.label}>Target weekly km</Text>
      <TextInput
        testID="input-target-km"
        style={styles.input}
        keyboardType="numeric"
        value={targetKm}
        onChangeText={setTargetKm}
      />

      <Text style={styles.label}>Progression mode</Text>
      <View style={styles.chips}>
        <Pressable
          testID="mode-build"
          style={[styles.chip, mode === 'build' && styles.chipOn]}
          onPress={() => setMode('build')}
        >
          <Text style={mode === 'build' ? styles.chipTextOn : styles.chipText}>build</Text>
        </Pressable>
        <Pressable
          testID="mode-hold"
          style={[styles.chip, mode === 'hold' && styles.chipOn]}
          onPress={() => setMode('hold')}
        >
          <Text style={mode === 'hold' ? styles.chipTextOn : styles.chipText}>hold</Text>
        </Pressable>
      </View>

      <Text style={styles.heading}>HR markers (optional)</Text>
      <Text style={styles.label}>Zone 2 ceiling bpm</Text>
      <TextInput
        testID="input-zone2"
        style={styles.input}
        keyboardType="numeric"
        value={zone2}
        onChangeText={setZone2}
      />
      <Text style={styles.label}>Threshold bpm</Text>
      <TextInput
        testID="input-threshold-bpm"
        style={styles.input}
        keyboardType="numeric"
        value={thresholdBpm}
        onChangeText={setThresholdBpm}
      />
      <Text style={styles.label}>Max HR bpm</Text>
      <TextInput
        testID="input-maxhr"
        style={styles.input}
        keyboardType="numeric"
        value={maxHr}
        onChangeText={setMaxHr}
      />

      <Text style={styles.heading}>Constraints</Text>
      <Text style={styles.label}>Run constraints (JSON)</Text>
      <TextInput
        testID="input-constraints"
        style={[styles.input, styles.multiline]}
        autoCapitalize="none"
        autoCorrect={false}
        multiline
        value={constraints}
        onChangeText={setConstraints}
      />

      <Text style={styles.heading}>Goal</Text>
      <TextInput
        testID="input-goal"
        style={[styles.input, styles.multiline]}
        multiline
        value={goal}
        onChangeText={setGoal}
      />

      <Pressable
        testID="btn-save-profile"
        style={styles.button}
        disabled={update.isPending}
        onPress={onSave}
      >
        <Text style={styles.buttonText}>{update.isPending ? 'Saving…' : 'Save profile'}</Text>
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
  multiline: { minHeight: 64, textAlignVertical: 'top' },
  chips: { flexDirection: 'row', gap: 8, marginTop: 4 },
  chip: {
    borderWidth: 1,
    borderColor: '#ccc',
    borderRadius: 16,
    paddingHorizontal: 16,
    paddingVertical: 6,
    alignSelf: 'flex-start',
  },
  chipOn: { backgroundColor: '#fc4c02', borderColor: '#fc4c02' },
  chipText: { color: '#444', fontSize: 14 },
  chipTextOn: { color: '#fff', fontSize: 14, fontWeight: '600' },
  button: {
    backgroundColor: '#fc4c02',
    borderRadius: 8,
    paddingVertical: 12,
    alignItems: 'center',
    marginTop: 16,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
});
