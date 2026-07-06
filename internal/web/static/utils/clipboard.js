// Copy `text` to the OS clipboard. Returns true on success, false on
// any failure. Never throws.
//
// Two paths:
//   1. `navigator.clipboard.writeText` — the modern API. Only available
//      in secure contexts (HTTPS, `localhost`, `127.0.0.1`, `::1`) so
//      self-hosted deployments on plain HTTP (VPN-only, LAN) fall
//      through.
//   2. Hidden `<textarea>` + `document.execCommand('copy')` — legacy
//      but works everywhere. We hide the textarea offscreen (not
//      `display:none`, which would break the selection) and remove
//      it in a `finally`.
//
// Kept in `utils/` so callers don't have to know which path fired;
// the token dialog is today's only site but the same helper will
// power any future copy affordances (commit SHA, task id, arch key).
export async function copyToClipboard(text) {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (_) {
      // Fall through to the legacy path — Firefox on some Linux
      // setups can reject the async write even in secure contexts.
    }
  }
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  ta.style.position = 'fixed';
  ta.style.top = '0';
  ta.style.left = '0';
  ta.style.width = '1px';
  ta.style.height = '1px';
  ta.style.padding = '0';
  ta.style.border = 'none';
  ta.style.outline = 'none';
  ta.style.boxShadow = 'none';
  ta.style.background = 'transparent';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  try {
    ta.select();
    ta.setSelectionRange(0, text.length);
    return document.execCommand('copy');
  } catch (_) {
    return false;
  } finally {
    document.body.removeChild(ta);
  }
}
