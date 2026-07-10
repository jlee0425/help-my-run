import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { changePassword, regenerateToken } from '../api/auth';
import {
  useClaudeStatus,
  useRevokeOtherSessions,
  useRevokeSession,
  useSessions,
  useClaudeTokenDelete,
  useClaudeTokenSet,
  useGarminDisconnect,
  useGarminStatus,
  useLogout,
  useProfile,
  useStatus,
  useSync,
  useUpdateProfile,
} from '../api/hooks';
import { describeUA } from '../lib/devices';
import { pushEnabled, pushSupported, sendTestPush, subscribePush, unsubscribePush } from '../lib/push';
import { MonoLabel, Toggle } from '../ui/kit';

const inputStyle = {
  width: '100%',
  background: 'var(--inset)',
  border: '1px solid var(--inset-border)',
  borderRadius: 10,
  padding: '10px 12px',
  fontSize: 14,
  color: 'var(--text)',
  outline: 'none',
} as const;

function Dot({ ok }: { ok: boolean }) {
  return (
    <span
      aria-hidden
      style={{
        width: 8,
        height: 8,
        borderRadius: '50%',
        background: ok ? 'var(--green)' : 'var(--faint)',
        display: 'inline-block',
      }}
    />
  );
}

function SettingsCard({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="card" style={{ padding: 18 }}>
      <MonoLabel>{label}</MonoLabel>
      <div style={{ marginTop: 12 }}>{children}</div>
    </div>
  );
}

function GarminCard() {
  const nav = useNavigate();
  const { data: garmin, isError } = useGarminStatus();
  const disconnect = useGarminDisconnect();

  return (
    <SettingsCard label="// GARMIN">
      {isError || garmin === null ? (
        <div style={{ fontSize: 13, color: 'var(--muted)' }}>Status unavailable.</div>
      ) : (
        <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
          <Dot ok={!!garmin?.connected} />
          <span style={{ fontSize: 14, color: 'var(--text-2)' }}>
            {garmin?.connected ? 'Connected' : 'Not connected'}
          </span>
          {garmin?.last_synced_at && (
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--faint)' }}>
              LAST SYNC {new Date(garmin.last_synced_at).toLocaleString()}
            </span>
          )}
        </div>
      )}
      <div style={{ display: 'flex', gap: 8, marginTop: 14 }}>
        <button className="btn-secondary" onClick={() => nav('/onboarding?step=garmin')}>
          {garmin?.connected ? 'Reconnect' : 'Connect Garmin'}
        </button>
        {garmin?.connected && (
          <button
            className="btn-secondary"
            style={{ color: 'var(--red)' }}
            disabled={disconnect.isPending}
            onClick={() => {
              if (window.confirm('Disconnect Garmin? Stored tokens are deleted.'))
                disconnect.mutate();
            }}
          >
            Disconnect
          </button>
        )}
      </div>
    </SettingsCard>
  );
}

function ClaudeCard() {
  const { data: claude, isError, refetch } = useClaudeStatus();
  const setToken = useClaudeTokenSet();
  const delToken = useClaudeTokenDelete();
  const [token, setTokenDraft] = useState('');

  return (
    <SettingsCard label="// CLAUDE">
      {isError || claude === null ? (
        <div style={{ fontSize: 13, color: 'var(--muted)' }}>Status unavailable.</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
            <Dot ok={!!claude?.binary_found} />
            <span style={{ fontSize: 14, color: 'var(--text-2)' }}>
              {claude?.binary_found ? 'claude CLI found' : 'claude CLI not installed'}
            </span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
            <Dot ok={!!claude?.authenticated} />
            <span style={{ fontSize: 14, color: 'var(--text-2)' }}>
              {claude?.authenticated
                ? `Subscription active · ${claude.model}`
                : claude?.detail || 'Not logged in'}
            </span>
          </div>
        </div>
      )}
      <div style={{ fontSize: 12, color: 'var(--faint)', lineHeight: 1.5, marginTop: 10 }}>
        Runs on your Claude subscription via the claude CLI — no API key, $0 per token. Normally you
        log in once on the server with <code>claude auth login</code>.
      </div>
      <details style={{ marginTop: 10 }}>
        <summary style={{ fontSize: 13, color: 'var(--muted)', cursor: 'pointer' }}>
          Headless server? Paste a setup-token
        </summary>
        <div style={{ marginTop: 10 }}>
          <input
            aria-label="Claude setup token"
            placeholder="sk-ant-oat…  (from `claude setup-token` on any machine)"
            value={token}
            onChange={(e) => setTokenDraft(e.target.value)}
            style={inputStyle}
          />
          {setToken.error && (
            <div className="error-line" style={{ marginTop: 8 }}>
              {(setToken.error as Error).message}
            </div>
          )}
          <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
            <button
              className="btn-secondary"
              disabled={!token.trim() || setToken.isPending}
              onClick={() =>
                setToken.mutate(token.trim(), {
                  onSuccess: () => {
                    setTokenDraft('');
                    void refetch();
                  },
                })
              }
            >
              Save token
            </button>
            <button
              className="btn-secondary"
              disabled={delToken.isPending}
              onClick={() => delToken.mutate()}
            >
              Remove stored token
            </button>
          </div>
        </div>
      </details>
    </SettingsCard>
  );
}

function NotificationsCard() {
  const supported = pushSupported();
  const [enabled, setEnabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const [line, setLine] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (supported) void pushEnabled().then(setEnabled);
  }, [supported]);

  const flip = async (next: boolean) => {
    setBusy(true);
    setLine(null);
    try {
      if (next) await subscribePush();
      else await unsubscribePush();
      setEnabled(next);
      setLine({ ok: true, text: next ? 'Morning briefings enabled.' : 'Notifications off.' });
    } catch (e) {
      setLine({ ok: false, text: (e as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <SettingsCard label="// NOTIFICATIONS">
      {!supported ? (
        <div style={{ fontSize: 13, color: 'var(--muted)', lineHeight: 1.5 }}>
          This browser doesn’t support Web Push. On iPhone, install the app to your home screen
          first (Share → Add to Home Screen).
        </div>
      ) : (
        <>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 14, color: 'var(--text-2)' }}>Morning briefing push</div>
              <div style={{ fontSize: 12, color: 'var(--faint)', marginTop: 2 }}>
                The coach’s daily verdict, delivered when it runs.
              </div>
            </div>
            <Toggle on={enabled} onChange={(v) => void flip(v)} label="Morning briefing push" />
          </div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 12 }}>
            <button
              className="btn-secondary"
              disabled={!enabled || busy}
              onClick={() =>
                void sendTestPush()
                  .then(() => setLine({ ok: true, text: 'Test sent — check your notifications.' }))
                  .catch((e: Error) => setLine({ ok: false, text: e.message }))
              }
            >
              Send test
            </button>
            {busy && <div className="spinner" style={{ width: 20, height: 20 }} aria-label="working" />}
          </div>
        </>
      )}
      {line && (
        <div className={line.ok ? 'ok-line' : 'error-line'} style={{ marginTop: 10 }}>
          {line.text}
        </div>
      )}
    </SettingsCard>
  );
}

function SyncCard() {
  const { data: status } = useStatus();
  const sync = useSync();
  const { data: profile } = useProfile();
  const update = useUpdateProfile();

  const agentEnabled = profile?.agent_enabled ?? true;

  return (
    <SettingsCard label="// SYNC & AGENT">
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--muted)', lineHeight: 1.8 }}>
        GARMIN · {status?.garmin.status?.toUpperCase() ?? '—'}
        {status?.garmin.last_synced_at
          ? ` · LAST ${new Date(status.garmin.last_synced_at).toLocaleString()}`
          : ''}
        <br />
        DATA · {status?.counts.activities ?? 0} ACTIVITIES · {status?.counts.recovery_days ?? 0}{' '}
        RECOVERY DAYS
      </div>
      {sync.error && (
        <div className="error-line" style={{ marginTop: 8 }}>
          {(sync.error as Error).message}
        </div>
      )}
      <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
        <button className="btn-secondary" disabled={sync.isPending} onClick={() => sync.mutate()}>
          {sync.isPending ? 'Syncing…' : 'Sync now'}
        </button>
      </div>

      {profile && (
        <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 14, color: 'var(--text-2)' }}>Daily agent</div>
              <div style={{ fontSize: 12, color: 'var(--faint)', marginTop: 2 }}>
                Runs every morning: sync, readiness, reshape today.
              </div>
            </div>
            <Toggle
              on={agentEnabled}
              onChange={(v) => update.mutate({ ...profile, agent_enabled: v })}
              label="Daily agent"
            />
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <div style={{ flex: 1 }}>
              <MonoLabel style={{ fontSize: 9, marginBottom: 6 }}>RUN TIME</MonoLabel>
              <input
                aria-label="Agent run time"
                defaultValue={profile.daily_run_time || '05:30'}
                onBlur={(e) => {
                  const v = e.target.value;
                  if (v !== profile.daily_run_time) update.mutate({ ...profile, daily_run_time: v });
                }}
                style={inputStyle}
              />
            </div>
            <div style={{ flex: 2 }}>
              <MonoLabel style={{ fontSize: 9, marginBottom: 6 }}>TIMEZONE (IANA)</MonoLabel>
              <input
                aria-label="Agent timezone"
                defaultValue={profile.timezone || 'UTC'}
                onBlur={(e) => {
                  const v = e.target.value;
                  if (v !== profile.timezone) update.mutate({ ...profile, timezone: v });
                }}
                style={inputStyle}
              />
            </div>
          </div>
          {update.error && (
            <div className="error-line">{(update.error as Error).message}</div>
          )}
        </div>
      )}
    </SettingsCard>
  );
}

function DevicesBlock() {
  const { data: sessions, error: sessionsError } = useSessions();
  const revoke = useRevokeSession();
  const revokeOthers = useRevokeOtherSessions();

  if (sessionsError) {
    return (
      <div style={{ borderTop: '1px solid var(--hairline)', marginTop: 16, paddingTop: 16 }}>
        <div style={{ fontSize: 14, color: 'var(--text-2)' }}>Devices</div>
        <div className="error-line" style={{ marginTop: 8 }}>
          Devices unavailable: {(sessionsError as Error).message}
        </div>
      </div>
    );
  }
  if (!sessions || sessions.length === 0) return null;
  const others = sessions.filter((s) => !s.current).length;

  return (
    <div style={{ borderTop: '1px solid var(--hairline)', marginTop: 16, paddingTop: 16 }}>
      <div style={{ fontSize: 14, color: 'var(--text-2)' }}>Devices</div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 10 }}>
        {sessions.map((s) => (
          <div
            key={s.id_hash}
            className="card--subtle"
            style={{ padding: '10px 12px', display: 'flex', alignItems: 'center', gap: 10 }}
          >
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 13, color: 'var(--text)' }}>
                {describeUA(s.user_agent)}
                {s.current && (
                  <span
                    style={{
                      marginLeft: 8,
                      fontFamily: 'var(--font-mono)',
                      fontSize: 9,
                      letterSpacing: '.1em',
                      color: 'var(--green)',
                      background: 'var(--green-tint)',
                      borderRadius: 8,
                      padding: '2px 7px',
                    }}
                  >
                    THIS DEVICE
                  </span>
                )}
              </div>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--faint)', marginTop: 3 }}>
                {s.ip && `${s.ip} · `}ADDED {new Date(s.created_at).toLocaleDateString()} · LAST SEEN{' '}
                {new Date(s.last_seen_at).toLocaleString()}
              </div>
            </div>
            {!s.current && (
              <button
                className="btn-secondary"
                style={{ padding: '6px 12px', fontSize: 12, color: 'var(--red)' }}
                disabled={revoke.isPending}
                aria-label={`Revoke ${describeUA(s.user_agent)}`}
                onClick={() => revoke.mutate(s.id_hash)}
              >
                Revoke
              </button>
            )}
          </div>
        ))}
      </div>
      {others > 0 && (
        <button
          className="btn-secondary"
          style={{ marginTop: 10 }}
          disabled={revokeOthers.isPending}
          onClick={() => {
            if (window.confirm('Sign out everywhere else? Other devices will need the password again.'))
              revokeOthers.mutate();
          }}
        >
          Sign out everywhere else
        </button>
      )}
      {revoke.error && (
        <div className="error-line" style={{ marginTop: 8 }}>
          {(revoke.error as Error).message}
        </div>
      )}
      {revokeOthers.error && (
        <div className="error-line" style={{ marginTop: 8 }}>
          {(revokeOthers.error as Error).message}
        </div>
      )}
    </div>
  );
}

function SecurityCard() {
  const logout = useLogout();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [pwLine, setPwLine] = useState<{ ok: boolean; text: string } | null>(null);
  const [newToken, setNewToken] = useState<string | null>(null);
  const [tokenErr, setTokenErr] = useState<string | null>(null);

  return (
    <SettingsCard label="// SECURITY">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setPwLine(null);
          changePassword(current, next)
            .then(() => {
              setCurrent('');
              setNext('');
              setPwLine({ ok: true, text: 'Password changed.' });
            })
            .catch((err: Error) => setPwLine({ ok: false, text: err.message }));
        }}
        style={{ display: 'flex', flexDirection: 'column', gap: 10 }}
      >
        <div style={{ fontSize: 14, color: 'var(--text-2)' }}>Change password</div>
        <input
          aria-label="Current password"
          type="password"
          placeholder="Current password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          style={inputStyle}
        />
        <input
          aria-label="New password"
          type="password"
          placeholder="New password (8+ characters)"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          style={inputStyle}
        />
        {pwLine && <div className={pwLine.ok ? 'ok-line' : 'error-line'}>{pwLine.text}</div>}
        <button className="btn-secondary" type="submit" disabled={!current || !next}>
          Change password
        </button>
      </form>

      <div style={{ borderTop: '1px solid var(--hairline)', marginTop: 16, paddingTop: 16 }}>
        <div style={{ fontSize: 14, color: 'var(--text-2)' }}>API token (for scripts)</div>
        <div style={{ fontSize: 12, color: 'var(--faint)', lineHeight: 1.5, marginTop: 4 }}>
          Used as <code>Authorization: Bearer …</code> by <code>make sync</code> and friends. Shown
          once when regenerated — store it somewhere safe.
        </div>
        {newToken && (
          <div
            className="inset"
            style={{
              marginTop: 10,
              padding: '10px 12px',
              fontFamily: 'var(--font-mono)',
              fontSize: 12,
              color: 'var(--green)',
              wordBreak: 'break-all',
              userSelect: 'all',
            }}
          >
            {newToken}
          </div>
        )}
        {tokenErr && (
          <div className="error-line" style={{ marginTop: 8 }}>
            {tokenErr}
          </div>
        )}
        <button
          className="btn-secondary"
          style={{ marginTop: 10 }}
          onClick={() => {
            setTokenErr(null);
            if (!window.confirm('Regenerate the API token? The old one stops working.')) return;
            regenerateToken()
              .then((r) => setNewToken(r.api_token))
              .catch((e: Error) => setTokenErr(e.message));
          }}
        >
          Regenerate token
        </button>
      </div>

      <DevicesBlock />

      <div style={{ borderTop: '1px solid var(--hairline)', marginTop: 16, paddingTop: 16 }}>
        <button
          className="btn-secondary"
          style={{ color: 'var(--red)' }}
          disabled={logout.isPending}
          onClick={() => logout.mutate()}
        >
          Log out
        </button>
      </div>
    </SettingsCard>
  );
}

export function SettingsPage() {
  return (
    <div
      className="scroll-pane"
      style={{ flex: 1, padding: '2px 18px 24px', maxWidth: 620, display: 'flex', flexDirection: 'column', gap: 12 }}
    >
      <div style={{ padding: '6px 0 2px' }}>
        <MonoLabel>{'// SETTINGS · THIS INSTANCE'}</MonoLabel>
      </div>
      <GarminCard />
      <ClaudeCard />
      <NotificationsCard />
      <SyncCard />
      <SecurityCard />
    </div>
  );
}
