/** Human summary of a session's user agent: "Firefox · Linux", "Safari · iPhone". */
export function describeUA(ua: string): string {
  if (!ua) return 'Unknown device';
  const browser = /Edg\//.test(ua)
    ? 'Edge'
    : /Firefox\//.test(ua)
      ? 'Firefox'
      : /Chrome\//.test(ua)
        ? 'Chrome'
        : /Safari\//.test(ua)
          ? 'Safari'
          : /curl|python|Go-http/i.test(ua)
            ? 'Script'
            : 'Browser';
  const os = /iPhone/.test(ua)
    ? 'iPhone'
    : /iPad/.test(ua)
      ? 'iPad'
      : /Android/.test(ua)
        ? 'Android'
        : /Mac OS X|Macintosh/.test(ua)
          ? 'Mac'
          : /Windows/.test(ua)
            ? 'Windows'
            : /Linux|X11/.test(ua)
              ? 'Linux'
              : '';
  return os ? `${browser} · ${os}` : browser;
}
