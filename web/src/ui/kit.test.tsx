import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { StackedBars, Stepper, Toggle, ZoneBand } from './kit';

describe('ui kit', () => {
  it('Toggle flips on click', async () => {
    function Host() {
      const [on, setOn] = useState(false);
      return <Toggle on={on} onChange={setOn} label="Protect the long run" />;
    }
    render(<Host />);
    const sw = screen.getByRole('switch', { name: 'Protect the long run' });
    expect(sw).toHaveAttribute('aria-checked', 'false');
    await userEvent.click(sw);
    expect(sw).toHaveAttribute('aria-checked', 'true');
  });

  it('Stepper inc/dec drive the value', async () => {
    function Host() {
      const [v, setV] = useState(4);
      return (
        <Stepper
          label="Runs / week"
          value={v}
          onInc={() => setV((x) => Math.min(9, x + 1))}
          onDec={() => setV((x) => Math.max(0, x - 1))}
        />
      );
    }
    render(<Host />);
    await userEvent.click(screen.getByRole('button', { name: /increase Runs/ }));
    expect(screen.getByText('5')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /decrease Runs/ }));
    await userEvent.click(screen.getByRole('button', { name: /decrease Runs/ }));
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('StackedBars scales run+cf against the max total', () => {
    render(
      <StackedBars
        weeks={[
          { label: 'w1', run: 50, cf: 50 }, // max total 100
          { label: 'w2', run: 25, cf: 25 },
        ]}
        height={66}
      />,
    );
    expect(screen.getByTestId('run-w1').style.height).toBe('33px');
    expect(screen.getByTestId('cf-w1').style.height).toBe('33px');
    expect(screen.getByTestId('run-w2').style.height).toBe('17px');
  });

  it('ZoneBand renders one segment per zone with pct widths', () => {
    render(
      <ZoneBand
        zones={[
          { zone: 1, pct: 6 },
          { zone: 2, pct: 30 },
          { zone: 3, pct: 18 },
          { zone: 4, pct: 38 },
          { zone: 5, pct: 8 },
        ]}
      />,
    );
    expect(screen.getByTestId('zone-2').style.width).toBe('30%');
    expect(screen.getByText('Z4 38%')).toBeInTheDocument();
  });
});
