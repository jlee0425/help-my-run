import { useState } from 'react';
import { ApiError } from '../api/client';
import { useLogin } from '../api/hooks';

export function LoginPage() {
  const [password, setPassword] = useState('');
  const loginMut = useLogin();

  const error =
    loginMut.error instanceof ApiError
      ? loginMut.error.status === 429
        ? 'Too many attempts — wait a moment.'
        : 'Wrong password.'
      : null;

  return (
    <div
      style={{
        minHeight: '100dvh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 22,
      }}
    >
      <form
        className="fade-up"
        style={{ width: 360, maxWidth: '100%' }}
        onSubmit={(e) => {
          e.preventDefault();
          if (password) loginMut.mutate(password);
        }}
      >
        <div
          className="mono-label mono-label--green"
          style={{ fontSize: 11, letterSpacing: '.26em', marginBottom: 14 }}
        >
          HELP MY RUN
        </div>
        <div style={{ fontSize: 28, fontWeight: 700, lineHeight: 1.15, marginBottom: 22 }}>
          Welcome back.
        </div>
        <label className="mono-label" htmlFor="pw" style={{ display: 'block', marginBottom: 8 }}>
          PASSWORD
        </label>
        <input
          id="pw"
          type="password"
          autoFocus
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          style={{
            width: '100%',
            background: 'var(--inset)',
            border: '1px solid var(--inset-border)',
            borderRadius: 12,
            padding: '13px 14px',
            fontSize: 15,
            color: 'var(--text)',
            outline: 'none',
          }}
        />
        {error && (
          <div className="error-line" style={{ marginTop: 10 }}>
            {error}
          </div>
        )}
        <button
          type="submit"
          className="btn-primary"
          disabled={loginMut.isPending || !password}
          style={{ width: '100%', marginTop: 18 }}
        >
          {loginMut.isPending ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </div>
  );
}
