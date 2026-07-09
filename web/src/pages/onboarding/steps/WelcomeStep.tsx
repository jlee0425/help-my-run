import { MonoLabel } from '../../../ui/kit';

export function WelcomeStep() {
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
