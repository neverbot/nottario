// SharedWorker holding the browser's single realtime stream for every
// open Nottario tab. Each tab talks to it over a MessagePort:
//
//   tab → worker  { op: 'sub', id, projectId }   follow a project (null = global)
//                 { op: 'unsub', id }            stop following
//                 { op: 'bye' }                  the tab is going away
//                 { op: 'state' }                diagnostics: replies { state }
//   worker → tab  { id, event }                  an event for subscription `id`
//
// Subscription ids are only unique within a tab, so the worker keys
// them by port.
import { Multiplexer } from './realtime-core.js';

const mux = new Multiplexer((url) => new EventSource(url));
let nextPort = 0;

self.onconnect = (e) => {
  const port = e.ports[0];
  const tag = `p${nextPort++}:`;
  port.onmessage = (msg) => {
    const m = msg.data || {};
    if (m.op === 'sub') {
      mux.subscribe(tag + m.id, m.projectId, (event) => port.postMessage({ id: m.id, event }));
    } else if (m.op === 'unsub') {
      mux.unsubscribe(tag + m.id);
    } else if (m.op === 'state') {
      port.postMessage({ state: mux.state() });
    } else if (m.op === 'bye') {
      mux.unsubscribeWhere((id) => id.startsWith(tag));
    }
  };
  port.start();
};
