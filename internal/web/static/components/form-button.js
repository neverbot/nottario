// formButton: button-anchored save acknowledgement helper.
//
// Wraps a form's submit handler so the submit button itself flips:
//
//   rest "Save changes" → disabled "Saving…" → "✓ Saved" (1.5s) → rest
//
// Used when the user's eyes are already on the button. Self-
// contained: no global element, no extra DOM, no extra plumbing.
// Pair with toast.success/error for cases where the result happens
// off-button or the form's dialog closes on success.
//
// Usage:
//
//   async save(e) {
//     await formButton(e, async () => {
//       const res = await fetch(...);
//       if (!res.ok) throw new Error('failed');
//     });
//   }
//
// The helper calls e.preventDefault() for you. If the fn throws,
// the button restores to its rest label and stays clickable so the
// user can retry; the error bubbles up to the caller for any other
// handling (logging, toast, inline error block).

const SAVING_LABEL = 'Saving…';
const OK_LABEL = '✓ Saved';
const OK_HOLD_MS = 1500;

export async function formButton(submitEvent, fn, opts = {}) {
  submitEvent.preventDefault();
  const form = submitEvent.target;
  // The submit button is usually inside the form, but some callers
  // pass an event whose target IS the button (e.g. a click handler
  // on a standalone Save button). Handle both.
  const btn =
    form?.querySelector?.('button[type="submit"]') || (form?.tagName === 'BUTTON' ? form : null);
  if (!btn) return fn();

  const savingLabel = opts.savingLabel || SAVING_LABEL;
  const okLabel = opts.okLabel || OK_LABEL;
  const okHold = typeof opts.okHold === 'number' ? opts.okHold : OK_HOLD_MS;
  const restLabel = btn.textContent;
  const restColor = btn.style.color;

  btn.disabled = true;
  btn.textContent = savingLabel;

  try {
    const result = await fn();
    btn.textContent = okLabel;
    btn.style.color = 'var(--fg-on-accent)';
    setTimeout(() => {
      btn.disabled = false;
      btn.textContent = restLabel;
      btn.style.color = restColor;
    }, okHold);
    return result;
  } catch (err) {
    btn.disabled = false;
    btn.textContent = restLabel;
    btn.style.color = restColor;
    throw err;
  }
}
