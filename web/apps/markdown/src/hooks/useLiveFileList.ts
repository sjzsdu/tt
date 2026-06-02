import { useEffect } from 'react';
import type { ListResponse } from '../types';
import { api } from '../api';

export function useLiveFileList(onList: (list: ListResponse) => void) {
  useEffect(() => {
    let closed = false;
    let retry: number | undefined;
    let ws: WebSocket | undefined;

    const connect = () => {
      if (closed) return;
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      ws = new WebSocket(`${proto}//${location.host}/ws`);
      ws.onmessage = () => {
        api.list().then(onList).catch(e => console.error('[WS list]', e));
      };
      ws.onclose = () => {
        if (!closed) retry = window.setTimeout(connect, 3000);
      };
      ws.onerror = e => console.error('[WS]', e);
    };

    connect();
    return () => {
      closed = true;
      if (retry) window.clearTimeout(retry);
      ws?.close();
    };
  }, [onList]);
}
