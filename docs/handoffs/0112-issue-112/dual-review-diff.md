## Implementation Review (Round 2)

### BLOCK finding responses

**[B-1] ACCEPTED** — The mounted test now exists and is genuinely discriminating. The sequence (setting ON at mount → explicit 🧷 FALSE → setting toggled OFF → setting toggled back ON → Settings-close re-pin) exercises exactly the branch order the BLOCK was about: the clicked value must survive both a contradicting live broadcast and the close-triggered re-pin. The assertions check `SetAlwaysOnTop` re-ran at the clicked value on every leg and that storage ends at the clicked value — an implementation that fell back to the setting or mount-time level would visibly fail. Full-suite green (82 files / 890 tests) confirms no regression, and NIT-1 (payload-free broadcast) was folded into the same edit.

Supporting NV items, briefly:

- **NV-1** — verified; pre-existing import, compiled in all 11 mounted tests.
- **NV-2** — verified; the window whose hook consumes `app` is created *through* `app`, so nil is unreachable there. Guard removal is behavior-neutral.
- **NV-3** — well evidenced: Wails `Emit` routes through the app-wide `customEventProcessor`, and the in-repo `EventEnginesChanged` precedent uses the identical settings→translate-window path in production since 0.5.x.
- **NV-4** — verified against beta.24 signatures; the comma-ok always succeeds. Manual registration/firing check correctly carried as a manual item.
- **NV-5** — the corner (stored choice contradicting the setting ⇒ one-time correction on first mount after that state) is real, correctly bounded (requires an explicit contradictory 🧷 click; impossible on fresh install), and the disclosure is sound. Reading localStorage from Go's creation path would be the worse alternative. Accepted as documented behavior.

W-1 and W-2 are reasonably dispositioned (idempotent cleanup; brittleness note acknowledged with a bounded scope for failure — a second Cmd+W registration elsewhere in main.go, which doesn't exist today). Neither blocks.

### Verdict

**PASS** — all BLOCK findings accepted; testing may proceed.