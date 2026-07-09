import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, apiPost } from '../../api/client';
import { setup } from '../../api/auth';
import { useAuthState, useFitness, useProfile, useStatus, useSync, useUpdateProfile } from '../../api/hooks';
import { pushSupported, subscribePush } from '../../lib/push';
import { MonoLabel, Stepper, Toggle } from '../../ui/kit';
import {
  GOAL_DEFS,
  INITIAL_WIZARD,
  REST_DAYS,
  RULE_DEFS,
  STEP_LABELS,
  stepBodyStyle,
  stepTitleStyle,
  wizardProfilePatch,
  type WizardState,
} from './steps';

const inputStyle = {
  width: '100%',
  background: 'var(--inset)',
  border: '1px solid var(--inset-border)',
  borderRadius: 12,
  padding: '13px 14px',
  fontSize: 15,
  color: 'var(--text)',
  outline: 'none',
} as const;

// ---------- step bodies ----------

function WelcomeStep() {
  return (
    <div className="fade-up" style={{ height: '100%', display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
      <div
        aria-hidden
        style={{
          width: 52,
          height: 52,
          borderRadius: 15,
          border: '1px solid var(--inset-border)',
          background: 'var(--subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          marginBottom: 26,
        }}
      >
        <span style={{ color: 'var(--green)', fontSize: 22 }}>↓</span>
      </div>
      <MonoLabel green style={{ fontSize: 11, letterSpacing: '.26em', marginBottom: 14 }}>
        RUNNING ON AI
      </MonoLabel>
      <div style={{ fontSize: 32, fontWeight: 700, lineHeight: 1.12, letterSpacing: '-.01em' }}>
        Your coach reads Garmin while you sleep.
      </div>
      <div style={{ fontSize: 15, color: 'var(--muted)', lineHeight: 1.6, marginTop: 16 }}>
        Sleep, HRV, Body Battery and load — read as one system every morning, so today’s plan is
        already right before you wake. Running that feeds your fitness, not just your log.
      </div>
    </div>
  );
}

function SecureStep({
  onDone,
  token,
}: {
  onDone: (apiToken: string) => void;
  token: string | null;
}) {
  const [pw, setPw] = useState('');
  const [confirm, setConfirm] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const qc = useQueryClient();

  if (token) {
    return (
      <div className="fade-up">
        <div style={stepTitleStyle}>Instance secured.</div>
        <div style={stepBodyStyle}>Your API token for scripts — shown only this once:</div>
        <div
          className="inset"
          style={{
            marginTop: 14,
            padding: '10px 12px',
            fontFamily: 'var(--font-mono)',
            fontSize: 12,
            color: 'var(--green)',
            wordBreak: 'break-all',
            userSelect: 'all',
          }}
        >
          {token}
        </div>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--faint)', marginTop: 12 }}>
          NEEDED ONLY FOR `make sync` & SCRIPTS · REGENERATE ANYTIME IN SETTINGS
        </div>
      </div>
    );
  }

  return (
    <form
      className="fade-up"
      onSubmit={(e) => {
        e.preventDefault();
        setErr(null);
        if (pw.length < 8) {
          setErr('Use at least 8 characters.');
          return;
        }
        if (pw !== confirm) {
          setErr('Passwords don’t match.');
          return;
        }
        setBusy(true);
        setup(pw)
          .then((r) => {
            void qc.invalidateQueries({ queryKey: ['auth'] });
            onDone(r.api_token);
          })
          .catch((e: Error) =>
            setErr(e instanceof ApiError && e.status === 409 ? 'Already set up — sign in instead.' : e.message),
          )
          .finally(() => setBusy(false));
      }}
    >
      <div style={stepTitleStyle}>Secure this instance</div>
      <div style={stepBodyStyle}>
        Set the owner password — you’ll use it to sign in from any device. It never leaves this
        server.
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 22 }}>
        <input
          aria-label="Password"
          type="password"
          placeholder="Password (8+ characters)"
          value={pw}
          onChange={(e) => setPw(e.target.value)}
          style={inputStyle}
        />
        <input
          aria-label="Confirm password"
          type="password"
          placeholder="Confirm password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          style={inputStyle}
        />
      </div>
      {err && (
        <div className="error-line" style={{ marginTop: 10 }}>
          {err}
        </div>
      )}
      <button type="submit" className="btn-primary" disabled={busy || !pw || !confirm} style={{ width: '100%', marginTop: 18 }}>
        {busy ? 'Securing…' : 'Set password'}
      </button>
    </form>
  );
}

type GarminPhase = 'form' | 'mfa' | 'connecting' | 'connected';

function GarminStep({ onConnected }: { onConnected: () => void }) {
  const [phase, setPhase] = useState<GarminPhase>('form');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [loginID, setLoginID] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const sync = useSync();
  const { data: status } = useStatus();

  const startLogin = () => {
    setErr(null);
    setPhase('connecting');
    apiPost<{ status: string; login_id?: string }>('/api/garmin/login', { email, password })
      .then((r) => {
        if (r.status === 'ok') {
          setPhase('connected');
          sync.mutate();
          onConnected();
        } else if (r.status === 'mfa_required' && r.login_id) {
          setLoginID(r.login_id);
          setPhase('mfa');
        }
      })
      .catch((e: Error) => {
        setErr(e.message);
        setPhase('form');
      });
  };

  const submitCode = () => {
    setErr(null);
    setPhase('connecting');
    apiPost<{ status: string }>('/api/garmin/login/mfa', { login_id: loginID, code })
      .then(() => {
        setPhase('connected');
        sync.mutate();
        onConnected();
      })
      .catch((e: Error) => {
        setErr(e.message);
        setPhase('mfa');
      });
  };

  const syncRows = [
    { name: 'Sleep & stages', done: (status?.counts.recovery_days ?? 0) > 0 },
    { name: 'Overnight HRV', done: (status?.counts.recovery_days ?? 0) > 0 },
    { name: 'Body Battery', done: (status?.counts.recovery_days ?? 0) > 0 },
    { name: 'Resting HR', done: (status?.counts.recovery_days ?? 0) > 0 },
    { name: 'Runs · pace, HR, splits', done: (status?.counts.activities ?? 0) > 0 },
  ];

  return (
    <div className="fade-up">
      <div style={stepTitleStyle}>Connect Garmin</div>
      <div style={stepBodyStyle}>
        We pull straight from Garmin Connect — sleep, HRV and Body Battery that no run-tracker can
        see. Read-only. Nothing is posted back.
      </div>

      {phase === 'form' && (
        <>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 22 }}>
            <input
              aria-label="Garmin email"
              type="email"
              placeholder="Garmin Connect email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              style={inputStyle}
            />
            <input
              aria-label="Garmin password"
              type="password"
              placeholder="Garmin Connect password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              style={inputStyle}
            />
          </div>
          {err && (
            <div className="error-line" style={{ marginTop: 10 }}>
              {err}
            </div>
          )}
          <button
            className="btn-primary"
            style={{ width: '100%', marginTop: 18 }}
            disabled={!email || !password}
            onClick={startLogin}
          >
            Sign in to Garmin Connect
          </button>
          <div
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: 10,
              color: 'var(--faint)',
              textAlign: 'center',
              marginTop: 12,
            }}
          >
            CREDENTIALS USED ONCE · TOKEN STORED · MFA ONCE · UNATTENDED NIGHTLY PULL
          </div>
        </>
      )}

      {phase === 'mfa' && (
        <div className="fade-up" style={{ marginTop: 22 }}>
          <div style={{ fontSize: 14, color: 'var(--text-2)' }}>
            Enter the code Garmin sent you.
          </div>
          <input
            aria-label="MFA code"
            inputMode="numeric"
            placeholder="123456"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            style={{ ...inputStyle, marginTop: 10, fontFamily: 'var(--font-mono)', letterSpacing: '.3em' }}
          />
          {err && (
            <div className="error-line" style={{ marginTop: 10 }}>
              {err}
            </div>
          )}
          <button
            className="btn-primary"
            style={{ width: '100%', marginTop: 14 }}
            disabled={code.length < 4}
            onClick={submitCode}
          >
            Verify code
          </button>
        </div>
      )}

      {phase === 'connecting' && (
        <div style={{ marginTop: 40, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 16 }}>
          <div className="spinner" role="status" aria-label="signing in" />
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--muted)' }}>
            Signing in to Garmin Connect…
          </div>
        </div>
      )}

      {phase === 'connected' && (
        <div className="fade-up">
          <div
            style={{
              marginTop: 22,
              background: 'var(--green-tint)',
              border: '1px solid var(--green-border)',
              borderRadius: 14,
              padding: '14px 16px',
              display: 'flex',
              alignItems: 'center',
              gap: 10,
            }}
          >
            <span
              aria-hidden
              style={{
                width: 22,
                height: 22,
                borderRadius: '50%',
                background: 'var(--green)',
                color: 'var(--on-green)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 13,
                fontWeight: 700,
              }}
            >
              ✓
            </span>
            <span style={{ fontSize: 14, color: '#DCEFE3' }}>Connected to Garmin</span>
          </div>
          <MonoLabel style={{ margin: '20px 0 10px', letterSpacing: '.18em' }}>NOW SYNCING</MonoLabel>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {syncRows.map((row) => (
              <div
                key={row.name}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 11,
                  padding: '11px 4px',
                  borderTop: '1px solid var(--hairline)',
                }}
              >
                {row.done ? (
                  <span style={{ color: 'var(--green)', fontSize: 13 }}>✓</span>
                ) : (
                  <span className="pulse-dot" style={{ background: 'var(--label)' }} />
                )}
                <span style={{ flex: 1, fontSize: 14, color: 'var(--text-2)' }}>{row.name}</span>
              </div>
            ))}
          </div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--faint)', marginTop: 12 }}>
            FIRST SYNC RUNS IN THE BACKGROUND — CONTINUE WHENEVER
          </div>
        </div>
      )}
    </div>
  );
}

function GoalStep({ w, setW }: { w: WizardState; setW: (fn: (w: WizardState) => WizardState) => void }) {
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

function MarkersStep({ w, setW }: { w: WizardState; setW: (fn: (w: WizardState) => WizardState) => void }) {
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

function RhythmStep({ w, setW }: { w: WizardState; setW: (fn: (w: WizardState) => WizardState) => void }) {
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

function GuardrailsStep({ w, setW }: { w: WizardState; setW: (fn: (w: WizardState) => WizardState) => void }) {
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

function ReadyStep({ w }: { w: WizardState }) {
  const { data: status } = useStatus();
  const [pushLine, setPushLine] = useState<string | null>(null);
  const nextRun = status?.agent_next_run
    ? new Date(status.agent_next_run).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : '06:00';

  return (
    <div className="fade-up" style={{ height: '100%', display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
      <div
        aria-hidden
        style={{
          width: 56,
          height: 56,
          borderRadius: '50%',
          background: 'var(--green)',
          color: 'var(--on-green)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 26,
          fontWeight: 700,
          marginBottom: 24,
        }}
      >
        ✓
      </div>
      <div style={{ fontSize: 30, fontWeight: 700, lineHeight: 1.14 }}>You’re set.</div>
      <div style={{ fontSize: 15, color: 'var(--muted)', lineHeight: 1.6, marginTop: 14 }}>
        Your first readout lands at <b style={{ color: 'var(--text-2)' }}>{nextRun}</b> — readiness,
        today’s session and what changed, before you’re awake. Check in anytime; ask the coach
        anything about your own data.
      </div>
      <div
        className="card--subtle"
        style={{
          marginTop: 22,
          padding: '14px 16px',
          fontFamily: 'var(--font-mono)',
          fontSize: 11,
          color: 'var(--muted)',
          lineHeight: 1.7,
        }}
      >
        NIGHTLY PULL · ARMED
        <br />
        AGENT · RUNS {nextRun} DAILY
        {w.rules.load_cap_55 && (
          <>
            <br />
            RUNNING CAP · ≤55% OF LOAD
          </>
        )}
      </div>
      {pushSupported() && (
        <>
          <button
            className="btn-secondary"
            style={{ marginTop: 16 }}
            onClick={() =>
              subscribePush()
                .then(() => setPushLine('Morning notifications enabled ✓'))
                .catch((e: Error) => setPushLine(e.message))
            }
          >
            Enable morning notifications
          </button>
          {pushLine && (
            <div className={pushLine.endsWith('✓') ? 'ok-line' : 'error-line'} style={{ marginTop: 8 }}>
              {pushLine}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------- the wizard shell ----------

export function OnboardingPage() {
  const [params] = useSearchParams();
  const { data: auth } = useAuthState();
  const nav = useNavigate();
  const update = useUpdateProfile();
  const { data: profile } = useProfile();

  // Settings re-entry: /onboarding?step=garmin on an already-authed instance.
  const reEntry = params.get('step') === 'garmin';
  const [step, setStep] = useState(reEntry && auth?.authed ? 2 : 0);
  const [w, setW] = useState<WizardState>(INITIAL_WIZARD);
  const [garminDone, setGarminDone] = useState(false);

  const next = () => {
    if (step === 7) {
      // Persist everything in one PUT, preserving untouched fields.
      const patch = wizardProfilePatch(w);
      update.mutate(
        {
          target_weekly_km: profile?.target_weekly_km ?? 20,
          progression_mode: profile?.progression_mode || 'build',
          run_constraints_json: profile?.run_constraints_json || '{}',
          goal_text: profile?.goal_text || 'Build the aerobic engine',
          daily_run_time: profile?.daily_run_time || '06:00',
          timezone: profile?.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
          agent_enabled: true,
          ...patch,
        },
        { onSuccess: () => nav('/') },
      );
      return;
    }
    if (reEntry && step === 2 && garminDone) {
      nav('/settings');
      return;
    }
    setStep((s) => Math.min(7, s + 1));
  };

  const back = () => setStep((s) => Math.max(0, s - 1));

  const canContinue = step === 1 ? w.apiToken !== null : step === 2 ? garminDone || reEntry : true;

  const nextLabel =
    step === 0 ? 'Get started' : step === 7 ? (update.isPending ? 'Saving…' : 'Open today') : 'Continue';

  return (
    <div style={{ minHeight: '100dvh', display: 'flex', flexDirection: 'column', maxWidth: 480, margin: '0 auto' }}>
      <div style={{ flex: 'none', padding: '18px 22px 4px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 9 }}>
          <MonoLabel style={{ letterSpacing: '.2em' }}>SETUP · {STEP_LABELS[step]}</MonoLabel>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--green)' }}>
            {step + 1}/8
          </span>
        </div>
        <div style={{ height: 3, background: '#1a212a', borderRadius: 2, overflow: 'hidden' }}>
          <div
            style={{
              height: '100%',
              background: 'var(--green)',
              borderRadius: 2,
              width: `${Math.round(((step + 1) / 8) * 100)}%`,
              transition: 'width .35s ease',
            }}
          />
        </div>
      </div>

      <div className="scroll-pane" style={{ flex: 1, minHeight: 0, padding: '22px 22px 8px' }}>
        {step === 0 && <WelcomeStep />}
        {step === 1 && (
          <SecureStep token={w.apiToken} onDone={(tok) => setW((p) => ({ ...p, apiToken: tok }))} />
        )}
        {step === 2 && <GarminStep onConnected={() => setGarminDone(true)} />}
        {step === 3 && <GoalStep w={w} setW={setW} />}
        {step === 4 && <MarkersStep w={w} setW={setW} />}
        {step === 5 && <RhythmStep w={w} setW={setW} />}
        {step === 6 && <GuardrailsStep w={w} setW={setW} />}
        {step === 7 && <ReadyStep w={w} />}
      </div>

      <div style={{ flex: 'none', display: 'flex', gap: 10, padding: '12px 22px 22px' }}>
        {step > 0 && step !== 7 && (
          <button className="btn-secondary" onClick={back} style={{ flex: 'none' }}>
            Back
          </button>
        )}
        <button className="btn-primary" style={{ flex: 1 }} disabled={!canContinue || update.isPending} onClick={next}>
          {nextLabel}
        </button>
      </div>
    </div>
  );
}
