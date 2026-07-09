import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { setup } from '../../../api/auth';
import { ApiError } from '../../../api/client';
import { inputStyle, stepBodyStyle, stepTitleStyle } from '../steps';

export function SecureStep({
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
