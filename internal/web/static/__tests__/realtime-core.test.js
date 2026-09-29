import { strict as assert } from 'node:assert';
import { beforeEach, describe, it } from 'node:test';

import { Multiplexer, streamURL, wants } from '../realtime-core.js';

// FakeStream stands in for EventSource: the test decides when it
// connects and what it receives.
class FakeStream {
  constructor(url) {
    this.url = url;
    this.readyState = 0;
    this.closed = false;
    this.onopen = null;
    this.onmessage = null;
    FakeStream.all.push(this);
  }
  open() {
    this.readyState = 1;
    this.onopen?.();
  }
  // The browser giving up for good, as it does on an HTTP error.
  fail() {
    this.readyState = 2;
    this.onerror?.();
  }
  // A network blip: EventSource keeps trying on its own.
  blip() {
    this.readyState = 0;
    this.onerror?.();
  }
  send(event) {
    this.onmessage?.({ data: JSON.stringify(event) });
  }
  close() {
    this.closed = true;
  }
}
FakeStream.all = [];

const live = () => FakeStream.all.filter((s) => !s.closed);

describe('streamURL', () => {
  it('asks for global events only when nobody follows a project', () => {
    assert.equal(streamURL([]), '/events');
    assert.equal(streamURL([null, undefined, '']), '/events');
  });

  it('lists each project once, in a stable order', () => {
    assert.equal(streamURL(['b', 'a', 'b', null]), '/events?project_id=a&project_id=b');
  });
});

describe('wants', () => {
  it('sends instance-wide events to everyone', () => {
    assert.ok(wants({ projectId: 'p1' }, { type: 'version_status' }));
    assert.ok(wants({ projectId: null }, { type: 'realtime.reconnected' }));
  });

  it('sends project events only to that project', () => {
    assert.ok(wants({ projectId: 'p1' }, { type: 'task.created', project_id: 'p1' }));
    assert.ok(!wants({ projectId: 'p2' }, { type: 'task.created', project_id: 'p1' }));
    assert.ok(!wants({ projectId: null }, { type: 'task.created', project_id: 'p1' }));
  });
});

describe('Multiplexer', () => {
  let mux;
  let timers;
  const runTimers = () => {
    const due = timers;
    timers = [];
    for (const t of due) t.fn();
  };
  beforeEach(() => {
    FakeStream.all = [];
    timers = [];
    mux = new Multiplexer(
      (url) => new FakeStream(url),
      (fn, ms) => timers.push({ fn, ms }),
    );
  });

  it('serves many subscribers from ONE connection — the whole point', () => {
    // A page subscription and the update banner, in one tab or many.
    mux.subscribe('board', 'p1', () => {});
    FakeStream.all[0].open();
    mux.subscribe('banner', null, () => {});
    mux.subscribe('gantt', 'p1', () => {});
    assert.equal(live().length, 1, 'one stream, however many subscribers');
    assert.equal(mux.url, '/events?project_id=p1');
  });

  it('routes each event to the subscribers that want it', () => {
    const got = { p1: [], p2: [], banner: [] };
    mux.subscribe('p1', 'p1', (e) => got.p1.push(e.type));
    mux.subscribe('p2', 'p2', (e) => got.p2.push(e.type));
    mux.subscribe('banner', null, (e) => got.banner.push(e.type));
    const stream = live()[0];
    stream.open();

    stream.send({ type: 'task.created', project_id: 'p1' });
    stream.send({ type: 'version_status' });

    assert.deepEqual(got.p1, ['task.created', 'version_status']);
    assert.deepEqual(got.p2, ['version_status']);
    assert.deepEqual(got.banner, ['version_status']);
  });

  it('switches streams without losing or repeating an event', () => {
    const got = [];
    mux.subscribe('a', 'p1', (e) => got.push(e.n));
    const first = live()[0];
    first.open();
    first.send({ type: 'x', project_id: 'p1', n: 1 });

    // A tab on another project joins: a wider stream is needed.
    mux.subscribe('b', 'p2', () => {});
    const second = FakeStream.all[1];
    assert.equal(second.url, '/events?project_id=p1&project_id=p2');
    // Until the new one connects, the old one keeps delivering.
    assert.ok(!first.closed, 'the old stream was dropped before its replacement connected');
    first.send({ type: 'x', project_id: 'p1', n: 2 });

    second.open();
    assert.ok(first.closed, 'the old stream outlived the switch');
    // Anything still arriving on the old stream is ignored…
    first.send({ type: 'x', project_id: 'p1', n: 99 });
    // …and the new one takes over.
    second.send({ type: 'x', project_id: 'p1', n: 3 });

    assert.deepEqual(got, [1, 2, 3]);
  });

  it('tells everyone to reload after the stream recovers from a drop', () => {
    const got = [];
    mux.subscribe('a', 'p1', (e) => got.push(e.type));
    const stream = live()[0];
    stream.open(); // first connect: nothing was missed
    assert.deepEqual(got, []);
    stream.open(); // EventSource reconnected after an error
    assert.deepEqual(got, ['realtime.reconnected']);
  });

  it('does not report a planned switch as a reconnect', () => {
    const got = [];
    mux.subscribe('a', 'p1', (e) => got.push(e.type));
    live()[0].open();
    mux.subscribe('b', 'p2', () => {});
    FakeStream.all[1].open();
    assert.deepEqual(got, [], 'a wider stream is not a lost connection');
  });

  it('closes the connection when the last subscriber leaves', () => {
    mux.subscribe('a', 'p1', () => {});
    live()[0].open();
    mux.unsubscribe('a');
    assert.equal(live().length, 0);
    assert.equal(mux.url, null);
  });

  it('narrows the stream when a project is no longer followed', () => {
    mux.subscribe('a', 'p1', () => {});
    mux.subscribe('b', 'p2', () => {});
    live().at(-1).open();
    mux.unsubscribe('b');
    live().at(-1).open();
    assert.equal(mux.url, '/events?project_id=p1');
    assert.equal(live().length, 1);
  });

  it('drops a half-open replacement when the change is undone', () => {
    mux.subscribe('a', 'p1', () => {});
    live()[0].open();
    mux.subscribe('b', 'p2', () => {}); // replacement starts connecting…
    mux.unsubscribe('b'); // …and is no longer needed
    assert.equal(live().length, 1, 'the unneeded replacement stayed open');
    assert.equal(mux.url, '/events?project_id=p1');
  });

  it('forgets every subscription of a tab that went away', () => {
    mux.subscribe('tab1:a', 'p1', () => {});
    mux.subscribe('tab2:a', 'p2', () => {});
    live().at(-1).open();
    mux.unsubscribeWhere((id) => id.startsWith('tab2:'));
    live().at(-1).open();
    assert.equal(mux.size, 1);
    assert.equal(mux.url, '/events?project_id=p1');
  });

  it('reopens a stream the browser gave up on — a 401 before sign-in, a 502 mid-deploy', () => {
    const got = [];
    mux.subscribe('a', 'p1', (e) => got.push(e.type));
    const dead = live()[0];
    dead.fail(); // HTTP error: EventSource will never retry this one
    assert.equal(live().length, 0);
    assert.equal(timers.length, 1, 'no retry was scheduled');

    runTimers();
    const fresh = live()[0];
    assert.ok(fresh && fresh !== dead, 'the dead stream was not replaced');
    fresh.open();
    assert.equal(mux.url, '/events?project_id=p1');
    // Never connected before, so nothing was missed.
    assert.deepEqual(got, []);
  });

  it('tells everyone to reload when a live stream dies and comes back', () => {
    const got = [];
    mux.subscribe('a', 'p1', (e) => got.push(e.type));
    live()[0].open();
    live()[0].fail();
    runTimers();
    live()[0].open();
    assert.deepEqual(got, ['realtime.reconnected']);
  });

  it('backs off between attempts and resets once connected', () => {
    mux.subscribe('a', 'p1', () => {});
    live()[0].fail();
    const first = timers[0].ms;
    runTimers();
    live()[0].fail();
    const second = timers[0].ms;
    assert.ok(second > first, `retry did not back off: ${first} then ${second}`);
    runTimers();
    live()[0].open();
    live()[0].fail();
    assert.equal(timers[0].ms, first, 'a successful connection did not reset the backoff');
  });

  it('leaves transient drops to EventSource itself', () => {
    mux.subscribe('a', 'p1', () => {});
    live()[0].open();
    live()[0].blip();
    assert.equal(timers.length, 0, 'retried a stream the browser is still retrying');
    assert.equal(live().length, 1);
  });

  it('does not resurrect a stream nobody wants any more', () => {
    mux.subscribe('a', 'p1', () => {});
    live()[0].fail();
    mux.unsubscribe('a');
    runTimers();
    assert.equal(live().length, 0);
  });
});
