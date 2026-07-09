import { useLocation, useNavigate } from 'react-router';

const TABS = [
  { id: '/', label: 'TODAY' },
  { id: '/trends', label: 'TRENDS' },
  { id: '/coach', label: 'COACH' },
];

/** Mobile bottom tab bar (RunCoachPhone design, direction A). */
export function TabBar() {
  const nav = useNavigate();
  const { pathname } = useLocation();
  return (
    <nav
      style={{
        flex: 'none',
        display: 'flex',
        borderTop: '1px solid var(--hairline)',
        background: '#0A0D12',
        paddingBottom: 'env(safe-area-inset-bottom, 8px)',
      }}
    >
      {TABS.map((tb) => {
        const active = pathname === tb.id;
        return (
          <button
            key={tb.id}
            onClick={() => nav(tb.id)}
            style={{
              flex: 1,
              background: 'none',
              border: 'none',
              borderTop: `2px solid ${active ? 'var(--green)' : 'transparent'}`,
              padding: '14px 0 12px',
              fontFamily: 'var(--font-mono)',
              fontSize: 10,
              letterSpacing: '.18em',
              color: active ? 'var(--green)' : 'var(--label)',
            }}
          >
            {tb.label}
          </button>
        );
      })}
    </nav>
  );
}
