// Lit Reactive Controller that closes an anchored popover when the
// user presses the pointer anywhere outside it. Sibling of
// EscController in ./esc.js — the two together are what an anchored
// menu needs to feel finished: Escape closes it, clicking away closes
// it.
//
// The document-level listener is attached on connect and detached on
// disconnect, so the controller is self-cleaning.
//
// Containment is decided one of two ways:
//
//   - `selector` given: matched against every node in the event's
//     composed path. Use this when the host renders several unrelated
//     things and only one subtree counts as "inside" — a page that
//     owns one popover among much else.
//   - `selector` omitted: the host element itself is the boundary, so
//     anything inside the component counts as inside. Use this for a
//     component that IS the popover.
//
// Either way the check runs over the composed path, so it sees through
// shadow boundaries and slotted content. Whatever you match must
// contain BOTH the trigger and the panel; otherwise pressing the
// trigger would close the popover on the same press that opened it.
//
// Usage:
//
//   new OutsideClickController(this, {
//     selector: '.cycle-switcher',
//     isOpen: () => this._cycleDropdownOpen,
//     close: () => { this._cycleDropdownOpen = false; },
//   });
//
// We listen for `mousedown` rather than `click` so the popover closes
// on press instead of release, and so a click that removes its own
// target still registers. The listener runs in the capture phase so an
// inner handler calling stopPropagation cannot strand a popover open.
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
      const inside = this.selector
        ? path.some((n) => n?.matches?.(this.selector))
        : path.includes(this.host);
      if (inside) return;
      this.close();
    };
    document.addEventListener('mousedown', this._onDown, true);
  }

  hostDisconnected() {
    document.removeEventListener('mousedown', this._onDown, true);
  }
}
