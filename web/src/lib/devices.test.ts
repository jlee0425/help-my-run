import { describe, expect, it } from 'vitest';
import { describeUA } from './devices';

describe('describeUA', () => {
  it('summarizes common agents', () => {
    expect(describeUA('Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) AppleWebKit Safari/605.1')).toBe('Safari · iPhone');
    expect(describeUA('Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko Firefox/128.0')).toBe('Firefox · Linux');
    expect(describeUA('Mozilla/5.0 (Linux; Android 15) Chrome/126.0 Mobile Safari/537.36')).toBe('Chrome · Android');
    expect(describeUA('Mozilla/5.0 (Windows NT 10.0) Chrome/126.0 Safari/537.36 Edg/126.0')).toBe('Edge · Windows');
    expect(describeUA('curl/8.6.0')).toBe('Script');
    expect(describeUA('')).toBe('Unknown device');
  });
});
