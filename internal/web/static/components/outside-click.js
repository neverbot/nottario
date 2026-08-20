// Lit Reactive Controller that closes an anchored popover when the
// user presses the pointer anywhere outside it. Sibling of
// EscController in ./esc.js — the two together are what an anchored
// menu needs to feel finished: Escape closes it, clicking away closes
// it.
//
// The document-level listener is attached on connect and detached on
// disconnect, so the controller is self-cleaning.
//
// `selector` is matched against every node in the event's composed
// path, so it sees through shadow boundaries and slotted content. It
// should match the wrapper that contains BOTH the trigger and the
// panel; otherwise pressing the trigger would close the popover the
// same click that opened it.
//
// Usage:
//
//   constructor() {
//     super();
//     new OutsideClickController(this, {
//       selector: '.cycle-switcher',
//       isOpen: () => this._cycleDropdownOpen,
//       close: () => { this._cycleDropdownOpen = false; },
//     });
//   }
//
// We listen on `mousedown` rather than `click` so the popover closes
// on press instead of release, and so a click that removes its own
// target still registers.
export class OutsideClickController {
  constructor(host, { selector, isOpen, close }) {
    this.host = host;
    this.selector = selector;
    this.isOpen = isOpen;
    this.close = close;
    host.addController(this);
  }

  hostConnected() {
    this._onDown = (e) => {
      if (!this.isOpen()) return;
      const path = e.composedPath?.() || [];
      if (path.some((n) => n?.matches?.(this.selector))) return;
      this.close();
    };
    document.addEventListener('mousedown', this._onDown);
  }

  hostDisconnected() {
    document.removeEventListener('mousedown', this._onDown);
  }
}
