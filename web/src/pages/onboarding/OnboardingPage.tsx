// The 8-step first-run wizard shell. Step bodies live in ./steps/ — this file
// owns only wizard state, progression rules, and the final profile PUT.
import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { useAuthState, useProfile, useUpdateProfile } from '../../api/hooks';
import { MonoLabel } from '../../ui/kit';
import { INITIAL_WIZARD, STEP_LABELS, wizardProfilePatch, type WizardState } from './steps';
import { WelcomeStep } from './steps/WelcomeStep';
import { SecureStep } from './steps/SecureStep';
import { GarminStep } from './steps/GarminStep';
import { GoalStep, GuardrailsStep, MarkersStep, RhythmStep } from './steps/ProfileSteps';
import { ReadyStep } from './steps/ReadyStep';

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
