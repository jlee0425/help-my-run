// The four profile-shaping steps (Goal, Markers, Rhythm, Guardrails). They are
// pure wizard-state editors that change together, so they share a file.
import { useEffect } from 'react';
import { useFitness, useProfile } from '../../../api/hooks';
import { MonoLabel, Stepper, Toggle } from '../../../ui/kit';
import {
  GOAL_DEFS,
  REST_DAYS,
  RULE_DEFS,
  stepBodyStyle,
  stepTitleStyle,
  type WizardState,
} from '../steps';

type StepProps = { w: WizardState; setW: (fn: (w: WizardState) => WizardState) => void };

export function GoalStep({ w, setW }: StepProps) {
  return (
    <div className="fade-up">
      <div style={stepTitleStyle}>What is running for?</div>
      <div style={stepBodyStyle}>
        This shapes every call. Pick what matters — most people here run to fuel something else.
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 22 }}>
        {GOAL_DEFS.map((g) => {
          const sel = w.goals[g.key];
          return (
            <button
              key={g.key}
              onClick={() => setW((prev) => ({ ...prev, goals: { ...prev.goals, [g.key]: !sel } }))}
              style={{
                textAlign: 'left',
                background: sel ? 'var(--green-tint)' : 'var(--surface)',
                border: `1px solid ${sel ? 'rgba(95,208,139,.4)' : 'var(--surface-border)'}`,
                borderRadius: 14,
                padding: '15px 16px',
                display: 'flex',
                alignItems: 'center',
                gap: 13,
              }}
            >
              <span
                aria-hidden
                style={{
                  width: 22,
                  height: 22,
                  flex: 'none',
                  borderRadius: 7,
                  border: `1.5px solid ${sel ? 'var(--green)' : '#3a434e'}`,
                  background: sel ? 'var(--green)' : 'transparent',
                  color: 'var(--on-green)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 13,
                  fontWeight: 700,
                }}
              >
                {sel ? '✓' : ''}
              </span>
              <span style={{ minWidth: 0 }}>
                <span style={{ display: 'block', fontSize: 15, fontWeight: 600, color: sel ? 'var(--text)' : 'var(--text-2)' }}>
                  {g.label}
                </span>
                <span style={{ display: 'block', fontSize: 12, color: 'var(--muted)', marginTop: 2 }}>
                  {g.desc}
                </span>
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

export function MarkersStep({ w, setW }: StepProps) {
  const { data: fitness } = useFitness();
  const { data: profile } = useProfile();

  useEffect(() => {
    // Prefill once from existing profile values.
    setW((prev) => ({
      ...prev,
      markers: {
        zone2: prev.markers.zone2 || (profile?.zone2_ceiling_bpm ? String(profile.zone2_ceiling_bpm) : ''),
        lthr: prev.markers.lthr || (profile?.threshold_bpm ? String(profile.threshold_bpm) : ''),
        maxhr: prev.markers.maxhr || (profile?.max_hr_bpm ? String(profile.max_hr_bpm) : ''),
      },
    }));
  }, [profile, setW]);

  const easyPace = fitness?.easy_pace || '—';

  const field = (key: 'zone2' | 'lthr' | 'maxhr', label: string) => (
    <div className="card" style={{ borderRadius: 13, padding: 14 }}>
      <MonoLabel style={{ fontSize: 9, letterSpacing: '.12em' }}>{label}</MonoLabel>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 4, marginTop: 6 }}>
        <input
          aria-label={label}
          inputMode="numeric"
          value={w.markers[key]}
          placeholder="—"
          onChange={(e) => setW((prev) => ({ ...prev, markers: { ...prev.markers, [key]: e.target.value } }))}
          style={{
            width: 64,
            background: 'none',
            border: 'none',
            outline: 'none',
            fontSize: 21,
            fontWeight: 600,
            color: 'var(--text)',
            padding: 0,
          }}
        />
        <span style={{ fontSize: 11, color: 'var(--label)' }}>bpm</span>
      </div>
    </div>
  );

  return (
    <div className="fade-up">
      <div style={stepTitleStyle}>Your numbers</div>
      <div style={stepBodyStyle}>
        Prefilled where we can detect them; edit anything. All of it tunes zones and pace targets.
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 22 }}>
        {field('zone2', 'ZONE 2 CEILING')}
        {field('lthr', 'THRESHOLD LTHR')}
        {field('maxhr', 'MAX HR')}
        <div className="card" style={{ borderRadius: 13, padding: 14 }}>
          <MonoLabel style={{ fontSize: 9, letterSpacing: '.12em' }}>EASY PACE</MonoLabel>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 4, marginTop: 6 }}>
            <span style={{ fontSize: 21, fontWeight: 600 }}>{easyPace}</span>
            <span style={{ fontSize: 11, color: 'var(--label)' }}>/km</span>
          </div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--faint)', marginTop: 4 }}>
            detected from your runs
          </div>
        </div>
      </div>
    </div>
  );
}

export function RhythmStep({ w, setW }: StepProps) {
  return (
    <div className="fade-up">
      <div style={stepTitleStyle}>Your week</div>
      <div style={stepBodyStyle}>
        Roughly how you like to train. The coach periodises around this and protects your gym days.
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 22 }}>
        <Stepper
          label="Runs / week"
          value={w.runsPerWeek}
          onInc={() => setW((p) => ({ ...p, runsPerWeek: Math.min(9, p.runsPerWeek + 1) }))}
          onDec={() => setW((p) => ({ ...p, runsPerWeek: Math.max(0, p.runsPerWeek - 1) }))}
        />
        <Stepper
          label="CrossFit days"
          value={w.crossfitDays}
          onInc={() => setW((p) => ({ ...p, crossfitDays: Math.min(9, p.crossfitDays + 1) }))}
          onDec={() => setW((p) => ({ ...p, crossfitDays: Math.max(0, p.crossfitDays - 1) }))}
        />
        <div
          className="card"
          style={{ borderRadius: 14, padding: '14px 16px', display: 'flex', alignItems: 'center', gap: 12 }}
        >
          <span style={{ flex: 1, fontSize: 15 }}>Rest day</span>
          <select
            aria-label="Rest day"
            value={w.restDay}
            onChange={(e) => setW((p) => ({ ...p, restDay: e.target.value }))}
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: 14,
              color: 'var(--green)',
              background: 'var(--green-tint)',
              border: 'none',
              borderRadius: 8,
              padding: '6px 12px',
              outline: 'none',
            }}
          >
            {REST_DAYS.map((d) => (
              <option key={d} value={d}>
                {d[0].toUpperCase() + d.slice(1)}
              </option>
            ))}
          </select>
        </div>
      </div>
    </div>
  );
}

export function GuardrailsStep({ w, setW }: StepProps) {
  return (
    <div className="fade-up">
      <div style={stepTitleStyle}>Coach guardrails</div>
      <div style={stepBodyStyle}>The rules it will never break when it reshapes your week.</div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 9, marginTop: 20 }}>
        {RULE_DEFS.map((r) => (
          <div
            key={r.key}
            className="card"
            style={{ borderRadius: 13, padding: '13px 14px', display: 'flex', alignItems: 'center', gap: 12 }}
          >
            <span style={{ minWidth: 0, flex: 1 }}>
              <span style={{ display: 'block', fontSize: 14, fontWeight: 500 }}>{r.label}</span>
              <span style={{ display: 'block', fontSize: 12, color: 'var(--muted)', marginTop: 2, lineHeight: 1.4 }}>
                {r.desc}
              </span>
            </span>
            <Toggle
              on={w.rules[r.key]}
              onChange={(v) => setW((p) => ({ ...p, rules: { ...p.rules, [r.key]: v } }))}
              label={r.label}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
