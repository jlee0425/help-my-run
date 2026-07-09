import { useLocation, useNavigate } from 'react-router';
import { useStatus } from '../api/hooks';

const NAV = [
  { id: '/', label: 'Today' },
  { id: '/trends', label: 'Trends' },
  { id: '/coach', label: 'Coach' },
  { id: '/plan', label: 'Plan' },
  { id: '/settings', label: 'Settings' },
];

function nextRunLabel(iso: string | null | undefined): string {
  if (!iso) return 'paused';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return `Next run ${d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
}

/** Desktop left rail (CoachWeb design): brand, nav, agent card, profile chip. */
export function Rail() {
  const nav = useNavigate();
  const { pathname } = useLocation();
  const { data: status } = useStatus();

  return (
    <div
      style={{
        flex: 'none',
        width: 236,
        borderRight: '1px solid var(--hairline)',
        background: '#0B0E13',
        display: 'flex',
        flexDirection: 'column',
        padding: '22px 16px',
        minHeight: '100vh',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 9, padding: '0 6px 22px' }}>
        <span
          aria-hidden
          style={{
            width: 30,
            height: 30,
            borderRadius: 9,
            border: '1px solid var(--inset-border)',
            background: 'var(--subtle)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: 'var(--green)',
            fontSize: 15,
          }}
        >
          ↓
        </span>
        <span
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 11,
            letterSpacing: '.16em',
            color: 'var(--text-2)',
          }}
        >
          RUNNING ON AI
        </span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        {NAV.map((n) => {
          const active = pathname === n.id;
          return (
            <button
              key={n.id}
              onClick={() => nav(n.id)}
              style={{
                textAlign: 'left',
                display: 'flex',
                alignItems: 'center',
                gap: 11,
                background: active ? 'var(--green-tint)' : 'transparent',
                border: `1px solid ${active ? 'var(--green-border)' : 'transparent'}`,
                borderRadius: 11,
                padding: '11px 13px',
                color: active ? 'var(--text)' : 'var(--muted)',
                fontSize: 14,
                fontWeight: 500,
              }}
            >
              <span
                aria-hidden
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: '50%',
                  background: active ? 'var(--green)' : '#3a434e',
                }}
              />
              {n.label}
            </button>
          );
        })}
      </div>

      <div style={{ marginTop: 'auto' }}>
        <div className="card--subtle" style={{ padding: 12 }}>
          <div className="mono-label" style={{ fontSize: 9, letterSpacing: '.14em' }}>
            AGENT · {status?.agent_enabled === false ? 'PAUSED' : 'ARMED'}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 8 }}>
            <span
              aria-hidden
              style={{
                width: 7,
                height: 7,
                borderRadius: '50%',
                background: status?.agent_enabled === false ? 'var(--faint)' : 'var(--green)',
              }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-2)' }}>
              {nextRunLabel(status?.agent_next_run)}
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
