import appConfig from '../../app.json';

describe('app.json M2 config', () => {
  const expo = (appConfig as { expo: Record<string, unknown> }).expo;

  it('lists expo-notifications in the plugins array', () => {
    const plugins = expo.plugins as Array<string | [string, unknown]>;
    const names = plugins.map((p) => (Array.isArray(p) ? p[0] : p));
    expect(names).toContain('expo-notifications');
  });

  it('exposes an EAS projectId under expo.extra.eas.projectId', () => {
    const extra = expo.extra as { eas?: { projectId?: string } } | undefined;
    expect(extra?.eas?.projectId).toBeDefined();
    expect(typeof extra?.eas?.projectId).toBe('string');
    expect((extra?.eas?.projectId ?? '').length).toBeGreaterThan(0);
  });
});
