import { useEffect, useState } from 'react';

/** Reactive media query (desktop breakpoint = min-width:1024px). */
export function useMedia(query: string): boolean {
  const [matches, setMatches] = useState(() => window.matchMedia(query).matches);
  useEffect(() => {
    const mq = window.matchMedia(query);
    const onChange = () => setMatches(mq.matches);
    mq.addEventListener('change', onChange);
    setMatches(mq.matches);
    return () => mq.removeEventListener('change', onChange);
  }, [query]);
  return matches;
}

export const useDesktop = () => useMedia('(min-width: 1024px)');
