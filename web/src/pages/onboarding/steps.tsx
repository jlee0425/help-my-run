// Step content definitions for the onboarding wizard (design: Onboarding.dc.html,
// extended to 8 steps by the SECURE step — spec §5.4/§6).
import type { CSSProperties } from 'react';

export const STEP_LABELS = [
  'WELCOME',
  'SECURE',
  'CONNECT',
  'GOAL',
  'MARKERS',
  'RHYTHM',
  'GUARDRAILS',
  'READY',
] as const;

export type StepIndex = 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7;

export const GOAL_DEFS = [
  { key: 'crossfit', label: 'Fuel my CrossFit', desc: 'Bigger engine, faster recovery between efforts' },
  { key: 'fitness', label: 'General fitness & health', desc: 'Lower resting HR, better sleep, longevity' },
  { key: 'race', label: 'Train for a race', desc: 'A goal event on the calendar' },
] as const;

export const RULE_DEFS = [
  { key: 'no_b2b_hard', label: 'Never two hard days back-to-back', desc: 'Unless the plan explicitly calls for it' },
  { key: 'protect_long_run', label: 'Protect the long run', desc: 'Move it before cutting it; never cut two in a row' },
  { key: 'easy_stays_easy', label: 'Easy stays easy', desc: 'Hold me under my Zone 2 ceiling on easy days' },
  { key: 'hrv_backoff', label: 'Back off when HRV drops', desc: '3+ days trending down auto-softens the week' },
  { key: 'load_cap_55', label: 'Keep running ≤55% of load', desc: 'So legs stay fresh for lifting' },
] as const;

export const REST_DAYS = [
  'monday',
  'tuesday',
  'wednesday',
  'thursday',
  'friday',
  'saturday',
  'sunday',
] as const;

export type WizardState = {
  goals: Record<string, boolean>;
  markers: { zone2: string; lthr: string; maxhr: string };
  runsPerWeek: number;
  crossfitDays: number;
  restDay: string;
  rules: Record<string, boolean>;
  apiToken: string | null; // shown once after SECURE
};

export const INITIAL_WIZARD: WizardState = {
  goals: { crossfit: true, fitness: true, race: false },
  markers: { zone2: '', lthr: '', maxhr: '' },
  runsPerWeek: 4,
  crossfitDays: 3,
  restDay: 'monday',
  rules: {
    no_b2b_hard: true,
    protect_long_run: true,
    easy_stays_easy: true,
    hrv_backoff: true,
    load_cap_55: true,
  },
  apiToken: null,
};

/** Serialize wizard answers into the profile's three M5 JSON fields. */
export function wizardProfilePatch(w: WizardState): {
  goals_json: string;
  week_json: string;
  guardrails_json: string;
  zone2_ceiling_bpm: number | null;
  threshold_bpm: number | null;
  max_hr_bpm: number | null;
} {
  const num = (s: string) => {
    const n = parseInt(s, 10);
    return Number.isFinite(n) && n > 0 ? n : null;
  };
  return {
    goals_json: JSON.stringify(GOAL_DEFS.filter((g) => w.goals[g.key]).map((g) => g.key)),
    week_json: JSON.stringify({
      runs_per_week: w.runsPerWeek,
      crossfit_days: w.crossfitDays,
      rest_day: w.restDay,
    }),
    guardrails_json: JSON.stringify(w.rules),
    zone2_ceiling_bpm: num(w.markers.zone2),
    threshold_bpm: num(w.markers.lthr),
    max_hr_bpm: num(w.markers.maxhr),
  };
}

export const inputStyle: CSSProperties = {
  width: '100%',
  background: 'var(--inset)',
  border: '1px solid var(--inset-border)',
  borderRadius: 12,
  padding: '13px 14px',
  fontSize: 15,
  color: 'var(--text)',
  outline: 'none',
};

export const stepTitleStyle: CSSProperties = { fontSize: 25, fontWeight: 600, lineHeight: 1.2 };
export const stepBodyStyle: CSSProperties = {
  fontSize: 14,
  color: 'var(--muted)',
  lineHeight: 1.55,
  marginTop: 8,
};
