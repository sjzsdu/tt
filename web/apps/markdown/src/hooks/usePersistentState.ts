import { useState } from 'react';

export function usePersistentState<T>(key: string, initialValue: T, normalize?: (value: string | null) => T) {
  const [value, setValueState] = useState<T>(() => {
    const stored = localStorage.getItem(key);
    return normalize ? normalize(stored) : ((stored ?? initialValue) as T);
  });

  const setValue = (next: T) => {
    localStorage.setItem(key, String(next));
    setValueState(next);
  };

  return [value, setValue] as const;
}
