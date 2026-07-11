import { useNavigate } from 'react-router';
import {
  useActivities,
  useAgentRun,
  useProgress,
  useRecovery,
  useToday,
  useUndoToday,
} from '../api/hooks';
import type { Activity, PlanDay, Today } from '../api/types';
import { buildTiles, type TileVM } from '../lib/tiles';
import { AreaChart, MonoLabel, Sparkline, readinessColor } from '../ui/kit';
import { fmtPace, gapless } from '../ui/charts';
import { useDesktop } from '../shell/useMedia';

const TILE_COLORS: Record<TileVM['deltaColor'], string> = {
  green: 'var(--green)',
  amber: 'var(--amber)',
  red: 'var(--red)',
  muted: 'var(--label)',
};

function cap(s: string): string {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}

function sessionTitle(day: PlanDay): string {
  return `${cap(day.run_type)} · ${day.distance_km} km`;
}

function DriverBox({
  k,
  v,
  dir,
  bad,
  sub,
}: {
  k: string;
  v: string;
  dir: '↓' | '↑' | '·';
  bad: boolean;
  sub: string;
}) {
  return (
    <div className="inset" style={{ flex: 1, padding: '9px 11px' }}>
      <MonoLabel style={{ fontSize: 9, letterSpacing: '.16em' }}>{k}</MonoLabel>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, marginTop: 4 }}>
        <span style={{ fontSize: 19, fontWeight: 600 }}>{v}</span>
        <span style={{ fontSize: 13, color: bad ? 'var(--red)' : 'var(--green)' }}>{dir}</span>
      </div>
      <div
        style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--label)', marginTop: 2 }}
      >
        {sub}
      </div>
    </div>
  );
}

function ReadinessCard({ today }: { today: Today }) {
  const color = readinessColor(today.readiness_color);
  const d = today.drivers;
  return (
    <div className="card" style={{ display: 'flex', overflow: 'hidden', padding: 0 }}>
      <div aria-hidden style={{ width: 6, background: color }} />
      <div style={{ padding: 18, flex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
          <span className="pulse-dot" aria-hidden style={{ background: color }} />
          <MonoLabel style={{ letterSpacing: '.24em' }}>READINESS</MonoLabel>
          {today.stale && (
            <span
              style={{
                marginLeft: 'auto',
                fontFamily: 'var(--font-mono)',
                fontSize: 10,
                color: 'var(--amber)',
              }}
            >
              SYNC STALE
            </span>
          )}
        </div>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 12, marginTop: 8 }}>
          <span
            style={{
              fontSize: 44,
              fontWeight: 600,
              letterSpacing: '.01em',
              lineHeight: 0.9,
              color,
            }}
          >
            {today.readiness_color.toUpperCase()}
          </span>
        </div>
        <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
          <DriverBox
            k="HRV"
            v={d.hrv_last_night_ms != null ? String(d.hrv_last_night_ms) : '—'}
            dir={d.hrv_delta_pct == null ? '·' : d.hrv_delta_pct < 0 ? '↓' : '↑'}
            bad={(d.hrv_delta_pct ?? 0) < 0}
            sub={d.hrv_baseline_ms != null ? `7-day ${Math.round(d.hrv_baseline_ms)}` : ' '}
          />
          <DriverBox
            k="RESTING HR"
            v={d.rhr_last_night != null ? String(d.rhr_last_night) : '—'}
            dir={d.rhr_delta_bpm == null ? '·' : d.rhr_delta_bpm > 0 ? '↑' : '↓'}
            bad={(d.rhr_delta_bpm ?? 0) > 0}
            sub={d.rhr_baseline != null ? `base ${Math.round(d.rhr_baseline)}` : ' '}
          />
        </div>
      </div>
    </div>
  );
}

function SessionCard({ today }: { today: Today }) {
  const nav = useNavigate();
  const undo = useUndoToday();
  const eff = today.effective_session;
  const orig = today.original_session;
  const changed =
    (today.action === 'SOFTEN' || today.action === 'MOVE') && orig !== null && eff !== null;

  return (
    <div className="card" style={{ padding: '16px 18px' }}>
      <MonoLabel>{'// TODAY’S SESSION'}</MonoLabel>
      {eff ? (
        <>
          <div style={{ fontSize: 26, fontWeight: 600, margin: '8px 0 2px' }}>
            {sessionTitle(eff)}
          </div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 13, color: 'var(--muted)' }}>
            {eff.pace_target}
            {eff.time_note ? ` · ${eff.time_note}` : ''}
          </div>
        </>
      ) : (
        <>
          <div style={{ fontSize: 26, fontWeight: 600, margin: '8px 0 2px' }}>Rest day</div>
          <div style={{ fontSize: 14, color: 'var(--muted)', lineHeight: 1.5 }}>
            {today.rationale}
          </div>
        </>
      )}
      {changed && (
        <div
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 6,
            marginTop: 12,
            background: 'var(--amber-tint)',
            border: '1px solid var(--amber-border)',
            borderRadius: 8,
            padding: '5px 10px',
          }}
        >
          <span aria-hidden style={{ color: 'var(--amber)', fontSize: 12 }}>
            ↓
          </span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--amber)' }}>
            {today.action === 'SOFTEN' ? 'softened' : 'moved'} from {orig!.run_type} ·{' '}
            {orig!.distance_km} km
          </span>
        </div>
      )}
      <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
        <button className="btn-primary" style={{ flex: 1 }} onClick={() => nav('/plan')}>
          Details
        </button>
        <button className="btn-secondary" onClick={() => nav('/coach')}>
          Ask coach
        </button>
      </div>
      {changed && (
        <button
          onClick={() => undo.mutate()}
          disabled={undo.isPending}
          style={{
            marginTop: 12,
            background: 'none',
            border: 'none',
            padding: 0,
            fontFamily: 'var(--font-mono)',
            fontSize: 11,
            color: 'var(--faint)',
            textDecoration: 'underline',
          }}
        >
          {undo.isPending ? 'undoing…' : 'undo — keep the original session'}
        </button>
      )}
    </div>
  );
}

function ReshapeBanner({ today }: { today: Today }) {
  const nav = useNavigate();
  if (today.action === 'STAND' || !today.rationale) return null;
  return (
    <button
      className="btn-ghost-green"
      onClick={() => nav('/coach', { state: { prefill: today.rationale } })}
      style={{
        width: '100%',
        textAlign: 'left',
        padding: '12px 14px',
        display: 'flex',
        alignItems: 'center',
        gap: 10,
      }}
    >
      <span
        aria-hidden
        style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--green)', flex: 'none' }}
      />
      <span style={{ fontSize: 13, color: 'var(--text-2)' }}>Coach reshaped your day.</span>
      <span
        style={{
          marginLeft: 'auto',
          fontFamily: 'var(--font-mono)',
          fontSize: 11,
          color: 'var(--green)',
        }}
      >
        why ›
      </span>
    </button>
  );
}

function SignalTile({ tile, showSpark }: { tile: TileVM; showSpark: boolean }) {
  const deltaColor = TILE_COLORS[tile.deltaColor];
  return (
    <div className="tile">
      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
        <MonoLabel style={{ fontSize: 9, letterSpacing: '.14em' }}>{tile.label}</MonoLabel>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: deltaColor }}>
          {tile.delta}
        </span>
      </div>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 5, margin: '5px 0' }}>
        <span style={{ fontSize: 20, fontWeight: 600 }}>{tile.value}</span>
        <span style={{ fontSize: 10, color: 'var(--label)' }}>{tile.unit}</span>
      </div>
      {showSpark && tile.spark.length >= 2 && (
        <Sparkline
          values={tile.spark}
          color={tile.deltaColor === 'red' ? 'var(--red)' : 'var(--green)'}
        />
      )}
      <div
        style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--faint)', marginTop: 5 }}
      >
        {tile.sub}
      </div>
    </div>
  );
}

function LastSession({ activities }: { activities: Activity[] }) {
  const nav = useNavigate();
  const last = activities.find((a) => a.distance_m > 0);
  if (!last) return null;
  const dow = new Date(last.start_time)
    .toLocaleDateString('en-US', { weekday: 'short' })
    .toUpperCase();
  const pace = last.avg_speed && last.avg_speed > 0 ? `${fmtPace(1000 / last.avg_speed)}/km` : '';
  return (
    <button
      onClick={() => nav(`/runs/${last.activity_id}`)}
      className="card--subtle"
      style={{
        width: '100%',
        textAlign: 'left',
        padding: '13px 15px',
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        borderRadius: 14,
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <MonoLabel style={{ fontSize: 9, letterSpacing: '.16em' }}>LAST SESSION · {dow}</MonoLabel>
        <div style={{ fontSize: 14, fontWeight: 500, marginTop: 3, color: 'var(--text)' }}>
          {last.name} · {(last.distance_m / 1000).toFixed(1)} km {pace && `· ${pace}`}
        </div>
      </div>
      <span aria-hidden style={{ color: 'var(--label)', fontSize: 18 }}>
        ›
      </span>
    </button>
  );
}

function NoDecision() {
  const agentRun = useAgentRun();
  return (
    <div className="card" style={{ padding: 20 }}>
      <MonoLabel>{'// TODAY'}</MonoLabel>
      <div style={{ fontSize: 20, fontWeight: 600, margin: '10px 0 6px' }}>
        No decision yet today.
      </div>
      <div style={{ fontSize: 14, color: 'var(--muted)', lineHeight: 1.55 }}>
        The coach runs every morning on its own. Run it now to get today’s readiness and session.
      </div>
      {agentRun.error && (
        <div className="error-line" style={{ marginTop: 10 }}>
          {(agentRun.error as Error).message}
        </div>
      )}
      <button
        className="btn-primary"
        style={{ marginTop: 16 }}
        disabled={agentRun.isPending}
        onClick={() => agentRun.mutate({ force: true })}
      >
        {agentRun.isPending ? 'Running coach…' : 'Run coach now'}
      </button>
    </div>
  );
}

export function TodayPage() {
  const desktop = useDesktop();
  const { data: today, isLoading } = useToday();
  const { data: recovery } = useRecovery(30);
  const { data: activities } = useActivities();
  const { data: progress } = useProgress(12);

  const vo2 = progress?.signals.find((s) => s.key === 'vo2max')?.series ?? [];
  const tiles = buildTiles(recovery ?? [], activities ?? [], vo2, new Date());
  const paceSignal = progress?.signals.find((s) => s.key === 'pace_at_hr');
  const paceVals = paceSignal ? gapless(paceSignal.series) : [];

  const dateLabel = new Date()
    .toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
    .toUpperCase()
    .replace(',', ' ·');

  if (isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 60 }}>
        <div className="spinner" role="status" aria-label="loading" />
      </div>
    );
  }

  const left = (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {today ? (
        <>
          <ReadinessCard today={today} />
          <ReshapeBanner today={today} />
          <SessionCard today={today} />
        </>
      ) : (
        <NoDecision />
      )}
      <LastSession activities={activities ?? []} />
    </div>
  );

  const signals = (
    <div>
      <MonoLabel style={{ margin: '16px 0 8px' }}>{'// SIGNALS'}</MonoLabel>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
        {tiles.map((t) => (
          <SignalTile key={t.label} tile={t} showSpark={desktop} />
        ))}
      </div>
    </div>
  );

  if (desktop) {
    return (
      <div
        style={{ display: 'grid', gridTemplateColumns: '1.05fr 1fr', gap: 16, alignItems: 'start' }}
      >
        {left}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {signals}
          {paceVals.length >= 2 && (
            <div className="card" style={{ padding: 16 }}>
              <div
                style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}
              >
                <MonoLabel green style={{ fontSize: 9, letterSpacing: '.14em' }}>
                  AEROBIC ENGINE ↗
                </MonoLabel>
                {paceSignal?.delta_abs != null && (
                  <span
                    style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--green)' }}
                  >
                    {paceSignal.delta_abs <= 0 ? '▼' : '▲'}{' '}
                    {Math.abs(Math.round(paceSignal.delta_abs))}s/km
                  </span>
                )}
              </div>
              <div style={{ fontSize: 13, color: 'var(--muted)', margin: '6px 0 2px' }}>
                Easy pace @ Z2 HR · {progress?.weeks ?? 12} wks
              </div>
              <AreaChart values={paceVals} width={286} height={72} pad={8} />
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  fontFamily: 'var(--font-mono)',
                  fontSize: 10,
                  color: 'var(--faint)',
                  marginTop: 4,
                }}
              >
                <span>{fmtPace(paceVals[0])}</span>
                <span>{fmtPace(paceVals[paceVals.length - 1])}</span>
              </div>
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="scroll-pane" style={{ flex: 1, padding: '2px 18px 20px' }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 0 14px',
        }}
      >
        <span
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            letterSpacing: '.24em',
            color: 'var(--green)',
          }}
        >
          HELP MY RUN
        </span>
        <span
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            letterSpacing: '.18em',
            color: 'var(--label)',
          }}
        >
          {dateLabel}
        </span>
      </div>
      {left}
      {signals}
    </div>
  );
}
