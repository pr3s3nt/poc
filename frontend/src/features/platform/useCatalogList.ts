import { useCallback, useEffect, useRef, useState } from 'react';

// Loads one Platform catalog list. A load failure is its own state with
// Retry, never an empty list; reload after a registration is separate from
// the registration outcome (UC-02/03/04 UI states).
export function useCatalogList<T>(load: () => Promise<T[]>) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const latest = useRef(0);
  const loader = useRef(load);
  loader.current = load;

  const reload = useCallback(async () => {
    const request = ++latest.current;
    setLoading(true); setLoadError('');
    try {
      const next = await loader.current();
      if (request === latest.current) setItems(next);
    } catch (reason) {
      if (request === latest.current) setLoadError(`Could not load the list: ${(reason as Error).message}`);
    } finally {
      if (request === latest.current) setLoading(false);
    }
  }, []);

  useEffect(() => { void reload(); }, [reload]);
  return { items, loading, loadError, reload };
}
