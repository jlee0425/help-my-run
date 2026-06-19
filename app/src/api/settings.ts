import { useState, useEffect, useCallback } from 'react';
import { getBaseUrl, getToken, saveConfig } from './config';

export interface Settings {
  baseUrl: string;
  token: string;
  loading: boolean;
  save: (baseUrl: string, token: string) => Promise<void>;
}

export function useSettings(): Settings {
  const [baseUrl, setBaseUrl] = useState('');
  const [token, setToken] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let mounted = true;
    (async () => {
      const [storedUrl, storedToken] = await Promise.all([getBaseUrl(), getToken()]);
      if (!mounted) return;
      setBaseUrl(storedUrl ?? '');
      setToken(storedToken ?? '');
      setLoading(false);
    })();
    return () => {
      mounted = false;
    };
  }, []);

  const save = useCallback(async (newBaseUrl: string, newToken: string) => {
    await saveConfig(newBaseUrl, newToken);
    setBaseUrl(newBaseUrl);
    setToken(newToken);
  }, []);

  return { baseUrl, token, loading, save };
}
