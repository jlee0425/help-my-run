import { useActivities, useProgress, useRecovery } from '../api/hooks';
import { acRatio } from '../lib/activity';
import {
  deltaChips,
  loadSplit,
  miniSeries,
  paceSeries,
  runSharePct,
  sleepPaceLink,
} from '../lib/trends';
import { AreaChart, MonoLabel, Sparkline, StackedBars } from '../ui/kit';
import { useDesktop } from '../shell/useMedia';

function Hero({ report }: { report: NonNullable<ReturnType<typeof usePaceVM>> }) {
  const desktop = useDesktop();
  return (
    <div className="card" style={{ padding: desktop ? 22 : 18 }}>
      <MonoLabel green style={{ letterSpacing: '.16em' }}>
        AEROBIC ENGINE ↗
      </MonoLabel>
      <div style={{ fontSize: desktop ? 22 : 19, fontWeight: 600, margin: '8px 0 2px', lineHeight: 1.25 }}>
        Same heart rate, faster running.
      </div>
      <div style={{ fontSize: 13, color: 'var(--muted)', lineHeight: 1.5, maxWidth: 440 }}>
        Easy pace at your Zone-2 heart rate — the clearest read on your base carrying your CrossFit
        work.
      </div>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 12, margin: '16px 0 4px' }}>
        <span style={{ fontSize: desktop ? 40 : 34, fontWeight: 600, fontFamily: 'var(--font-mono)' }}>
          {report.now}
        </span>
        <span style={{ fontSize: 13, color: 'var(--label)' }}>/km @ Z2 HR</span>
        <span
          style={{
            marginLeft: 'auto',
            fontFamily: 'var(--font-mono)',
            fontSize: 13,
            color: 'var(--green)',
          }}
        >
          {report.deltaLabel}
        </span>
      </div>
      <AreaChart values={report.values} width={desktop ? 620 : 286} height={desktop ? 150 : 96} pad={10} />
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
        <span>12 wks ago · {report.then}</span>
        <span>now · {report.now}</span>
      </div>
    </div>
  );
}

function usePaceVM() {
  const { data: progress } = useProgress(12);
  return progress ? paceSeries(progress) : null;
}

export function TrendsPage() {
  const desktop = useDesktop();
  const { data: progress, isLoading } = useProgress(12);
  const { data: activities } = useActivities();
  const { data: recovery } = useRecovery(90);

  if (isLoading || !progress) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 60 }}>
        <div className="spinner" role="status" aria-label="loading" />
      </div>
    );
  }

  const pace = paceSeries(progress);
  const chips = deltaChips(progress);
  const hrv = miniSeries(progress, 'hrv_baseline');
  const rhr = miniSeries(progress, 'resting_hr');
  const split = loadSplit(activities ?? [], 6, new Date());
  const share = runSharePct(split);
  const ac = acRatio(activities ?? [], new Date());
  const link = sleepPaceLink(recovery ?? [], activities ?? []);

  if (!progress.enough_data && !pace) {
    return (
      <div style={{ padding: '18px' }}>
        <div className="card" style={{ padding: 20, maxWidth: 520 }}>
          <MonoLabel>{'// TRENDS'}</MonoLabel>
          <div style={{ fontSize: 20, fontWeight: 600, margin: '10px 0 6px' }}>
            Not enough data yet.
          </div>
          <div style={{ fontSize: 14, color: 'var(--muted)', lineHeight: 1.55 }}>
            Keep syncing — trends unlock once a couple of weeks of runs and recovery data are in.
          </div>
        </div>
      </div>
    );
  }

  const rightColumn = (
    <div style={{ display: 'flex', flexDirection: 'column', gap: desktop ? 12 : 10 }}>
      {chips.length > 0 && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: desktop ? 10 : 8 }}>
          {chips.map((c) => (
            <div key={c.label} className="card--subtle" style={{ padding: '11px 8px', textAlign: 'center' }}>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 8, letterSpacing: '.12em', color: 'var(--label)' }}>
                {c.label}
              </div>
              <div style={{ fontSize: 16, fontWeight: 600, color: c.color, marginTop: 5 }}>{c.value}</div>
            </div>
          ))}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: desktop ? 10 : 8 }}>
        {hrv && (
          <div className="card" style={{ borderRadius: 14, padding: 13 }}>
            <MonoLabel style={{ fontSize: 9, letterSpacing: '.12em' }}>HRV BASELINE</MonoLabel>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 5, marginTop: 5 }}>
              <span style={{ fontSize: 22, fontWeight: 600 }}>{hrv.current}</span>
              <span style={{ fontSize: 11, color: hrv.good ? 'var(--green)' : 'var(--red)' }}>{hrv.delta}</span>
            </div>
            <div style={{ marginTop: 6 }}>
              <Sparkline values={hrv.values} color="var(--green)" width={120} height={30} />
            </div>
          </div>
        )}
        {rhr && (
          <div className="card" style={{ borderRadius: 14, padding: 13 }}>
            <MonoLabel style={{ fontSize: 9, letterSpacing: '.12em' }}>RESTING HR</MonoLabel>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 5, marginTop: 5 }}>
              <span style={{ fontSize: 22, fontWeight: 600 }}>{rhr.current}</span>
              <span style={{ fontSize: 11, color: rhr.good ? 'var(--green)' : 'var(--red)' }}>{rhr.delta}</span>
            </div>
            <div style={{ marginTop: 6 }}>
              <Sparkline values={rhr.values} color="var(--green)" width={120} height={30} />
            </div>
          </div>
        )}
      </div>

      {link && (
        <div
          style={{
            background: 'linear-gradient(135deg, rgba(232,178,76,.1), rgba(232,178,76,.02))',
            border: '1px solid rgba(232,178,76,.25)',
            borderRadius: 14,
            padding: '14px 15px',
          }}
        >
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: '.16em', color: 'var(--amber)' }}>
            SLEEP → PACE LINK
          </div>
          <div style={{ fontSize: 14, color: '#D6DDE3', lineHeight: 1.5, marginTop: 6 }}>
            On nights under 6h, your next-day easy pace ran{' '}
            <b style={{ color: 'var(--amber)' }}>{link.slowerSecPerKm} s/km slower</b> at the same
            heart rate. A link no single app can see.
          </div>
        </div>
      )}

      {split.some((w) => w.run + w.cf > 0) && (
        <div className="card" style={{ borderRadius: 16, padding: 16 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <MonoLabel style={{ fontSize: 9, letterSpacing: '.14em' }}>
              WEEKLY LOAD · RUN × CROSSFIT
            </MonoLabel>
            {ac !== null && (
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--green)' }}>
                a:c {ac.toFixed(2)}
              </span>
            )}
          </div>
          <div style={{ marginTop: 14 }}>
            <StackedBars weeks={split} />
          </div>
          <div style={{ display: 'flex', gap: 16, marginTop: 12 }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--muted)' }}>
              <span aria-hidden style={{ width: 9, height: 9, borderRadius: 2, background: 'var(--green)' }} />
              Running {share !== null ? `~${share}%` : ''}
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--muted)' }}>
              <span aria-hidden style={{ width: 9, height: 9, borderRadius: 2, background: 'var(--cf-bar)' }} />
              CrossFit
            </span>
          </div>
        </div>
      )}
    </div>
  );

  if (desktop) {
    return (
      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: 16, alignItems: 'start' }}>
        <div>{pace ? <Hero report={pace} /> : null}</div>
        {rightColumn}
      </div>
    );
  }

  return (
    <div className="scroll-pane" style={{ flex: 1, padding: '2px 18px 20px' }}>
      <div style={{ padding: '6px 0 12px' }}>
        <MonoLabel>{'// TRENDS · 12 WEEKS OF DATA'}</MonoLabel>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {pace && <Hero report={pace} />}
        {rightColumn}
      </div>
    </div>
  );
}
