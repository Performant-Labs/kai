# Brief: #127 The "Translate again in this context" control looks like plain text, not a button

## Problem

In the context chat panel, the control labelled "Translate again in this context" (`translate.contextSend`; Spanish UI wording: "Traducir de nuevo en este contexto") does not look like a button. It reads as plain text, so it is not obvious that it can be clicked.

## Cause (from the code, not yet checked on screen)

`frontend/src/components/ContextChatPanel.svelte` (~line 145) already renders a real `<button>`, but with only `class="u-btn u-no-drag px-3 py-1 text-xs"`. In `frontend/src/app.css`, a bare `.u-btn` has no background or border; only the variants `.u-btn--primary`, `.u-btn--danger` and `.u-btn--ghost` do. While the context box is empty the button is also `disabled`, which sets `opacity: 0.5`, so it looks like faded text.

## Wanted

- The control looks like a button in both states: a visible background or border, using an existing variant (likely `u-btn--primary`).
- The disabled state (empty context text, or a request in flight) is still clearly a button, just dimmed.
- Keep `data-testid="context-send"`, the title/tooltip, and the current click and Cmd+Enter behaviour.
- Add a test that pins the variant class, or a visual check, so it does not regress to plain text.

## Out of scope

Changing the label text or the send behaviour.
