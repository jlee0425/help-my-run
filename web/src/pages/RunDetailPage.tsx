import { useNavigate, useParams } from 'react-router';
import { useActivities, useAnalysis, useFetchStream } from '../api/hooks';
import type { Activity, StreamAnalysis } from '../api/types';
import { fmtDuration, fmtPace } from '../ui/charts';
import { MonoLabel, Pill, ZoneBand } from '../ui/kit';

/** "✓ on target" only when decoupling is computed and under 8%. */
export function onTarget(analysis: StreamAnalysis | undefined): boolean {
  return !!analysis && analysis.decoupling_pct !== null && analysis.decoupling_pct < 8;
}

function StatTile({ k, v, u }: { k: string; v: string; u: string }) {
  return (
    <div className="card" style={{ borderRadius: 13, padding: '12px 13px' }}>
      <MonoLabel style={{ fontSize: 9, letterSpacing: '.12em' }}>{k}</MonoLabel>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 4, marginTop: 5 }}>
        <span style={{ fontSize: 21, fontWeight: 600 }}>{v}</span>
        <span style={{ fontSize: 10, color: 'var(--label)' }}>{u}</span>
      </div>
    </div>
  );
}

export function RunDetailPage() {
  const { id } = useParams<{ id: string }>();
  const activityId = id ? Number(id) : undefined;
  const nav = useNavigate();
  const { data: activities } = useActivities();
  const { data: analysis } = useAnalysis(activityId);
  const fetchStream = useFetchStream();

  const activity: Activity | undefined = activities?.find((a) => a.activity_id === activityId);

  const dateLabel = activity
    ? new Date(activity.start_time)
        .toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
        .toUpperCase()
        .replace(',', ' ·')
    : '';
  const pace =
    activity?.avg_speed && activity.avg_speed > 0 ? fmtPace(1000 / activity.avg_speed) : '—';

  return (
    <div className="scroll-pane" style={{ flex: 1, padding: '2px 18px 20px', maxWidth: 720 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '6px 0 14px' }}>
        <button
          aria-label="Back"
          onClick={() => nav(-1)}
          className="btn-secondary"
          style={{ width: 34, height: 34, padding: 0, borderRadius: 10, fontSize: 16 }}
        >
          ‹
        </button>
        <div style={{ minWidth: 0 }}>
          <MonoLabel style={{ fontSize: 9, letterSpacing: '.16em' }}>{dateLabel}</MonoLabel>
          <div style={{ fontSize: 17, fontWeight: 600 }}>{activity?.name ?? `Run ${id}`}</div>
        </div>
        {onTarget(analysis) && (
          <span style={{ marginLeft: 'auto' }}>
            <Pill color="green">✓ on target</Pill>
          </span>
        )}
      </div>

      {activity && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
          <StatTile k="DISTANCE" v={(activity.distance_m / 1000).toFixed(1)} u="km" />
          <StatTile k="TIME" v={fmtDuration(activity.moving_time_s)} u="" />
          <StatTile k="AVG PACE" v={pace} u="/km" />
          <StatTile
            k="AVG HR"
            v={activity.avg_hr != null ? String(Math.round(activity.avg_hr)) : '—'}
            u="bpm"
          />
        </div>
      )}

      {analysis?.has_stream && analysis.has_hr && analysis.time_in_zone.length > 0 && (
        <>
          <MonoLabel style={{ margin: '18px 0 8px', letterSpacing: '.2em' }}>
            {'// HR ZONES'}
          </MonoLabel>
          <ZoneBand zones={analysis.time_in_zone.map((z) => ({ zone: z.zone, pct: z.pct }))} />
          {analysis.decoupling_pct !== null && (
            <div
              style={{
                marginTop: 14,
                fontFamily: 'var(--font-mono)',
                fontSize: 11,
                color: 'var(--muted)',
                lineHeight: 1.7,
              }}
            >
              PA:HR DRIFT {analysis.decoupling_pct.toFixed(1)}%
              {analysis.pa_hr_first !== null && analysis.pa_hr_second !== null && (
                <>
                  {' '}
                  · first {analysis.pa_hr_first.toFixed(2)} / second{' '}
                  {analysis.pa_hr_second.toFixed(2)}
                </>
              )}
            </div>
          )}
        </>
      )}

      {analysis && !analysis.has_stream && (
        <div className="card--subtle" style={{ marginTop: 16, padding: 16 }}>
          <div style={{ fontSize: 14, color: 'var(--muted)', lineHeight: 1.5 }}>
            No stream data fetched for this run yet.
          </div>
          {fetchStream.error && (
            <div className="error-line" style={{ marginTop: 8 }}>
              {(fetchStream.error as Error).message}
            </div>
          )}
          <button
            className="btn-secondary"
            style={{ marginTop: 12 }}
            disabled={fetchStream.isPending || activityId === undefined}
            onClick={() => activityId !== undefined && fetchStream.mutate(activityId)}
          >
            {fetchStream.isPending ? 'Fetching…' : 'Fetch stream'}
          </button>
        </div>
      )}

      <button
        className="btn-ghost-green"
        onClick={() =>
          nav('/coach', {
            state: {
              prefill: activity
                ? `Looking at: ${activity.name} on ${activity.start_time.slice(0, 10)}.`
                : undefined,
            },
          })
        }
        style={{
          width: '100%',
          marginTop: 20,
          padding: 14,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 8,
          color: 'var(--green)',
          fontFamily: 'var(--font-mono)',
          fontSize: 12,
          letterSpacing: '.08em',
        }}
      >
        ASK COACH ABOUT THIS RUN ›
      </button>
    </div>
  );
}
