// Small design-system components (values from the design files). One file —
// each piece is a handful of lines; splitting would scatter the system.
import type { CSSProperties, ReactNode } from 'react';
import { areaChart, sparkPoints } from './charts';

export function MonoLabel({
  children,
  green,
  style,
}: {
  children: ReactNode;
  green?: boolean;
  style?: CSSProperties;
}) {
  return (
    <div className={`mono-label${green ? ' mono-label--green' : ''}`} style={style}>
      {children}
    </div>
  );
}

export function Card({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  return (
    <div className="card" style={{ padding: 18, ...style }}>
      {children}
    </div>
  );
}

const READINESS_COLORS: Record<string, string> = {
  green: 'var(--green)',
  amber: 'var(--amber)',
  red: 'var(--red)',
};

export function readinessColor(word: string): string {
  return READINESS_COLORS[word] ?? 'var(--muted)';
}

/** Small status pill (readiness / on-target). */
export function Pill({
  color,
  children,
}: {
  color: 'green' | 'amber' | 'red' | 'muted';
  children: ReactNode;
}) {
  const c =
    color === 'muted' ? 'var(--muted)' : READINESS_COLORS[color] ?? 'var(--muted)';
  const tint =
    color === 'green'
      ? 'var(--green-tint-2)'
      : color === 'amber'
        ? 'var(--amber-tint)'
        : color === 'red'
          ? 'var(--red-tint)'
          : 'var(--inset)';
  return (
    <span
      style={{
        fontFamily: 'var(--font-mono)',
        fontSize: 11,
        color: c,
        background: tint,
        borderRadius: 20,
        padding: '4px 11px',
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </span>
  );
}

/** Design toggle: 42×24 track, 20px knob, green when on. */
export function Toggle({
  on,
  onChange,
  label,
  disabled = false,
}: {
  on: boolean;
  onChange: (next: boolean) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <button
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => !disabled && onChange(!on)}
      style={{
        flex: 'none',
        width: 42,
        height: 24,
        borderRadius: 14,
        border: 'none',
        background: on ? 'var(--green)' : '#2a323d',
        position: 'relative',
        transition: 'background .2s',
        padding: 0,
        opacity: disabled ? 0.5 : 1,
        cursor: disabled ? 'not-allowed' : 'pointer',
      }}
    >
      <span
        aria-hidden
        style={{
          position: 'absolute',
          top: 2,
          left: on ? 20 : 2,
          width: 20,
          height: 20,
          borderRadius: '50%',
          background: '#fff',
          transition: 'left .2s',
        }}
      />
    </button>
  );
}

/** Design stepper row: label, −/+ inset buttons, mono value. */
export function Stepper({
  label,
  value,
  onInc,
  onDec,
}: {
  label: string;
  value: number;
  onInc: () => void;
  onDec: () => void;
}) {
  return (
    <div
      className="card"
      style={{
        borderRadius: 14,
        padding: '14px 16px',
        display: 'flex',
        alignItems: 'center',
        gap: 12,
      }}
    >
      <span style={{ flex: 1, fontSize: 15 }}>{label}</span>
      <button
        aria-label={`decrease ${label}`}
        onClick={onDec}
        className="inset"
        style={{ width: 32, height: 32, borderRadius: 9, color: 'var(--text-2)', fontSize: 18 }}
      >
        −
      </button>
      <span
        style={{
          width: 26,
          textAlign: 'center',
          fontSize: 19,
          fontWeight: 600,
          fontFamily: 'var(--font-mono)',
        }}
      >
        {value}
      </span>
      <button
        aria-label={`increase ${label}`}
        onClick={onInc}
        className="inset"
        style={{ width: 32, height: 32, borderRadius: 9, color: 'var(--text-2)', fontSize: 18 }}
      >
        +
      </button>
    </div>
  );
}

// --- charts -----------------------------------------------------------------

export function Sparkline({
  values,
  color,
  width = 110,
  height = 26,
}: {
  values: number[];
  color: string;
  width?: number;
  height?: number;
}) {
  if (values.length < 2) return null;
  return (
    <svg
      role="img"
      aria-label="trend sparkline"
      width="100%"
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      style={{ display: 'block' }}
    >
      <polyline
        points={sparkPoints(values, width, height)}
        fill="none"
        stroke={color}
        strokeWidth={1.6}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

let gradientSeq = 0;

export function AreaChart({
  values,
  width,
  height,
  pad = 10,
}: {
  values: number[];
  width: number;
  height: number;
  pad?: number;
}) {
  if (values.length < 2) return null;
  const { line, area } = areaChart(values, width, height, pad);
  const id = `ac-${++gradientSeq}`;
  return (
    <svg
      role="img"
      aria-label="trend chart"
      width="100%"
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      style={{ display: 'block' }}
    >
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#5FD08B" stopOpacity="0.26" />
          <stop offset="1" stopColor="#5FD08B" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${id})`} />
      <polyline
        points={line}
        fill="none"
        stroke="var(--green)"
        strokeWidth={2.2}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

/** Weekly run×CrossFit stacked bars (green bottom, slate top). */
export function StackedBars({
  weeks,
  height = 66,
}: {
  weeks: { label: string; run: number; cf: number }[];
  height?: number;
}) {
  const maxTot = Math.max(...weeks.map((w) => w.run + w.cf), 1);
  return (
    <div style={{ display: 'flex', alignItems: 'flex-end', gap: 10, height: height + 14 }}>
      {weeks.map((w) => (
        <div
          key={w.label}
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'flex-end',
            alignItems: 'center',
            gap: 3,
          }}
        >
          <div
            data-testid={`cf-${w.label}`}
            style={{
              width: '100%',
              borderRadius: '3px 3px 0 0',
              background: 'var(--cf-bar)',
              height: `${Math.round((w.cf / maxTot) * height)}px`,
            }}
          />
          <div
            data-testid={`run-${w.label}`}
            style={{
              width: '100%',
              borderRadius: '0 0 3px 3px',
              background: 'var(--green)',
              height: `${Math.round((w.run / maxTot) * height)}px`,
            }}
          />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 8, color: 'var(--faint)' }}>
            {w.label}
          </span>
        </div>
      ))}
    </div>
  );
}

const ZONE_COLORS = ['var(--z1)', 'var(--z2)', 'var(--z3)', 'var(--z4)', 'var(--z5)'];

/** HR zones stacked band + legend (design's run-detail zone strip). */
export function ZoneBand({ zones }: { zones: { zone: number; pct: number }[] }) {
  return (
    <div>
      <div style={{ display: 'flex', height: 18, borderRadius: 6, overflow: 'hidden' }}>
        {zones.map((z) => (
          <div
            key={z.zone}
            data-testid={`zone-${z.zone}`}
            style={{
              height: '100%',
              width: `${z.pct}%`,
              background: ZONE_COLORS[(z.zone - 1) % 5],
            }}
          />
        ))}
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginTop: 10 }}>
        {zones.map((z) => (
          <span
            key={z.zone}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              fontFamily: 'var(--font-mono)',
              fontSize: 10,
              color: 'var(--muted)',
            }}
          >
            <span
              aria-hidden
              style={{
                width: 9,
                height: 9,
                borderRadius: 2,
                background: ZONE_COLORS[(z.zone - 1) % 5],
              }}
            />
            Z{z.zone} {Math.round(z.pct)}%
          </span>
        ))}
      </div>
    </div>
  );
}
