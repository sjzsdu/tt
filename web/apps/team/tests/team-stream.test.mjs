import assert from 'node:assert/strict';
import { connectTeamStateStream } from '../.test-dist/src/team-stream.js';

class FakeEventSource {
  readyState = 0;
  onopen = null;
  onerror = null;
  listeners = new Set();
  closed = false;

  addEventListener(type, listener) {
    assert.equal(type, 'state');
    this.listeners.add(listener);
  }

  removeEventListener(type, listener) {
    assert.equal(type, 'state');
    this.listeners.delete(listener);
  }

  close() {
    this.closed = true;
  }

  emitState(state) {
    for (const listener of this.listeners) listener({ data: JSON.stringify(state) });
  }
}

const source = new FakeEventSource();
const states = [];
const errors = [];
const disconnect = connectTeamStateStream(
  state => states.push(state),
  error => errors.push(error),
  url => {
    assert.equal(url, '/api/events');
    return source;
  },
);

source.onerror();
assert.equal(errors.at(-1), '实时连接已断开，正在重连…');
source.onopen();
assert.equal(errors.at(-1), '');
source.emitState({ team: { team: 'review' }, events: [] });
assert.equal(states.at(-1).team.team, 'review');

for (const listener of source.listeners) listener({ data: '{invalid' });
assert.equal(errors.at(-1), '实时状态数据无效');

disconnect();
assert.equal(source.closed, true);
assert.equal(source.listeners.size, 0);
console.log('team SSE connection tests passed');
