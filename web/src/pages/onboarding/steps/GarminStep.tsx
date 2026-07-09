import { useState } from 'react';
import { apiPost } from '../../../api/client';
import { useStatus, useSync } from '../../../api/hooks';
import { MonoLabel } from '../../../ui/kit';
import { inputStyle, stepBodyStyle, stepTitleStyle } from '../steps';

type GarminPhase = 'form' | 'mfa' | 'connecting' | 'connected';

export function GarminStep({ onConnected }: { onConnected: () => void }) {
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
                ) : sync.isError ? (
                  <span style={{ color: 'var(--red)', fontSize: 13 }}>✕</span>
                ) : (
                  <span className="pulse-dot" style={{ background: 'var(--label)' }} />
                )}
                <span style={{ flex: 1, fontSize: 14, color: 'var(--text-2)' }}>{row.name}</span>
              </div>
            ))}
          </div>
          {sync.isError ? (
            <div
              style={{
                marginTop: 12,
                background: 'var(--red-tint)',
                border: '1px solid var(--red-border)',
                borderRadius: 12,
                padding: '11px 13px',
              }}
            >
              <div style={{ fontSize: 13, color: 'var(--red)', lineHeight: 1.5 }}>
                First sync failed: {(sync.error as Error).message}
              </div>
              <button
                className="btn-secondary"
                style={{ marginTop: 10, padding: '8px 14px', fontSize: 13 }}
                disabled={sync.isPending}
                onClick={() => sync.mutate()}
              >
                {sync.isPending ? 'Retrying…' : 'Retry sync'}
              </button>
            </div>
          ) : (
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--faint)', marginTop: 12 }}>
              FIRST SYNC RUNS IN THE BACKGROUND — CONTINUE WHENEVER
            </div>
          )}
        </div>
      )}
    </div>
  );
}
