import type { ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { useDesktop } from './useMedia';
import { Rail } from './Rail';
import { TabBar } from './TabBar';
import { useToday } from '../api/today';

const CRUMBS: Record<string, string> = {
  '/': 'HOME',
  '/trends': 'ANALYSIS · 12 WEEKS',
  '/coach': 'ASK YOUR DATA',
  '/plan': 'WEEKLY PLAN',
  '/settings': 'INSTANCE',
};

const TITLES: Record<string, string> = {
  '/': 'Today',
  '/trends': 'Trends',
  '/coach': 'Coach',
  '/plan': 'Plan',
  '/settings': 'Settings',
};

const READINESS_STYLE: Record<string, { color: string; band: string; border: string }> = {
  green: {
    color: 'var(--green)',
    band: 'linear-gradient(135deg, rgba(95,208,139,.14), rgba(95,208,139,.03))',
    border: 'var(--green-border)',
  },
  amber: {
    color: 'var(--amber)',
    band: 'linear-gradient(135deg, rgba(232,178,76,.14), rgba(232,178,76,.03))',
    border: 'rgba(232,178,76,.35)',
  },
  red: {
    color: 'var(--red)',
    band: 'linear-gradient(135deg, rgba(232,104,92,.14), rgba(232,104,92,.03))',
    border: 'var(--red-border)',
  },
};

/** Responsive app shell: mobile bottom tabs / desktop left rail + header. */
export function Shell({ children }: { children: ReactNode }) {
  const desktop = useDesktop();
  const { pathname } = useLocation();
  const nav = useNavigate();
  const { data: today } = useToday();

  if (!desktop) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', minHeight: '100dvh' }}>
        <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
          {children}
        </div>
        <TabBar />
      </div>
    );
  }

  const crumb = CRUMBS[pathname] ?? '';
  const title = TITLES[pathname] ?? '';
  const rd = today ? READINESS_STYLE[today.readiness_color] : undefined;

  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <Rail />
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
        <div
          style={{
            flex: 'none',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '20px 28px 14px',
            borderBottom: '1px solid #131922',
          }}
        >
          <div>
            <div className="mono-label">{crumb}</div>
            <div style={{ fontSize: 22, fontWeight: 600, marginTop: 3 }}>{title}</div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            {rd && today && (
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  background: rd.band,
                  border: `1px solid ${rd.border}`,
                  borderRadius: 11,
                  padding: '8px 14px',
                }}
              >
                <span
                  aria-hidden
                  style={{ width: 8, height: 8, borderRadius: '50%', background: rd.color }}
                />
                <span
                  style={{
                    fontFamily: 'var(--font-mono)',
                    fontSize: 11,
                    letterSpacing: '.14em',
                    color: rd.color,
                  }}
                >
                  {today.readiness_color.toUpperCase()}
                </span>
              </div>
            )}
            <button
              aria-label="Settings"
              onClick={() => nav('/settings')}
              className="btn-secondary"
              style={{ padding: '8px 12px', borderRadius: 10, fontSize: 14 }}
            >
              ⚙
            </button>
          </div>
        </div>
        <div className="scroll-pane" style={{ flex: 1, minHeight: 0, padding: '22px 28px 28px' }}>
          {children}
        </div>
      </div>
    </div>
  );
}
