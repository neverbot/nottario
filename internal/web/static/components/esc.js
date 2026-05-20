// Lit Reactive Controller that fires `handler(event)` whenever the
// user presses Escape while the host element is connected. The
// document-level listener is attached on connect and detached on
// disconnect so the controller is self-cleaning.
//
// Usage:
//
//   constructor() {
//     super();
//     new EscController(this, (e) => this._onEsc(e));
//   }
//
//   _onEsc(e) {
//     // Close the topmost dialog this page knows about. Stop
//     // propagation when something is actually closed so an outer
//     // controller (the topbar dropdown, etc.) doesn't also react.
//     if (this.selected)     { this.closeDetail();   e.stopPropagation(); return; }
//     if (this.showCreate)   { this.showCreate=false;e.stopPropagation(); return; }
//   }
export class EscController {
  constructor(host, handler) {
    this.host = host;
    this.handler = handler;
    host.addController(this);
  }
  hostConnected() {
    this._onKey = (e) => {
      if (e.key !== 'Escape') return;
      this.handler(e);
    };
    document.addEventListener('keydown', this._onKey);
  }
  hostDisconnected() {
    document.removeEventListener('keydown', this._onKey);
  }
}
