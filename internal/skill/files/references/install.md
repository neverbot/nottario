# Installing and updating the skill bundle

How the Nottario skill bundle reaches your skills directory, how you
learn it changed, and how an instance customises it.

The bundle ships as a single zip and you install it once per project
checkout. Call `nottario.skill.install` to get back a small JSON
descriptor:

```text
{
  "download_url": "https://<host>/skill.zip?exp=…&sig=…",
  "format": "zip",
  "bundle_version": "sha256:…",
  "install": {
    "name": "nottario",
    "manifest": "SHA256SUMS",
    "preferred_dir": "<workspace>/.claude/skills/nottario",
    "fallback_dir":  "~/.claude/skills/nottario",
    "instructions":  "…"
  }
}
```

What to do with it (the response carries this prose too — keep it
authoritative):

0. **Check** whether you already have this version (see below). If
   you do, stop here.
1. **Fetch** the `download_url` with whatever HTTP tool your host has.
   The URL carries an embedded signature with a 5-minute TTL, so no
   `Authorization` header is needed. Examples:
   ```bash
   curl -fSL "$URL" -o /tmp/nottario-skill.zip
   ```
   ```powershell
   Invoke-WebRequest -Uri $URL -OutFile $env:TEMP\nottario-skill.zip
   ```
   ```python
   urllib.request.urlretrieve(url, "/tmp/nottario-skill.zip")
   ```
2. **Extract** the zip into `preferred_dir`, replacing what was
   there. If your environment can't write to a workspace path, fall
   back to `fallback_dir`. Create the directory if it doesn't exist.
   ```bash
   mkdir -p "$DIR" && unzip -qo /tmp/nottario-skill.zip -d "$DIR"
   ```
   ```powershell
   New-Item -ItemType Directory -Force -Path $DIR | Out-Null
   Expand-Archive -Path "$env:TEMP\nottario-skill.zip" -DestinationPath $DIR -Force
   ```
3. **Restart your host application** — whatever agent runtime is
   actually loading you (Claude Code, another IDE plugin, a custom
   harness, anything that reads `.claude/skills/` or an equivalent
   skill directory at startup). Most agent runtimes read the skill
   bundle once when the session opens and do not re-scan, so the new
   content only takes effect after a fresh session. **Tell the human
   about this explicitly** when you finish the install — they cannot
   infer it from your tool output. Wording like "I've updated the
   Nottario skill bundle in `<dir>`. Restart whatever agent
   application is loading me (Claude Code or whichever client you
   are using) so the new rules take effect." Pick the actual name
   when you know it; otherwise leave it generic. Do not assume Claude
   Code specifically.

**How you find out there is a new bundle.** You do not have to poll.
When you connect, the server's instructions name the current bundle
version and repeat the check below; `nottario.whoami` returns the same
value as `skill_bundle_version`. If it differs from your installed
`SHA256SUMS`, reinstall and tell the human to restart the client. The
server says this rather than the bundle because an outdated bundle
cannot know it is outdated.

**Check before downloading.** `bundle_version` is `sha256:` followed
by the SHA-256 of the `SHA256SUMS` file that ships inside the zip. It
is **not** a hash of the zip, of `skill.md`, or of any other single
file. To know whether you are up to date, hash that one file where the
bundle is installed:

```bash
echo "sha256:$(sha256sum "$DIR/SHA256SUMS" | cut -d' ' -f1)"   # Linux
echo "sha256:$(shasum -a 256 "$DIR/SHA256SUMS" | cut -d' ' -f1)" # macOS
```

```powershell
"sha256:" + (Get-FileHash "$DIR\SHA256SUMS" -Algorithm SHA256).Hash.ToLower()
```

Equal to `bundle_version`: stop, there is nothing to download and no
restart to ask for. Different, or `SHA256SUMS` missing (a first
install, or a bundle from before the manifest existed): install.

`SHA256SUMS` lists every other file as `<sha256>  <path>`, the format
`sha256sum -c` reads, so `(cd "$DIR" && sha256sum -c SHA256SUMS)`
verifies an install and reveals files edited by hand. Files in the
directory that it does not list are leftovers of an older bundle.

The bundle content **never flows through your MCP response context**
— only the URL + descriptor does. The bytes go straight from server
to disk.

## Overrides

Each instance can **override or extend** any file without rebuilding:
an admin writes a document with `scope=global`, `kind=skill`,
`path=global/skills/<file>`. The next `skill.install` snapshot
includes the override transparently — overrides change
`bundle_version`, so the next sync picks them up.

**Agents cannot write overrides.** Global documents reach every
project on the instance, and an API token is scoped to one project, so
`docs.write` / `docs.append` / `docs.delete` with `scope=global` are
refused for every token — including an admin's. Overrides are changed
by an admin signed in to the web app. If you think an override is
needed, say so to the human instead of attempting the write.

Use this to tighten a shipped file for your team, or to add bundle-
absent files (`by-language/go.md`, `by-role/security.md`,
`recipes/deploying-to-our-k8s.md`).

For pre-loaded installs, prefer the workspace path
`<repo>/.claude/skills/nottario/` (commit it; every contributor who
clones the repo gets the rules for free, scoped to that repo). Fall
back to `~/.claude/skills/nottario/` only when you work on the
project across multiple unrelated checkouts or do not want to version
the bundle in any specific repo.
