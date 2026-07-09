import { useState } from 'react';
import { useStatus } from '../../../api/hooks';
import { pushSupported, subscribePush } from '../../../lib/push';
import type { WizardState } from '../steps';

export function ReadyStep({ w }: { w: WizardState }) {
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
