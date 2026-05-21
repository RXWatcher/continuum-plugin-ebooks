# User guide (customer-facing)

This page is for end users of the ebooks portal. Operators should read
[operations.md](operations.md) instead.

## What the portal does

The portal is a web app at the root URL where your continuum
administrator installed it. You can:

- Browse the library and read EPUBs in the browser.
- Track reading progress, mark favorites, rate and review.
- Highlight and annotate while reading.
- Pull the same library into other apps over OPDS.
- Sync KOReader reading progress across devices.
- Send books to a Kobo eReader over USB-free transfer.
- Email books to your Kindle.
- Request books that aren't currently in the library.

## Reading in the browser

Click any book → **Read**. Your progress, highlights, and notes are
saved per-account on the server. The reader works on tablets and
laptops; on phones the bottom navigation collapses.

If a book has multiple formats (EPUB, PDF, MOBI, …), the in-browser
reader prefers EPUB. Other formats download to your device.

## OPDS (Marvin, Moon+Reader, etc.)

OPDS lets reader apps browse the same library you see on the portal.

**Set up:**

1. Open the portal → **Settings → OPDS tokens**.
2. Click **Create token**, give it a name (e.g. "Marvin on iPad").
3. The portal shows a long random string **once**. Copy it.
4. In your OPDS-compatible app, add a feed with:
   - URL: `https://your-portal/opds/`
   - Username: your continuum user id (shown above the token list)
   - Password: the token string

The token isn't your account password — it's a per-device key. If you
lose a device, click **Revoke** on its token and the device immediately
loses access.

**Apps known to work:** Marvin, Moon+Reader, KOReader, Aldiko Next,
Foliate, Thorium Reader.

**Common gotchas:**

- Always include the trailing `/` in the URL.
- If your portal is hosted at a port other than 80/443, include it in
  the URL.
- HTTP-only setups: some apps refuse to send credentials over plain
  HTTP. Enable HTTPS or check the app's "allow plain HTTP" setting.

## KOReader sync (across devices)

You can link your KOReader devices so they share reading progress.
You have two paths:

**Recommended — log in from the SPA first:**

1. **Settings → KOReader sync → Register**. Pick a username and
   password.
2. On each KOReader device: Tools → Progress sync → Register, then use
   the same username and password.
3. KOReader will push progress on page turns; other devices will pull
   on open.

**Direct registration from KOReader** (if you don't have an account on
the portal):

You can register a kosync-only account straight from KOReader by
pointing it at `https://your-portal/kosync/` and choosing a username.
This account is *not* linked to a continuum identity — it just syncs
progress between your own KOReader devices.

**Sync URL:** `https://your-portal/kosync/`

**Gotchas:**

- If you registered through KOReader directly and later get a
  continuum account, your sync state stays under the synthetic
  identity. Re-register from the SPA to merge.
- Two devices on the same book see whichever wrote progress most
  recently. Long airplane reads sync only when you reconnect.

## Send to Kobo

Click a book → **Send to Kobo**. The portal converts the EPUB to
Kobo's `.kepub.epub` format and returns a **transfer URL** and a
short code.

**To install on the Kobo:**

1. On the Kobo, open the browser (Beta features → Web browser).
2. Visit the transfer URL or `your-portal/kobo/<code>`.
3. The download starts; the file appears in your library after a sync.

The URL is valid for 30 minutes. After expiry the file is removed from
the server.

**Gotchas:**

- The Kobo browser is slow on first load. Be patient.
- The 4-character code shown in earlier versions has been replaced with
  a 10-character code; bookmarks of old codes won't work.

## Send to Kindle

Click a book → **Send to Kindle** → enter your `@kindle.com` address.

The portal queues the send and emails the book within a couple of
minutes. The status appears in **Settings → Kindle send log**.

**One-time setup on Amazon's side:**

Before the first send, you must allow-list the portal's sender address
in your Amazon account. The administrator can tell you the sender
address.

  Amazon → Manage Your Content and Devices → Preferences →
  Personal Document Settings → Approved Personal Document E-mail List
  → Add e-mail address.

**Gotchas:**

- Only `@kindle.com`, `@kindle.cn`, and `@free.kindle.com` addresses
  are accepted (Amazon's official Send-to-Kindle domains).
- "Sent" in the portal means the email left the SMTP relay. Amazon
  delivers it to your device asynchronously; usually <5 minutes,
  sometimes longer.
- If the send shows `failed` after 3 retries, the administrator will
  see the underlying SMTP error in the log. Ask them to check.

## Requesting books

If the library doesn't have a book yet, you can request it:

1. **Search → "Request this book"** when search returns no results, or
   **My library → Requests → New request**.
2. Fill in title and author (ISBN helps when there are multiple
   editions).
3. Submit. Status starts as `pending`.

What happens next depends on the administrator's configuration:

- **Auto-approve mode**: the request goes straight to the configured
  download provider. Status flows through `submitted → acknowledged →
  downloading → fulfilled`.
- **Manual approval**: an admin reviews each request. Status stays
  `pending` until approved, then proceeds as above.

You see live status under **My library → Requests**. When the request
succeeds, the new book shows up in your library; the request links to
it.

**Cancelling:** you can cancel `pending` requests. Submitted or later
requests are owned by the download provider — the admin can deny them
on your behalf.

## Privacy and your data

- **Reading progress and annotations** are per-account on this
  continuum install. Not shared with anyone.
- **Highlights and notes** stay in your account; export from
  **Settings → Export data**.
- **OPDS tokens** are bound to your continuum account; an admin can
  revoke them but cannot read them.
- The portal **does not** send analytics off the server.
