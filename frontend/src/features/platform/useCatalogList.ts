import { useCallback, useEffect, useRef, useState } from 'react';

// Loads one Platform catalog list. A load failure is its own state with
// Retry, never an empty list; reload after a registration is separate from
// the registration outcome (UC-02/03/04 UI states).
export function useCatalogList<T>(load: () => Promise<T[]>, describeError: (reason: unknown) => string = () => 'Không tải được danh sách từ máy chủ. Hãy thử lại.', enabled = true) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(enabled);
  const [loadError, setLoadError] = useState('');
  const latest = useRef(0);
  const loader = useRef(load);
  loader.current = load;
  const describer = useRef(describeError);
  describer.current = describeError;

  // Resolves true only when this very request loaded the list successfully.
  const reload = useCallback(async (): Promise<boolean> => {
    const request = ++latest.current;
    setLoading(true); setLoadError('');
    let loaded = false;
    try {
      const next = await loader.current();
      if (request === latest.current) { setItems(next); loaded = true; }
    } catch (reason) {
      if (request === latest.current) setLoadError(describer.current(reason));
    } finally {
      if (request === latest.current) setLoading(false);
    }
    return loaded;
  }, []);

  useEffect(() => { if (enabled) void reload(); }, [reload, enabled]);
  return { items, loading, loadError, reload };
}
