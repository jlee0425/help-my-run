import { useRef, useState } from 'react';
import { useCrossfitParse, usePlan, usePlanGenerate } from '../api/hooks';
import type { CrossFitWeek, PlanDay } from '../api/types';
import { MonoLabel, Pill } from '../ui/kit';

/** Monday (YYYY-MM-DD) of the ISO week containing d. */
export function mondayOf(d: Date): string {
  const copy = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()));
  const off = (copy.getUTCDay() + 6) % 7;
  copy.setUTCDate(copy.getUTCDate() - off);
  return copy.toISOString().slice(0, 10);
}

export function shiftWeek(weekStart: string, deltaWeeks: number): string {
  const d = new Date(`${weekStart}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + deltaWeeks * 7);
  return d.toISOString().slice(0, 10);
}

function cap(s: string): string {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}

function DayRow({ day, today }: { day: PlanDay; today: string }) {
  const isToday = day.date === today;
  const rest = day.run_type === 'rest';
  return (
    <div
      className="card"
      style={{
        borderRadius: 14,
        padding: '13px 15px',
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        borderColor: isToday ? 'var(--green-border)' : undefined,
        opacity: rest ? 0.55 : 1,
      }}
    >
      <span
        style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 10,
          letterSpacing: '.12em',
          color: isToday ? 'var(--green)' : 'var(--label)',
          width: 34,
        }}
      >
        {day.dow.toUpperCase()}
      </span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 15, fontWeight: 500 }}>
          {rest ? 'Rest' : `${cap(day.run_type)} · ${day.distance_km} km`}
        </div>
        {!rest && (
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--muted)', marginTop: 2 }}>
            {day.pace_target}
            {day.time_note ? ` · ${day.time_note}` : ''}
          </div>
        )}
      </div>
      {day.optional_if_cns && <Pill color="amber">optional</Pill>}
    </div>
  );
}

function IngestCard({ weekStart }: { weekStart: string }) {
  const parse = useCrossfitParse();
  const generate = usePlanGenerate();
  const fileRef = useRef<HTMLInputElement>(null);
  const [parsed, setParsed] = useState<CrossFitWeek | null>(null);

  const busy = parse.isPending || generate.isPending;
  const err = (parse.error ?? generate.error) as Error | null;

  return (
    <div className="card" style={{ padding: 18 }}>
      <MonoLabel>{'// CROSSFIT WEEK'}</MonoLabel>
      <div style={{ fontSize: 14, color: 'var(--muted)', lineHeight: 1.55, margin: '8px 0 0' }}>
        Upload this week’s box schedule photo — the coach reads it and periodises your runs around
        the heavy days.
      </div>

      {parsed && (
        <div style={{ marginTop: 14, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {parsed.days.map((d) => (
            <span
              key={d.date}
              className="chip"
              style={{
                color: d.has_crossfit ? 'var(--text-2)' : 'var(--faint)',
                borderColor: d.has_crossfit ? 'var(--green-border)' : undefined,
              }}
            >
              {d.dow.slice(0, 3).toUpperCase()} {d.has_crossfit ? `· ${d.focus || 'WOD'}` : '· —'}
            </span>
          ))}
        </div>
      )}

      {err && (
        <div className="error-line" style={{ marginTop: 10 }}>
          {err.message}
        </div>
      )}

      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        data-testid="cf-file"
        style={{ display: 'none' }}
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) {
            setParsed(null);
            parse.mutate(
              { weekStart, image: f },
              { onSuccess: (week) => setParsed(week) },
            );
          }
          e.target.value = '';
        }}
      />

      <div style={{ display: 'flex', gap: 8, marginTop: 16, alignItems: 'center' }}>
        <button
          className="btn-secondary"
          disabled={busy}
          onClick={() => fileRef.current?.click()}
        >
          {parse.isPending ? 'Reading schedule…' : parsed ? 'Re-upload photo' : 'Upload schedule photo'}
        </button>
        <button
          className="btn-primary"
          disabled={busy || !parsed}
          onClick={() => parsed && generate.mutate({ weekStart, crossfitWeek: parsed })}
        >
          {generate.isPending ? 'Generating…' : 'Generate plan'}
        </button>
        {busy && <div className="spinner" style={{ width: 22, height: 22 }} aria-label="working" />}
      </div>
    </div>
  );
}

export function PlanPage() {
  const [weekStart, setWeekStart] = useState(() => mondayOf(new Date()));
  const { data: plan, isLoading } = usePlan(weekStart);
  const today = new Date().toISOString().slice(0, 10);

  return (
    <div className="scroll-pane" style={{ flex: 1, padding: '2px 18px 20px', maxWidth: 720 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 0 14px',
        }}
      >
        <button
          aria-label="Previous week"
          className="btn-secondary"
          style={{ width: 34, height: 34, padding: 0, borderRadius: 10 }}
          onClick={() => setWeekStart((w) => shiftWeek(w, -1))}
        >
          ‹
        </button>
        <div style={{ textAlign: 'center' }}>
          <MonoLabel style={{ letterSpacing: '.2em' }}>{'// WEEK OF'}</MonoLabel>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 14, marginTop: 2 }}>{weekStart}</div>
        </div>
        <button
          aria-label="Next week"
          className="btn-secondary"
          style={{ width: 34, height: 34, padding: 0, borderRadius: 10 }}
          onClick={() => setWeekStart((w) => shiftWeek(w, 1))}
        >
          ›
        </button>
      </div>

      {isLoading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
          <div className="spinner" role="status" aria-label="loading" />
        </div>
      ) : plan ? (
        <>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {plan.days.map((d) => (
              <DayRow key={d.date} day={d} today={today} />
            ))}
          </div>
          {plan.week_rationale && (
            <div className="card--subtle" style={{ marginTop: 12, padding: 15 }}>
              <MonoLabel style={{ fontSize: 9, letterSpacing: '.16em' }}>WEEK RATIONALE</MonoLabel>
              <div style={{ fontSize: 13.5, color: 'var(--text-2)', lineHeight: 1.55, marginTop: 6 }}>
                {plan.week_rationale}
              </div>
              <div
                style={{
                  fontFamily: 'var(--font-mono)',
                  fontSize: 10,
                  color: 'var(--faint)',
                  marginTop: 8,
                }}
              >
                TARGET {plan.weekly_target_km} KM{plan.one_flag ? ` · ${plan.one_flag}` : ''}
              </div>
            </div>
          )}
          <div style={{ marginTop: 12 }}>
            <IngestCard weekStart={weekStart} />
          </div>
        </>
      ) : (
        <>
          <div className="card--subtle" style={{ padding: 16, marginBottom: 12 }}>
            <div style={{ fontSize: 14, color: 'var(--muted)', lineHeight: 1.5 }}>
              No plan for this week yet. Upload the CrossFit schedule below and generate one.
            </div>
          </div>
          <IngestCard weekStart={weekStart} />
        </>
      )}
    </div>
  );
}
