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
  // The submit button is usually inside the form. Two fallbacks:
  //  - a standalone click-handler whose target IS the button.
  //  - a button placed outside the form but linked via `form="<id>"`
  //    (HTML form-association), looked up by id in the containing
  //    root (works in both light DOM and shadow DOM since we walk
  //    up from the form's getRootNode()).
  let btn = form?.querySelector?.('button[type="submit"]');
  if (!btn && form?.id) {
    const root = form.getRootNode?.() || document;
    btn = root.querySelector(`button[form="${form.id}"]`);
  }
  if (!btn && form?.tagName === 'BUTTON') btn = form;
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
