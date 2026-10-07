<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { t, langName, engineName } from '../i18n';
  import { rootStyle } from '../stores/theme';
  import { currentLang } from '../stores/ui';
  import { persisted, pinKey } from '../stores/persisted';
  import { rootStyleToStyle } from '../utils/style';
  import { onEvent, emitEvent } from '../runtime';
  import { Window, Clipboard } from '@wailsio/runtime';

  // Pin state persists to localStorage and survives reopening the window.
  // #39: pinned by default (modal semantics) — the working window stays above other apps until
  // explicitly hidden (hotkey toggle / red X / tray) and no longer "disappears" below them when
  // it loses focus. The 🧷 button keeps its meaning: the unpin/pin toggle.
  const PIN_MODAL_MIGRATION_KEY = 'kai:translate:pinnedModalDefault';
  const pinnedStore = persisted<boolean>(pinKey('translate'), true);
  // One-time migration: existing users were silently written false under the old default (false)
  // by subscribe, so merely flipping the constructed default does nothing for them — on seeing
  // "stored false and no migration flag" we reset to pinned and set the flag. Users who unpin
  // after the migration (flag already present) are always respected.
  if (typeof localStorage !== 'undefined' && !localStorage.getItem(PIN_MODAL_MIGRATION_KEY)) {
    if (localStorage.getItem(pinKey('translate')) === JSON.stringify(false)) {
      pinnedStore.set(true);
    }
    localStorage.setItem(PIN_MODAL_MIGRATION_KEY, '1');
  }
  let pinned = $derived($pinnedStore);
  async function togglePin() {
    const next = !pinned;
    pinnedStore.set(next);
    try {
      await Window.SetAlwaysOnTop(next);
    } catch (e) {
      console.error('toggle pin failed', e);
    }
  }

  // Auto-read clipboard and translate: when enabled, SaveConfig turns the copy hotkey
  // (execkeys.copy.enabled/fallback) off and records a snapshot, restored on disable, avoiding
  // double-triggering with the copy hotkey. The toggle state persists in settings.json
  // (auto_clipboard). When enabled, pressing the "input translation" hotkey makes the backend
  // read the system clipboard and translate directly — no more frontend polling.
  let autoClipboard = $state(false);

  async function applyAutoClipboard(next: boolean) {
    // Must save the full Settings, otherwise SaveConfig zeroes out the other fields.
    const cfg = (await GetConfig()) ?? ({} as any);
    if (next) {
      // Record the snapshot only if none exists yet (avoid double snapshots), and turn off both copy-hotkey switches.
      if (!cfg.copy_key_snapshot && cfg.execkeys?.copy) {
        cfg.copy_key_snapshot = { ...cfg.execkeys.copy };
      }
      if (cfg.execkeys?.copy) {
        cfg.execkeys.copy.enabled = false;
        cfg.execkeys.copy.fallback = false;
      }
      cfg.auto_clipboard = true;
    } else {
      // Restore the copy hotkey's original state and drop the snapshot.
      if (cfg.copy_key_snapshot && cfg.execkeys?.copy) {
        cfg.execkeys.copy.enabled = cfg.copy_key_snapshot.enabled;
        cfg.execkeys.copy.fallback = cfg.copy_key_snapshot.fallback;
      }
      cfg.copy_key_snapshot = null;
      cfg.auto_clipboard = false;
    }
    await SaveConfig(cfg as any);
    autoClipboard = next;
    // Broadcast to the settings page, disabling/restoring the copy-hotkey switches in real time.
    emitEvent(EventAutoClipboardChanged, next);
  }

  import {
    EventTranslateResult,
    EventTranslateProgress,
    EventInputFill,
    EventWindowClosing,
    EventEnginesChanged,
    EventAutoClipboardChanged,
    EventClearContextShortcutChanged,
    EventCopyKeyFailed,
    EventDoubleCopyPermissionMissing,
    EventAccessibilityMissing,
  } from '../utils/events';
  import type { TranslateProgressPayload } from '../utils/events';
  import { WindowSettings, WindowTranslate } from '../constants/window';
  import type { TranslateResult } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
  import type {
    AllEngineItem,
    EngineListItem,
    NamedItem,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import { TRANSLATE_LANG, ALL_TRANSLATE_LANGS, type TranslateLang } from '../constants/lang';
  import {
    TranslateMulti,
    CancelTranslate,
    PlanSourceSwitch,
    ReportSourceSwitchSkipped,
    CorrectSource,
    CorrectionAvailability,
    RetranslateWithContext,
    BackTranslate,
    CommitAutoHistory,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/translatewrapper.ts';
  import { Learn as LearnLangVariant } from '@bindings/cnb.cool/dtapp/kai/internal/service/langprefwrapper.ts';
  import { learnFromSelection } from '../utils/langLearn.ts';
  import {
    GetEngines,
    GetAllEngines,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts';
  import {
    activeEngineFor,
    statusDots,
    paneState,
    isEngineOptionDisabled,
    engineOptionLabel,
    resetEdits,
    failureMessage,
    type DotState,
  } from '../utils/resultPane.ts';
  import {
    emptySession,
    restoreSession,
    type TranslateSession,
  } from '../utils/translateSession.ts';
  import {
    emptyHistory,
    record as recordChange,
    breakTyping,
    undo as undoStep,
    redo as redoStep,
    canUndo,
    canRedo,
    caretAfterRestore,
    type ChangeKind,
  } from '../utils/sourceHistory.ts';
  import {
    newRequestID,
    requestSettled,
    startProgress,
    applyChunk,
    progressLine,
    formatClock,
    splitElapsed,
    type ProgressLine,
    type ProgressState,
  } from '../utils/translateProgress.ts';
  import { swapLanguages } from '../utils/swapLangs.ts';
  import { cueActive, type SwitchCue } from '../utils/sourceSwitch.ts';
  import { createSourceSwitcher } from './sourceSwitchFlow.ts';
  import {
    correctionShown,
    textToSend,
    unavailableKey,
    type AppliedCorrection,
  } from '../utils/sourceCorrection.ts';
  import { isTargetDisabled } from '../utils/targetCapability.ts';
  import ContextChatPanel from './ContextChatPanel.svelte';
  import { backTranslateOn } from '../stores/backTranslate';
  import { autoTranslateOn } from '../stores/autoTranslate';
  import {
    AUTO_COMMIT_DELAY_MS,
    autoKindOf,
    autoTranslateDelay,
    commitStillValid,
    createAutoScheduler,
  } from '../utils/autoTranslate.ts';
  import {
    acceptBack,
    backPair,
    backShownFor,
    buildBackRequest,
    needsBack,
    shouldBackTranslate,
    type BackAnswer,
    type BoundBack,
  } from '../utils/backTranslation.ts';
  import { clearContextShortcut } from '../stores/clearContextShortcut';
  import {
    emptyChat,
    clearChat,
    hasContext,
    addUserMessage,
    buildContextRequest,
    applyAnswer,
    answerNoteKey,
    shownFor,
    dropRetranslation,
    isClearShortcut,
    type ContextChat,
    type ContextAnswer,
    type BoundRetranslation,
  } from '../utils/contextChat.ts';
  import {
    GetLanguages,
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { ShowSettings } from '@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts';

  // The retained session (issue #81): the source text, the per-engine results and the request
  // marker survive closing the window and an app restart, so reopening shows the last translation.
  // It uses the same localStorage-backed store as the pin and the last-used engine; the values
  // read back are untrusted, so restoreSession validates them and falls back to the empty session
  // when anything is off. Read once, here. The $effect below is the only writer: Clear, a new
  // request and every arriving result all reach storage through it. The two languages are not
  // part of the session, they persist through the settings file (loadDefaults / persistLangs).
  const sessionStore = persisted<TranslateSession>('kai:translate:session', emptySession());
  const restored = restoreSession(get(sessionStore));

  let input = $state(restored.input);
  let engines = $state<EngineListItem[]>([]);
  let languages = $state<NamedItem[]>([]);
  let fromLang = $state<TranslateLang>(TRANSLATE_LANG.Auto);
  let toLang = $state<TranslateLang>(TRANSLATE_LANG.EN);
  // Per-engine translation results, aggregated by engine name (multi-engine concurrency,
  // arriving one by one). Seeded from the retained session: those entries were validated as
  // objects that name their engine, but restoreSession is bindings-free and types them
  // structurally (PaneResult), so the cast to the generated result type is made here, once.
  let results = $state<Record<string, TranslateResult>>(
    restored.results as unknown as Record<string, TranslateResult>,
  );
  // The target the current results were requested with, captured when the request is sent and
  // retained with the results (the #81 session field). Nothing reads it back any more: a result
  // is always translated into the requested target (issue #80), so there is no second target to
  // compare it with. Read-only bookkeeping: nothing writes toLang from it.
  let requestedTo = $state<string>(restored.requestedTo);
  // Whether a translation was requested and not cleared since (issue #81): set by doTranslate(),
  // reset by Clear. "No result and not loading" alone cannot tell an idle window (nothing requested
  // yet, or just cleared) from a request whose engines all failed, and only the second one is a
  // failure. Retained with the results; a restore that finds no result reads as idle again.
  let requested = $state(restored.requested);
  // Whether a request was made in THIS window run (issue #116). The persisted `requested` survives a
  // restore whenever one engine's result did, so an engine that had failed last time (its failure
  // payload is dropped on restore) would read as failed for a request the user does not remember.
  // Failure is decided from this run-local marker: a restored window shows stored results and is
  // otherwise blank; set by doTranslate(), reset by Clear.
  let requestedThisRun = $state(false);
  let loading = $state(false);
  // The request is still open (issue #109): set by doTranslate(), cleared when the request settles,
  // i.e. the call has returned and every engine the backend started has reported a translation, a
  // failure or a cancel (requestSettled), or when the call itself failed. No timer ends it: a slow
  // engine is waited for until the user cancels. `loading` alone cannot say this because the first
  // arriving result clears it; with two engines the fast one would end it and the slower active
  // engine would read as failed until it answered.
  let awaiting = $state(false);
  // The id of the request this window is waiting on. doTranslate names it before it calls the
  // backend, because a fast engine can answer before the call returns; every result and progress
  // event carries it back, and an event of any other request (one that a newer translate or a Clear
  // replaced) is ignored. '' while nothing is open.
  let requestId = $state('');
  // The engines the backend says it started for the request; null until TranslateMulti returned.
  let started = $state<string[] | null>(null);
  // What is known about each running engine of the request (its started and chunk events), which
  // the progress line is drawn from.
  let progress = $state<Record<string, ProgressState>>({});
  // The clock the progress line reads: this window's own, re-read once a second while a request is
  // open, so the elapsed time is measured from when this window heard of the engine, never from the
  // backend's clock.
  let nowMs = $state(Date.now());

  // Write-back of the retained session (issue #81): whenever the text, the results, the requested
  // target or the requested marker change, the whole session is stored again. Clear resets these
  // same four fields, so it empties what is stored too. Neither the loading flag (nothing is in
  // flight after a restart) nor the manual result edits (discarded by design, §3) are part of it.
  // The snapshot detaches the stored value from the reactive proxies.
  $effect(() => {
    sessionStore.set({
      input,
      results: $state.snapshot(results),
      requestedTo,
      requested,
    });
  });

  // Two-pane layout (issue #10, locked decision: always side by side, no stacked fallback).
  // The divider splits the content row into the source pane (left) and the results pane (right);
  // the ratio persists to localStorage and survives reopening the window.
  // All clamping/conversion math lives in utils/paneLayout.ts (pure functions, covered by vitest).
  import { clampRatio, ratioFromPoint } from '../utils/paneLayout.ts';
  const dividerStore = persisted<number>('translate:divider', 0.5);
  let panesEl = $state<HTMLElement | null>(null);
  let leftRatio = $derived(clampRatio($dividerStore));

  // Divider drag: after mousedown, mousemove is tracked on window (dragging past the divider
  // keeps working); mouseup releases. Pointer horizontal position → row ratio → clamp → write
  // back to the persisted store.
  function startDividerDrag(ev: MouseEvent) {
    ev.preventDefault();
    const row = panesEl;
    if (!row) return;
    const move = (e: MouseEvent) => {
      dividerStore.set(
        ratioFromPoint(e.clientX, row.getBoundingClientRect().left, row.clientWidth),
      );
    };
    const up = () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  }

  // Real text in both panes (issue #144): each pane shows its text in one textarea that is always
  // there and never swapped for another view, so a selection, the caret and the scroll position
  // stay where they are, and line breaks, blank lines and spaces show exactly as typed or as the
  // engine returned them. Word-level interaction (dictionary, alternatives: #18 / #19) is to build
  // on the textarea's selection. The source textarea is referenced for the undo shortcut's scope
  // (shortcutAction) and for the caret after an undo or redo (showRestored).
  let sourceEl = $state<HTMLTextAreaElement | null>(null);

  // Undo / redo of the source text (issue #118). Kai owns the source pane's undo completely: the
  // browser's own undo of the textarea never sees Clear, swap or a fill, which write the text
  // directly. The history rules (typing runs, steps, caps) live in the pure utils/sourceHistory.ts;
  // this is only the wiring. setSource is the one writer of a change: Clear, swap, the
  // EventInputFill handler and the textarea's input all record through it, and applyUndo /
  // applyRedo are the only other writers of `input`. Undo changes the text only: it never
  // translates, never cancels the open request (#109) and leaves the results and the languages
  // alone; the #81 $effect stores the restored text like any other change. The history is window
  // state, not part of the retained session: it starts empty on every launch (the restored text is
  // the baseline) and survives closing the window, which only hides it.
  let sourceHistory = $state(emptyHistory());

  // The automatic source switch (issue #200): text that arrives in another language than the pinned
  // source shows switches the source select to it, makes the old source the target and translates.
  // The cue remembers the pair before the switch, so the swap button can undo it, and shows the
  // note while the window still is in the pair the switch made. Neither is persisted or taught.
  let switchCue = $state<SwitchCue | null>(null);
  // "Correct grammar and wording" (issue #208). The checkbox is the setting correct_source_text
  // (OFF by default, and only an explicit true turns it on), remembered in settings.json like the
  // auto-clipboard toggle. Whether the correction can run at all is the backend's answer
  // (correctAvail): unavailable disables the checkbox, and nothing is asked of the backend then.
  // `correction` is what the last arrival was corrected to (the text on screen is never replaced:
  // the source pane keeps what the user gave), `usedCorrection` the correction the request now in
  // the result pane was actually sent with, which is what the result pane's note describes, and
  // `correcting` is true while the model works. None of the three is ever persisted or taught.
  let correctEnabled = $state(false);
  let correctAvail = $state<{ available: boolean; reason: string }>({
    available: true,
    reason: '',
  });
  let correcting = $state(false);
  let correction = $state<AppliedCorrection | null>(null);
  let usedCorrection = $state<AppliedCorrection | null>(null);
  const correctionOn = $derived(correctEnabled && correctAvail.available);
  // The correction the result pane's note describes: only while a result is showing, and only while
  // the source text is still the one that was corrected.
  const noteCorrection = $derived(
    pane === 'result' && correctionShown(usedCorrection, input) ? usedCorrection : null,
  );
  const correctTip = $derived(
    correctAvail.available
      ? t('translate.correctSourceHint')
      : t(unavailableKey(correctAvail.reason)),
  );
  // The request whose result already made the engine-detection retry (translateIfSwitched), so one
  // request retries at most once.
  let hintedRequestId = '';

  // The one writer of a change to the source text: records it (the text before it is `input`),
  // then applies it. Both time rules of the history (the typing window here and the shortcut
  // dedupe below) read the same clock, Date.now().
  function setSource(next: string, kind: ChangeKind) {
    sourceHistory = recordChange(sourceHistory, input, next, kind, Date.now());
    input = next;
  }

  // Puts a text that undo or redo restores into the source textarea itself, before `input` takes it
  // (issue #144). Any write of a textarea's value leaves the caret at the end of the text, so this
  // one write places it: collapsed at caretAfterRestore (where the undone edit was), with the
  // scroll position kept. Svelte's own write that follows finds the textarea already holding the
  // text and skips it, so the caret stands. It never moves the focus: an undo from the button
  // leaves the focus on the button, and typing resumes at the caret once the user is back in the
  // textarea.
  function showRestored(shown: string, restored: string) {
    const el = sourceEl;
    if (!el) return;
    const caret = caretAfterRestore(shown, restored);
    const top = el.scrollTop;
    el.value = restored;
    el.setSelectionRange(caret, caret);
    el.scrollTop = top;
  }

  // Undo / redo the last step: the restored text goes into the textarea first (showRestored), then
  // into `input`. With nothing to undo or redo, nothing happens.
  function applyUndo() {
    const step = undoStep(sourceHistory, input);
    if (!step) return;
    sourceHistory = step.history;
    showRestored(input, step.text);
    input = step.text;
  }

  function applyRedo() {
    const step = redoStep(sourceHistory, input);
    if (!step) return;
    sourceHistory = step.history;
    showRestored(input, step.text);
    input = step.text;
  }

  // The kind of a user edit of the source textarea, read off its input event: paste, cut, drag and
  // drop are a step of their own; an IME composition never splits on a pause, whether it is still
  // composing or committing (WebKit reports the commit as deleteCompositionText /
  // insertFromComposition, after compositionend); everything else is typing.
  function changeKindOf(e: Event): ChangeKind {
    const { inputType, isComposing } = e as InputEvent;
    if (
      inputType === 'insertFromPaste' ||
      inputType === 'insertFromDrop' ||
      inputType === 'deleteByCut' ||
      inputType === 'deleteByDrag'
    ) {
      return 'paste';
    }
    if (
      isComposing ||
      inputType === 'insertCompositionText' ||
      inputType === 'deleteCompositionText' ||
      inputType === 'insertFromComposition'
    ) {
      return 'compose';
    }
    return 'typing';
  }

  // The source textarea is controlled (value={input} plus this handler, no two-way binding), so
  // every edit reaches setSource. A native undo or redo is cancelled before it happens
  // (onSourceBeforeInput); should one still get through, its text is put back and never recorded.
  function onSourceInput(e: Event) {
    const el = e.currentTarget as HTMLTextAreaElement;
    const { inputType } = e as InputEvent;
    if (inputType === 'historyUndo' || inputType === 'historyRedo') {
      el.value = input;
      return;
    }
    setSource(el.value, changeKindOf(e));
    // "Translate as I type" (issue #57): any edit ends the quiet moment a history write waited for, and
    // starts (or restarts) the wait for the next pause. A paste or a drop translates at once.
    commitAuto.cancel();
    const delay = autoTranslateDelay({
      enabled: $autoTranslateOn,
      text: el.value,
      kind: autoKindOf(inputType, (e as InputEvent).isComposing),
      hasEngines: activeEngines.length > 0,
    });
    if (delay !== null) autoRun.schedule(delay);
    // Nothing to translate yet (too short, blank, or an IME word still being composed): a wait left over
    // from earlier typing must not fire on text that is not final. compositionend restarts it.
    else autoRun.cancel();
    // A paste or a drop puts text into the source pane: an arrival, like the fill. With "Translate as
    // I type" off it is translated only when it switched the pair; with it on, the automatic
    // translation above already does the whole flow (the switch included).
    if ((inputType === 'insertFromPaste' || inputType === 'insertFromDrop') && delay === null) {
      translateIfSwitched(el.value);
    }
  }

  // An IME word is final at compositionend: translate after the usual pause from there.
  function onSourceCompositionEnd(e: Event) {
    const el = e.currentTarget as HTMLTextAreaElement;
    const delay = autoTranslateDelay({
      enabled: $autoTranslateOn,
      text: el.value,
      kind: 'typing',
      hasEngines: activeEngines.length > 0,
    });
    if (delay !== null) autoRun.schedule(delay);
  }

  // A native undo or redo of the source textarea (the Edit menu's key equivalent, a context menu)
  // is always cancelled, so the browser's own undo stack never acts on the source text, and Kai's
  // history is walked instead. One exception keeps one keypress one step: on macOS Wails' default
  // Edit menu binds Cmd+Z as well, so when the window keydown below has just applied the shortcut,
  // this one is only cancelled.
  const SHORTCUT_DEDUPE_MS = 500;
  let shortcutAt = -Infinity;

  function onSourceBeforeInput(e: InputEvent) {
    if (e.inputType !== 'historyUndo' && e.inputType !== 'historyRedo') return;
    e.preventDefault();
    const since = Date.now() - shortcutAt;
    if (since >= 0 && since < SHORTCUT_DEDUPE_MS) return;
    if (e.inputType === 'historyUndo') applyUndo();
    else applyRedo();
  }

  // Leaving the textarea ends the typing run: typing after coming back is a new step.
  function endTypingRun() {
    sourceHistory = breakTyping(sourceHistory);
  }

  // Which history action a key press asks for: Cmd+Z / Ctrl+Z undo, Shift+Cmd+Z / Ctrl+Shift+Z /
  // Ctrl+Y redo, never with Alt; null for any other key. Also null when the press belongs to another
  // editable element than the source textarea: the result textarea keeps its native undo, and a
  // select or any other field keeps its keys. Anywhere else (the source textarea, a button, the
  // page) the shortcut is Kai's, so it still works after Clear, Undo or Swap took the focus.
  function shortcutAction(e: KeyboardEvent): 'undo' | 'redo' | null {
    if (e.altKey || typeof e.key !== 'string') return null;
    const pressed = e.key.toLowerCase();
    let action: 'undo' | 'redo' | null = null;
    if (pressed === 'z' && (e.metaKey || e.ctrlKey)) action = e.shiftKey ? 'redo' : 'undo';
    else if (pressed === 'y' && e.ctrlKey && !e.metaKey && !e.shiftKey) action = 'redo';
    if (action === null) return null;
    const target = e.target;
    if (
      target !== sourceEl &&
      target instanceof HTMLElement &&
      (target.isContentEditable || ['TEXTAREA', 'INPUT', 'SELECT'].includes(target.tagName))
    ) {
      return null;
    }
    return action;
  }

  // The window-level undo / redo shortcut. Every match in scope is cancelled, even with nothing
  // left to undo, so the browser's own undo never runs on the source text. While an IME
  // composition is open the key belongs to the IME.
  function onWindowKeydown(e: KeyboardEvent) {
    if (e.isComposing) return;
    if (handleTranslateShortcut(e)) return;
    if (isClearShortcut(e, clearShortcut)) {
      e.preventDefault();
      clearContext();
      return;
    }
    const action = shortcutAction(e);
    if (action === null) return;
    e.preventDefault();
    shortcutAt = Date.now();
    if (action === 'undo') applyUndo();
    else applyRedo();
  }

  // Cmd+Enter translates (issue #165). Mac-only (metaKey, never ctrlKey, matching the rest of
  // this file's shortcut conventions), and only reachable via the svelte:window listener below,
  // which only fires while this window has focus. Always prevents the default (Enter would
  // otherwise insert a newline in the source textarea); the translate call itself only fires
  // when the Translate button would currently be enabled (matches its disabled={awaiting ||
  // !input.trim()} condition), so a request already in flight is never silently replaced by a
  // stray Cmd+Enter.
  function handleTranslateShortcut(e: KeyboardEvent): boolean {
    if (e.key !== 'Enter' || !e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return false;
    e.preventDefault();
    if (!awaiting && input.trim() && !correcting) translateWithSwitch();
    return true;
  }

  // Translating marquee: animated ellipsis (. → .. → ... → .... cycling)
  let dotCount = $state(0);
  $effect(() => {
    if (!loading && !awaiting) {
      dotCount = 0;
      return;
    }
    const timer = setInterval(() => {
      dotCount = (dotCount + 1) % 4;
    }, 400);
    return () => clearInterval(timer);
  });

  // The elapsed counter of the progress line: a 1 s tick that runs only while a request is open.
  $effect(() => {
    if (!awaiting) return;
    nowMs = Date.now();
    const tick = setInterval(() => {
      nowMs = Date.now();
    }, 1000);
    return () => clearInterval(tick);
  });

  const curLang = $derived(currentLang());

  const activeEngines = $derived(engines.filter((e) => e.kind === 'translate'));

  // Last-used engine (persisted in localStorage, same mechanism as the persisted store's pinKey).
  // The highest-priority source when resolving the result pane's active engine (issue #9; written
  // by this window's engine dropdown, see handleEngineChange).
  const LAST_ENGINE_KEY = 'kai:translate:lastEngine';
  const lastUsedStore = persisted<string>(LAST_ENGINE_KEY, '');
  // settings' primary engine (default_engine): the middle layer of the resolution chain, read on mount.
  let defaultEngine = $state<string>('');
  // Engine list in the GetAllEngines shape (includes enabled/kind/supported): the primary-engine
  // resolution's enabled judgment needs it (GetEngines' EngineListItem has no enabled field).
  let allEngines = $state<AllEngineItem[]>([]);

  // Target-language options: engines like system translation don't support auto-detecting the
  // target language, so the target dropdown must exclude auto.
  const targetLanguages = $derived(languages.filter((l) => l.value !== TRANSLATE_LANG.Auto));

  // The engine the result pane is currently bound to (issue #9): activeEngineFor = #8's
  // resolvedPrimary derivation (last-used ?? primary(default_engine) ?? first-enabled) plus one
  // defensive fallback (falls back to the first engine when resolution is '' but enabled translate
  // engines still exist, guaranteeing the select never dangles). Recomputed whenever the engine
  // list / last-used / settings change; switching engines writes last-used (#8's setLastUsedEngine).
  // activeEngineFor reads the last-used value from localStorage itself, which Svelte cannot track,
  // so $lastUsedStore is read here to make a pick in the dropdown re-derive the active engine
  // (without it the pane kept the old engine and the next render snapped the select back).
  const activeEngine = $derived(
    ($lastUsedStore, activeEngineFor(LAST_ENGINE_KEY, defaultEngine, allEngines)),
  );
  // One dot per enabled translate engine (state = the fan-out's real output: done/pending/failed/
  // cancelled, design §4 and issue #109; an idle window, nothing requested, shows no failed dots:
  // issue #81; the request marker is the run-local one, issue #116).
  const dots = $derived(statusDots(allEngines, results, awaiting, requestedThisRun));
  // Which of the pane's six states applies (issue #81, and cancelled from issue #109): no-engine /
  // loading / result / idle / failed / cancelled. The template's chain reads this one value, so a
  // window that was never asked to translate (idle) can no longer fall into the failed branch.
  const pane = $derived(
    paneState({
      hasEngines: activeEngines.length > 0,
      engine: activeEngine,
      results,
      loading: awaiting,
      requested: requestedThisRun,
    }),
  );
  // The active engine's current result (an engine that has not reported is absent from results → null).
  const activeResult = $derived(activeEngine ? (results[activeEngine] ?? null) : null);
  // The active engine's progress line while it is in flight (issue #109), from what its started and
  // chunk events told this window; null while nothing is known, and the placeholder then keeps its
  // plain loading text.
  const activeLine = $derived(
    activeEngine && progress[activeEngine] ? progressLine(progress[activeEngine], nowMs) : null,
  );
  // The progress line's copy. progressLine decides which line it is and holds the facts; this only
  // words them.
  function progressText(line: ProgressLine): string {
    const engine = engineName(line.engine);
    if (line.kind === 'chunk') {
      return t('translate.part', { done: line.done ?? 0, total: line.total ?? 0 });
    }
    if (line.kind === 'stalled') {
      const { minutes, seconds } = splitElapsed(line.elapsedMs);
      return minutes > 0
        ? t('translate.stillWaitingMin', { engine, minutes, seconds })
        : t('translate.stillWaiting', { engine, seconds });
    }
    return t('translate.workingOn', { engine, time: formatClock(line.elapsedMs) });
  }
  // A status dot's tooltip and accessible label: the engine name, plus its state once it has one
  // (an idle dot has nothing to report, so its label is the engine name alone). A failed engine that
  // sent a payload says why with the failure headline (issue #96); one that sent none keeps the bare
  // "Failed".
  function dotLabel(engine: string, st: DotState, failedHeadline?: string): string {
    const state =
      st === 'done'
        ? t('translate.engineDone')
        : st === 'pending'
          ? t('translate.enginePending')
          : st === 'failed'
            ? (failedHeadline ?? t('translate.engineFailed'))
            : st === 'cancelled'
              ? t('translate.engineCancelled')
              : '';
    return engineName(engine) + (state ? ' · ' + state : '');
  }
  // The text the active engine is showing / can show: manual edit ?? engine result ?? empty string.
  const activeDisplay = $derived(edited.get(activeEngine) ?? activeResult?.result ?? '');
  // The active engine's failure, ready to render (issue #96): headline, muted detail and optional
  // action, from the one failureMessage the dot tooltip and the screenshot card also read. Only the
  // failed pane uses it. An engine that sent no payload (it reported nothing) gets the bare generic
  // headline and nothing else; only the two credential kinds (not configured, key rejected) carry
  // the Settings action.
  const failure = $derived(failureMessage(activeResult, t, engineName(activeEngine)));
  // The result-pane engine dropdown's display value: activeEngine already includes the
  // "'' → first enabled" defensive fallback, so it is never empty while activeEngines is non-empty
  // (the select never points at nothing).
  const firstEnabledName = $derived(activeEngines[0]?.value ?? '');
  const selectValue = $derived(activeEngine || firstEnabledName);
  // The source language the active engine's result reports (its From: the pin, or the language the
  // engine detected, issue #53). The swap below reads it (issue #13).
  const detectedFrom = $derived(String(activeResult?.from ?? ''));
  // The source select's option labels (issue #161): the Auto entry always reads "Detected", whatever
  // the result, and every other entry its language name. The select's labels never depend on a
  // result; the language a text was actually translated from is the result pane's note. (A pinned
  // source does change when text arrives in another language, but that is the automatic switch of
  // issue #200, decided before the request goes out; it is not a relabel of the result.)
  function fromOptionLabel(value: string): string {
    return value === TRANSLATE_LANG.Auto ? t('translate.sourceAuto') : langName(value);
  }
  // The label of the pane's per-engine Cancel (issue #109), worded here so the pane markup names no
  // engine (issue #95: the dropdown above does).
  const cancelActiveLabel = $derived(
    t('translate.cancelEngine', { engine: engineName(activeEngine) }),
  );
  // The pair the swap button would apply (issue #13), or null when there is nothing to exchange:
  // the source is auto and the active engine detected nothing the target select can hold. One
  // derivation feeds both the button's disabled state and swap(), so the two cannot disagree.
  // isSelectable composes the two layers of "can be picked as a target": the loaded language list
  // (bare es / pt are recognized but not in it) and the issue #52 capability gate, so a detection
  // never lands the target on an option the select renders disabled.
  // Whether the note of the last automatic switch still describes the window (issue #200).
  const cueShown = $derived(cueActive(switchCue, input, { from: fromLang, to: toLang }));
  const swapPair = $derived(
    swapLanguages({
      from: fromLang,
      to: toLang,
      detectedFrom,
      autoCode: TRANSLATE_LANG.Auto,
      isSelectable: (code) =>
        targetLanguages.some((l) => l.value === code) && !isTargetDisabled(allEngines, code),
      options: targetLanguages.map((l) => l.value),
    }),
  );
  // Result-pane manual edits (aggregated by engine name): discarded wholesale on engine switch /
  // retranslate / clearing input; a new engine always starts from its own stored result
  // (no per-engine edit memory, design §3).
  let edited = $state<Map<string, string>>(new Map());
  function setEdited(engine: string, value: string) {
    edited = new Map(edited).set(engine, value);
  }
  function handleEngineChange(ev: Event) {
    const name = (ev.currentTarget as HTMLSelectElement).value;
    setLastUsedEngine(name);
    // Engine switch: discard the previous engine's manual edit; the new engine starts from its own stored result.
    edited = resetEdits(edited, activeEngine, name, results);
  }

  onMount(() => {
    // Event listeners must register first (so a result arriving during the awaited loads isn't lost).
    const offResult = onEvent(EventTranslateResult, (payload: TranslateResult) => {
      if (payload && payload.engine) {
        // A result of another request (an older one that a newer translate or a Clear replaced) is
        // not this window's business.
        if (payload.request_id !== requestId) return;
        results = { ...results, [payload.engine]: payload };
        loading = false;
        // The engine told the text is in another language than the pinned source (issue #161) and
        // no switch was made before the request went out, which is what happens where there is no
        // local detector: retry through the same switch, once per request.
        if (payload.detected_from && !payload.identity && hintedRequestId !== requestId) {
          hintedRequestId = requestId;
          translateIfSwitched(input, payload.detected_from);
        }
        // The request settles only when every engine the backend started has reported (result,
        // failure or cancel): until then the active engine keeps its loading placeholder even if a
        // faster sibling answered first.
        if (requestSettled(started, results)) awaiting = false;
      }
    });
    // The non-terminal facts of a running engine: its start and, later, its chunk progress. Only
    // events of the request this window is waiting on count; the event is a broadcast.
    const offProgress = onEvent(EventTranslateProgress, (payload: TranslateProgressPayload) => {
      if (!payload || !payload.engine || payload.request_id !== requestId) return;
      const now = Date.now();
      if (payload.phase === 'chunk') {
        const current = progress[payload.engine] ?? startProgress(payload.engine, now);
        progress = {
          ...progress,
          [payload.engine]: applyChunk(current, { done: payload.done, total: payload.total }, now),
        };
      } else {
        progress = { ...progress, [payload.engine]: startProgress(payload.engine, now) };
      }
    });
    const offInputFill = onEvent(EventInputFill, (text: string) => {
      if (!text) return;
      // Its own undo step (issue #118): Undo brings back the text the fill replaced.
      setSource(text, 'program');
      // A new arrival is a new text: an undo of an earlier switch does not carry over to it.
      switcher.newArrival();
      translateWithSwitch();
    });
    // Issue #175 item 5: the copy-key branch simulated a copy but never saw the clipboard
    // change, so nothing arrived via EventInputFill above. Without this, the window still
    // comes to the front showing whatever text was already there (issue #81's retained
    // session) — indistinguishable from that old text being the actual new selection. Show a
    // toast so a failed capture is visibly a failure, not a silent stale "success".
    const offCopyKeyFailed = onEvent(EventCopyKeyFailed, () => {
      showToast(t('translate.copyKeyFailed'), 3200);
    });
    // Issue #199: "translate on double Cmd+C" is on but Input Monitoring is missing. Say what to
    // enable and where, instead of a feature that silently does nothing.
    const offDoubleCopyPermission = onEvent(EventDoubleCopyPermissionMissing, () => {
      showToast(t('translate.doubleCopyPermission'), 9000);
    });
    // Issue #194: the capture found the Accessibility permission missing, so no key was sent. Say
    // what to enable and where, instead of a failure that looks like "nothing selected".
    const offAccessibilityMissing = onEvent(EventAccessibilityMissing, () => {
      showToast(t('translate.accessibilityMissing'), 12000);
    });
    const offClosing = onEvent(EventWindowClosing, (name: string) => {
      // Issue #69: opening Settings drops this window out of always-on-top so Settings is not
      // hidden behind a pinned window; when Settings closes, put the persisted pin back.
      if (name === WindowSettings) {
        Window.SetAlwaysOnTop($pinnedStore).catch((e) =>
          console.error(t('log.restorePinFailed'), e),
        );
        return;
      }
      // Global broadcast: only this window's (translate) closing is looked at, so closing another
      // window never touches the translation.
      if (name !== WindowTranslate) return;
      // A pending history write for the text on screen is made now, not lost with the timer (issue #57).
      commitAuto.cancel();
      commitAutoNow();
      // Issue #81: closing the translate window used to clear the text and the results here. It
      // no longer clears anything: the session (text, results, request marker; the languages
      // persist on their own) is retained, so reopening the window shows the last translation.
      // It is replaced only by Clear, a new EventInputFill or a new translate. The request is
      // left alone too (issue #109): closing the window does not cancel it, results still in
      // flight keep landing in the retained session and settle it, and the user can still Cancel it
      // after reopening, so a quick reopen is not turned into a failed pane.
    });
    // Broadcast after engines are added/removed or enabled/disabled in settings: re-fetch the
    // engine list so the translate window's result pane syncs to the latest state (otherwise
    // enabled/disabled engines never refresh and the old list sticks).
    const offEngines = onEvent(EventEnginesChanged, () => {
      loadEngines();
    });
    // The settings page saved a new clear-context shortcut (issue #48): use it from now on.
    const offClearShortcut = onEvent(EventClearContextShortcutChanged, (v) => {
      if (typeof v === 'string') clearContextShortcut.set(v);
    });
    // Initialization and first render: restore pin state, then wait for engines/languages/defaults to load.
    (async () => {
      // Restore the persisted pin state.
      try {
        await Window.SetAlwaysOnTop($pinnedStore);
      } catch (e) {
        console.error(t('log.restorePinFailed'), e);
      }
      // Must wait for engines/languages/defaults to load first (the result pane's dropdown and dots depend on them).
      await Promise.all([
        loadDefaults(),
        loadEngines(),
        loadLanguages(),
        loadCorrectionAvailability(),
      ]);
      // Load the "auto-read clipboard" toggle + primary engine (both persisted in settings.json).
      // default_engine is the middle layer of the primary-engine resolution chain (last-used ?? primary ?? first-enabled).
      try {
        const cfg = await GetConfig();
        if (cfg?.auto_clipboard) {
          autoClipboard = true;
        }
        correctEnabled = cfg?.correct_source_text === true;
        if (cfg?.default_engine) {
          defaultEngine = cfg.default_engine;
        }
      } catch (e) {
        console.error(t('log.autoClipboardLoadFailed'), e);
      }
    })();
    return () => {
      offResult();
      offProgress();
      offInputFill();
      offCopyKeyFailed();
      offDoubleCopyPermission();
      offAccessibilityMissing();
      offClosing();
      offEngines();
      offClearShortcut();
    };
  });

  // Read default source/target languages from the settings file as initial values (fall back to auto/zh when unset).
  async function loadDefaults() {
    try {
      const cfg = await GetConfig();
      if (cfg?.default_from) fromLang = cfg.default_from as TranslateLang;
      if (cfg?.default_to) toLang = cfg.default_to as TranslateLang;
    } catch (e) {
      console.error(t('log.readDefaultLangFailed'), e);
    }
  }

  // Persist the current source/target languages to the settings file.
  async function persistLangs() {
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, default_from: fromLang, default_to: toLang });
    } catch (e) {
      console.error(t('log.persistLangPrefFailed'), e);
    }
  }

  // A language picked in either select (issue #53): teach the backend's variant-preference store,
  // then persist the choice as before. Picking es-MX once makes every later auto-detected Spanish
  // come back qualified as es-MX for this window session; the backend ignores bases and languages
  // without dialects, so every pick is passed through as it is.
  // Called from the selects' own onchange ONLY: swap() and loadDefaults() assign fromLang/toLang
  // directly and must never teach (an explicit choice is the only signal — "swap consumes, never
  // teaches"). Reads the picked value off the event so it doesn't depend on bind:value ordering.
  function onLangPicked(ev: Event) {
    // A correction was made under the pair the window had: another language is a new question.
    correction = null;
    switcher.resetCorrection();
    learnLangVariant((ev.currentTarget as HTMLSelectElement).value as TranslateLang);
    persistLangs();
  }

  async function learnLangVariant(lang: TranslateLang) {
    await learnFromSelection(lang, LearnLangVariant, (e) =>
      console.error(t('log.learnLangVariantFailed'), e),
    );
  }

  const fallbackLanguages = $derived<NamedItem[]>(
    ALL_TRANSLATE_LANGS.map((c) => ({
      value: c,
      name: langName(c),
    })),
  );

  async function loadEngines() {
    // GetEngines: result-pane rendering (no enabled field; only translate/ocr + supported).
    // GetAllEngines: the enabled judgment for primary-engine resolution (includes enabled/kind/supported).
    // Both fetched in parallel to avoid a second round trip; on failure each falls back independently, never blocking the other.
    try {
      const [list, all] = await Promise.all([GetEngines(), GetAllEngines()]);
      engines = list ?? [];
      allEngines = all ?? [];
    } catch (e) {
      console.error(t('log.loadEngineListFailed'), e);
      engines = [];
      allEngines = [];
    }
  }

  // Records the "last-used engine" to localStorage (last-used is the highest-priority source in
  // the primary-engine resolution chain).
  // Called by the result pane's engine selector (a later issue): when the user switches to an
  // engine, setLastUsedEngine(it) runs, and after reopening the window the first render binds
  // directly to it (if that engine is still enabled).
  function setLastUsedEngine(name: string) {
    lastUsedStore.set(name);
  }

  async function loadLanguages() {
    try {
      const list = await GetLanguages(curLang);
      languages = list?.length ? list : fallbackLanguages;
    } catch (e) {
      console.error(t('log.loadLangListFailed'), e);
      languages = fallbackLanguages;
    }
  }

  // The automatic source switch (issue #200), for every way text arrives. The flow lives in
  // sourceSwitchFlow.ts (pure, tested by running it); this only supplies what it needs. Assigning
  // the two selects is all it does to them: an automatic switch is not a choice, so nothing is
  // taught to the variant store and nothing is persisted (a select's own onchange does that).
  const switcher = createSourceSwitcher({
    plan: (req) =>
      PlanSourceSwitch({
        text: req.text,
        from: req.from as TranslateLang,
        to: req.to as TranslateLang,
        detected: req.detected as TranslateLang,
      }),
    getText: () => input,
    getPair: () => ({ from: fromLang, to: toLang }),
    setPair: (p) => {
      fromLang = p.from as TranslateLang;
      toLang = p.to as TranslateLang;
    },
    autoCode: TRANSLATE_LANG.Auto,
    sourceOptions: () => languages.map((l) => l.value),
    canBeTarget: (code) =>
      targetLanguages.some((l) => l.value === code) && !isTargetDisabled(allEngines, code),
    translate: () => doTranslate(),
    onCue: (c) => (switchCue = c),
    onError: (e) => console.error(t('log.sourceSwitchFailed'), e),
    // Issue #16: a miss is never silent; the backend logs its own decision line, and the window's own
    // reason is sent to the backend too so it lands in the same main log (the frontend log file was
    // empty in practice).
    onSkip: (reason) => {
      console.info(t('log.sourceSwitchSkipped', { reason }));
      void Promise.resolve(ReportSourceSwitchSkipped(reason)).catch(() => {});
    },
    // The correction step (issue #208): every arrival is corrected first, and the rest of the flow
    // goes on with the corrected text.
    correct: (req) =>
      CorrectSource({
        text: req.text,
        from: req.from as TranslateLang,
        detected: req.detected as TranslateLang,
      }),
    isCorrectionEnabled: () => correctionOn,
    onCorrection: (c) => (correction = c),
    onCorrecting: (busy) => (correcting = busy),
  });

  // Whether the correction can run on this Mac (the backend's answer), read once the window is up.
  async function loadCorrectionAvailability() {
    try {
      const a = await CorrectionAvailability();
      correctAvail = { available: a?.available === true, reason: a?.reason ?? '' };
    } catch (e) {
      console.error(t('log.correctAvailabilityFailed'), e);
      correctAvail = { available: false, reason: 'unavailable' };
    }
  }

  // The checkbox: saved through SaveConfig with the rest of the config, like the auto-clipboard
  // toggle. It changes what later arrivals do; what is on screen stays as it is. Nothing is taught.
  async function toggleCorrect(e: Event) {
    const next = (e.currentTarget as HTMLInputElement).checked;
    correctEnabled = next;
    switcher.resetCorrection();
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, correct_source_text: next });
    } catch (err) {
      console.error(t('log.generalSaveCorrectFailed'), err);
    }
  }

  // The result pane's one button: translate the text as it came instead of its correction.
  function translateOriginal() {
    switcher.useOriginal();
    doTranslate();
  }

  // Translate the source text, switching the pair first when the text is in another language: the
  // Translate button, Cmd+Enter and the fill (hotkey, tray, auto-clipboard) come here.
  function translateWithSwitch() {
    // A translation the user asked for is not an automatic one, and replaces a pending wait.
    autoRun.cancel();
    nextAuto = false;
    return switcher.translateWithSwitch();
  }

  // "Translate as I type" (issue #57). The automatic translation is the Translate button's own path,
  // flagged so the backend holds its history row until the text has stopped changing.
  let nextAuto = false;
  const autoRun = createAutoScheduler(() => {
    nextAuto = true;
    void switcher.translateWithSwitch();
  });
  // The quiet moment after an automatic translation: the history write for the text still on screen.
  // `autoCommit` names the automatic request waiting for it.
  let autoCommit = $state<{ id: string; text: string } | null>(null);
  const commitAuto = createAutoScheduler(commitAutoNow);
  function commitAutoNow() {
    const c = autoCommit;
    if (!c) return;
    if (!commitStillValid({ requestText: c.text, currentText: input, current: requestId === c.id }))
      return;
    autoCommit = null;
    void Promise.resolve(CommitAutoHistory(c.id)).catch((e) =>
      console.error(t('log.translateRequestFailed'), e),
    );
  }
  $effect(() => {
    // Once an automatic request has settled, wait for a quiet moment before the history write.
    const waiting = autoCommit !== null && !awaiting;
    untrack(() => {
      if (waiting) commitAuto.schedule(AUTO_COMMIT_DELAY_MS);
      else commitAuto.cancel();
    });
  });

  // A paste, or a result that arrived after the request went out: translate only when it switched.
  function translateIfSwitched(text: string, detected = '') {
    return switcher.translateIfSwitched(text, detected);
  }

  // Swap the two languages and translate back (issue #13, DeepL style). swapPair already holds the
  // exchanged pair (the old target becomes the source, an auto source is replaced by the language
  // that was detected, so auto never survives) or is null when there is nothing to exchange; the
  // button is disabled then and this is a no-op. The text on screen for the active engine (manual
  // edit ?? result) becomes the new source text and the old source text is dropped; with no result
  // yet the input stays as it is. The existing doTranslate() then clears results and edits and sends
  // the request with the new pair, so requestedTo is set there like for any other request. A swap
  // consumes preferences and never writes them: the pair is persisted, but only a select's own
  // onchange ever teaches the variant store. The new source text is its own undo step (issue #118):
  // Undo brings back the old source text and leaves the languages swapped.
  function swap() {
    // Always a real swap, also while the note of an automatic switch (issue #200) is showing: undoing
    // that switch put the old pair back around the unchanged text, leaving each pane in a language
    // other than its dropdown's (issue #39).
    const pair = swapPair;
    if (!pair) return;
    // A swap translates the text on screen in the other direction: the correction (made for the
    // language it had) does not go with it.
    correction = null;
    const { from, to } = pair;
    fromLang = from as TranslateLang;
    toLang = to as TranslateLang;
    if (activeDisplay !== '') setSource(activeDisplay, 'program');
    persistLangs();
    doTranslate();
  }

  async function doTranslate() {
    // Whatever called this started the translation: a pending wait is over, and the flag says whether
    // it was the automatic one (issue #57).
    const auto = nextAuto;
    nextAuto = false;
    autoRun.cancel();
    commitAuto.cancel();
    autoCommit = null;
    if (!input.trim() || activeEngines.length === 0) return;
    loading = true;
    awaiting = true;
    // Something is now being asked for: from here on, no result means failed, not idle (issue #81).
    requested = true;
    requestedThisRun = true;
    requestedTo = toLang;
    results = {};
    // The engines' own translations replace any retranslation shown for the previous round.
    retrans = null;
    // A new fan-out round starts blank: the previous batch's edits are meaningless for the new
    // round and are discarded with it.
    edited = new Map();
    // The request is named here, before the backend is called (issue #109). A request that is
    // still open is replaced: the backend cancels it silently (a hotkey fill during a running
    // translation does this), and its late events are ignored because they carry the old id.
    // The text that goes out is the corrected one while a correction applies to the text on screen
    // (issue #208); the result pane's note describes exactly the correction used, and no other.
    const sendText = textToSend(correction, input);
    usedCorrection = correctionShown(correction, input) ? correction : null;
    requestId = newRequestID();
    const id = requestId;
    if (auto) autoCommit = { id, text: input };
    started = null;
    progress = {};
    try {
      // Multi-engine concurrency is handled in parallel by the backend across enabled engines, independent of any single engine;
      // the bindings-generated TranslateRequest.engine is required, so pass an empty string to satisfy the type (the backend ignores it).
      const res = await TranslateMulti({
        text: sendText,
        from: fromLang as TranslateLang,
        to: toLang as TranslateLang,
        engine: '',
        request_id: requestId,
        auto,
      });
      // A newer request replaced this one while the call was pending: it is not this request's
      // business any more.
      if (requestId !== id) return;
      // The call says which engines it started. Results arrive asynchronously one by one via
      // EventTranslateResult, and the request settles when each of these has reported (possibly
      // already, when the fast ones answered before the call returned). No engines started: settled.
      started = res?.engines ?? [];
      if (requestSettled(started, results)) awaiting = false;
      if (started.length === 0) loading = false;
      // A kept context applies to this text too (issue #48).
      if (hasContext(chat)) void runContext('');
    } catch (e) {
      // A rejected call started nothing, so the request is over at once; no timer is involved.
      console.error(t('log.translateRequestFailed'), e);
      if (requestId === id) {
        awaiting = false;
        loading = false;
      }
    }
  }

  // The Cancel controls (issue #109) only ask the backend to stop. The window's wait ends the way
  // it ends for any other outcome: each cancelled engine reports one result (cancelled), and the
  // request settles when all of them have.
  function reportCancelFailure(e: unknown) {
    console.error(t('log.translateCancelFailed'), e);
  }
  // Cancel the whole request.
  function cancelRequest() {
    CancelTranslate(requestId, '').catch(reportCancelFailure);
  }
  // Cancel one engine of the request; the others keep running.
  function cancelEngine(engine: string) {
    CancelTranslate(requestId, engine).catch(reportCancelFailure);
  }

  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout> | undefined;
  function showToast(msg: string, durationMs = 1600) {
    toast = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = ''), durationMs);
  }

  async function copy(text: string) {
    if (!text) return;
    try {
      await Clipboard.SetText(text);
      showToast(t('common.copied'));
    } catch (e) {
      console.error('copy failed', e);
    }
  }

  // The back-translation (issue #56). With the toggle on, the displayed result is translated back
  // into the source language and shown under it, to check it in the user's own language. The backend
  // (BackTranslate) does it with the engine whose result is shown: its own request, cancellable, no
  // history, never an error. What is on screen is bound to the exact displayed text and engine, so
  // an edit, a new text or an engine switch can never leave it describing something else.
  let back = $state<BoundBack | null>(null);
  let backBusy = $state(false);
  let backRequestId = '';
  // The text and engine a request was last made for: a failed or empty answer is not asked again for
  // the same pair, and an effect run that changes nothing never starts a second request.
  let backAsked: { displayed: string; engine: string } | null = null;
  const backShown = $derived(backShownFor(back, activeDisplay, activeEngine));

  function stopBack() {
    if (backRequestId !== '') CancelTranslate(backRequestId, '').catch(reportCancelFailure);
    backRequestId = '';
    backBusy = false;
    backAsked = null;
    back = null;
  }

  async function startBack(displayed: string, engine: string, pair: { from: string; to: string }) {
    if (backRequestId !== '') CancelTranslate(backRequestId, '').catch(reportCancelFailure);
    const id = newRequestID();
    backRequestId = id;
    backAsked = { displayed, engine };
    backBusy = true;
    let ans: BackAnswer | null = null;
    try {
      ans = (await BackTranslate(
        buildBackRequest({ text: displayed, pair, engine, requestId: id }) as any,
      )) as BackAnswer | null;
    } catch (e) {
      console.error(t('log.translateRequestFailed'), e);
    }
    // Replaced, cancelled or stopped while the call was pending: not this request's business.
    if (backRequestId !== id) return;
    backBusy = false;
    back = acceptBack(ans, displayed, engine, pair.to);
  }

  $effect(() => {
    const wanted = shouldBackTranslate({
      enabled: $backTranslateOn,
      displayed: activeDisplay,
      isResult: pane === 'result',
      failed: Boolean(activeResult?.error),
      cancelled: Boolean(activeResult?.cancelled),
      identity: Boolean(activeResult?.identity),
    });
    const displayed = activeDisplay;
    const engine = activeEngine;
    const res = activeResult;
    untrack(() => {
      if (!wanted || !res) {
        if (backRequestId !== '' || back !== null || backAsked !== null) stopBack();
        return;
      }
      if (!needsBack(back, displayed, engine)) return;
      if (backAsked && backAsked.displayed === displayed && backAsked.engine === engine) return;
      const pair = backPair({
        resultFrom: String(res.from ?? ''),
        resultTo: String(res.to ?? ''),
        detectedFrom: String(res.detected_from ?? ''),
      });
      if (!pair) {
        if (backRequestId !== '' || back !== null) stopBack();
        return;
      }
      void startBack(displayed, engine, pair);
    });
  });

  // The context chat (issue #48). The context the user gave is kept across texts until it is
  // cleared (the button, or the shortcut): nothing below is reset when the text or the pair
  // changes. The backend decides which engine answers (the selected one when it can follow a
  // context, else the on-device model, else a configured cloud LLM) and whether anything can.
  let chat = $state<ContextChat>(emptyChat);
  let chatOpen = $state(false);
  let chatBusy = $state(false);
  let chatRequestId = '';
  // The retranslation shown in place of the engine's own result, bound to the text and engine it
  // was made for.
  let retrans = $state<BoundRetranslation | null>(null);
  const clearShortcut = $derived($clearContextShortcut);
  const shownRetrans = $derived(shownFor(retrans, input, activeEngine));
  // The sentence under the chat when another engine than the selected one did the retranslation.
  const retransNote = $derived(
    shownRetrans?.fallback
      ? t('translate.contextFallback', {
          engine:
            shownRetrans.engine === 'apple-foundation-models'
              ? t('translate.contextOnDevice')
              : engineName(shownRetrans.engine),
          selected: engineName(activeEngine),
        })
      : '',
  );

  // Translates the text on screen again in the context in force. `previous` is the translation the
  // user says is wrong ('' when there is none yet: a new text translated under a kept context).
  async function runContext(previous: string) {
    if (!hasContext(chat) || !input.trim()) return;
    const id = newRequestID();
    chatRequestId = id;
    chatBusy = true;
    const forInput = input;
    const forEngine = activeEngine;
    const req = buildContextRequest(chat, {
      text: textToSend(correction, input),
      from: fromLang,
      to: toLang,
      engine: forEngine,
      previous,
      requestId: id,
    });
    let ans: ContextAnswer;
    try {
      ans = (await RetranslateWithContext(req as any)) as ContextAnswer;
    } catch (e) {
      console.error(t('log.translateRequestFailed'), e);
      ans = {
        status: 'failed',
        result: '',
        engine: '',
        fallback: false,
        reason: '',
        error: '',
        request_id: id,
      };
    }
    // Cleared, or replaced by a newer one, while the call was pending: not this request's business.
    if (chatRequestId !== id) return;
    chatBusy = false;
    const noteKey = answerNoteKey(ans);
    const applied = applyAnswer(chat, ans, noteKey ? t(noteKey) : '');
    chat = applied.chat;
    if (applied.shown) {
      retrans = { ...applied.shown, forInput, forEngine };
      // The retranslation takes the place of the engine's result the way a manual edit does, so
      // the pane, Copy and the engine switch treat it exactly like one.
      if (forEngine === activeEngine) edited = new Map(edited).set(forEngine, applied.shown.text);
    }
  }

  function sendContext(message: string) {
    chat = addUserMessage(chat, message);
    void runContext(activeDisplay);
  }

  // Back to the engine's own translation; the context stays.
  function showOriginal() {
    const next = new Map(edited);
    next.delete(activeEngine);
    edited = next;
    retrans = null;
  }

  function cancelContext() {
    CancelTranslate(chatRequestId, '').catch(reportCancelFailure);
  }

  // Clear context: the button and the shortcut. Forgets the context and the chat, stops a request
  // in flight, and goes back to the engine's own translation.
  function clearContext() {
    if (chatBusy) cancelContext();
    chatRequestId = '';
    chatBusy = false;
    chat = clearChat();
    // The retranslation is shown through the edited map: take it out of there too.
    edited = dropRetranslation(edited, retrans);
    retrans = null;
  }

  // Back to the idle window (issue #81): the four retained fields (text, results, requested target,
  // requested marker) are reset here and the $effect above stores the empty session; there is no
  // second write to storage. A request that is still open is abandoned (issue #109): the backend
  // stops it, so nothing keeps running unseen, and its late events are ignored, so they cannot
  // bring the cleared window back. The cleared text is its own undo step (issue #118): Undo brings
  // the text back, and the result pane stays idle until the user translates again.
  function clearInput() {
    autoRun.cancel();
    commitAuto.cancel();
    autoCommit = null;
    if (awaiting) cancelRequest();
    requestId = '';
    started = null;
    progress = {};
    setSource('', 'program');
    correction = null;
    usedCorrection = null;
    results = {};
    requestedTo = '';
    requested = false;
    requestedThisRun = false;
    awaiting = false;
    loading = false;
    edited = new Map();
  }
</script>

<!-- Undo / redo of the source text from the keyboard (issue #118), at window level so it still
     works when the focus is on a button; shortcutAction decides what is in scope. -->
<svelte:window onkeydown={onWindowKeydown} />

<div class="u-surface flex h-screen flex-col" style={rootStyleToStyle($rootStyle)}>
  <main class="flex h-full min-h-0 flex-col gap-4 overflow-hidden p-4">
    <!-- Toolbar row: spans both panes (from/swap/to apply to the whole translation; DeepL-style
         layout). Three columns with equal 1fr sides keep the from/swap/to group centered while
         the Settings gear (issue #69) sits alone in the right column, at the row's right end. -->
    <div class="grid grid-cols-[1fr_auto_1fr] items-center gap-2">
      <!-- "Correct grammar and wording" (issue #208): the left column of the toolbar row, which was
           empty. Off by default, remembered like the other settings, and disabled with the reason
           as its tooltip when the on-device model cannot run here. The tooltip is the pane
           headers' (#165) u-tooltip, opening downward and anchored at its left edge (the label sits
           at the window's edge, where a centred tooltip would be clipped). -->
      <div class="col-start-1 flex flex-col items-start gap-1 justify-self-start">
        <label
          class="u-tooltip u-tooltip--start flex items-center gap-2 text-xs"
          class:u-muted={!correctionOn}
          data-tooltip={correctTip}
        >
          <input
            type="checkbox"
            class="u-no-drag"
            data-testid="correct-source-checkbox"
            checked={correctionOn}
            disabled={!correctAvail.available}
            aria-describedby="correct-source-tip"
            onchange={toggleCorrect}
          />
          <span>{t('translate.correctSource')}</span>
          <span id="correct-source-tip" class="sr-only">{correctTip}</span>
        </label>
        <!-- "Show a back-translation" (issue #56): the result translated back into the source
           language, to check it. Off by default; it doubles the translation calls. -->
        <label
          class="u-tooltip u-tooltip--start flex items-center gap-2 text-xs"
          class:u-muted={!$backTranslateOn}
          data-tooltip={t('translate.backTranslateTip')}
        >
          <input
            type="checkbox"
            class="u-no-drag"
            data-testid="back-translate-checkbox"
            aria-describedby="back-translate-tip"
            checked={$backTranslateOn}
            onchange={(e) => backTranslateOn.set(e.currentTarget.checked)}
          />
          <span>{t('translate.backTranslate')}</span>
          <span id="back-translate-tip" class="sr-only">{t('translate.backTranslateTip')}</span>
        </label>
        <!-- "Translate as I type" (issue #57): translate by itself when the user stops typing or pastes.
             On by default; turn it off to translate only with the button or Cmd+Enter. -->
        <label
          class="u-tooltip u-tooltip--start flex items-center gap-2 text-xs"
          class:u-muted={!$autoTranslateOn}
          data-tooltip={t('translate.autoTranslateTip')}
        >
          <input
            type="checkbox"
            class="u-no-drag"
            data-testid="auto-translate-checkbox"
            aria-describedby="auto-translate-tip"
            checked={$autoTranslateOn}
            onchange={(e) => {
              autoTranslateOn.set(e.currentTarget.checked);
              if (!e.currentTarget.checked) autoRun.cancel();
            }}
          />
          <span>{t('translate.autoTranslate')}</span>
          <span id="auto-translate-tip" class="sr-only">{t('translate.autoTranslateTip')}</span>
        </label>
      </div>
      <div class="col-start-2 flex items-center justify-center gap-2">
        <select
          class="u-field u-select u-lang-select px-3 py-2 text-sm"
          bind:value={fromLang}
          onchange={onLangPicked}
          aria-label={t('translate.from')}
        >
          {#each languages as l}
            <option value={l.value}>{fromOptionLabel(l.value)}</option>
          {/each}
        </select>

        <!-- Swap (issue #13): disabled while there is nothing to exchange (source auto and nothing
             usable detected yet); enabled whenever the source is pinned. Label and tooltip stay
             the same either way. -->
        <button
          class="u-icon-btn u-no-drag"
          onclick={swap}
          disabled={swapPair === null}
          aria-label={t('translate.swap')}
          title={t('translate.swap')}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M7 10h14l-4-4" />
            <path d="M17 14H3l4 4" />
          </svg>
        </button>

        <select
          class="u-field u-select u-lang-select px-3 py-2 text-sm"
          bind:value={toLang}
          onchange={onLangPicked}
          aria-label={t('translate.to')}
        >
          {#each targetLanguages as l}
            <!-- issue #52: a target no enabled engine can translate into is shown disabled;
                 the capability comes from the backend (allEngines[].target_languages), never a
                 frontend map. The source select above is never gated (every engine accepts every
                 source). -->
            <option value={l.value} disabled={isTargetDisabled(allEngines, l.value)}>
              {langName(l.value)}
            </option>
          {/each}
        </select>
      </div>

      <!-- Settings gear (issue #69): app-level configuration, so it sits at the toolbar's right end,
           outside both cards. Opens the Settings window through the existing WindowWrapper
           binding (ShowSettings, no new Go API); this window stays open. Not a toggle, so it never
           gets the --active styling. Glyph: Lucide "settings" (ISC), inline like the swap
           button's. Label reuses the titlebar.settings key (issue #69); the tooltip (issue #173)
           is the more descriptive titlebar.settingsHint, distinct so the error-panel's visible
           "Settings" button text (below) stays short. -->
      <div class="col-start-3 flex justify-end">
        <button
          class="u-icon-btn u-no-drag"
          onclick={() => ShowSettings()}
          aria-label={t('titlebar.settings')}
          title={t('titlebar.settingsHint')}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path
              d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"
            />
            <circle cx="12" cy="12" r="3" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Two-pane row (issue #10, locked decision: always side by side, no stacked fallback):
         left = source text, right = results; the divider between panes is draggable (ratio
         persists to localStorage, math in utils/paneLayout.ts) -->
    <div bind:this={panesEl} class="flex min-h-0 flex-1 gap-3">
      <!-- Left pane: source text (width = persisted ratio, changed by divider drag; the textarea fills the remaining height) -->
      <section
        class="u-card u-card--panel flex min-w-0 flex-col overflow-hidden u-pane-container"
        style="width: {leftRatio * 100}%"
      >
        <div class="u-border-b u-pane-header flex items-center justify-between px-3 py-2">
          <span class="u-label">{t('translate.from')}</span>
          <div class="flex items-center gap-2">
            {#if activeEngines.length === 0}
              <span class="u-muted text-xs">{t('translate.noActiveEngine')}</span>
            {:else}
              <span class="u-muted text-xs"
                >{activeEngines.length} · {t('translate.multiEngineHint')}</span
              >
            {/if}
            <button
              class="u-icon-btn u-icon-btn--sm u-no-drag u-tooltip u-tooltip--end"
              class:u-icon-btn--active={pinned}
              onclick={togglePin}
              aria-label={pinned ? t('translate.unpin') : t('translate.pin')}
              data-tooltip={pinned ? t('translate.unpin') : t('translate.pin')}
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M12 17v5" />
                <path
                  d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"
                />
              </svg>
            </button>
            <button
              class="u-icon-btn u-icon-btn--sm u-no-drag u-tooltip u-tooltip--end"
              class:u-icon-btn--active={autoClipboard}
              onclick={() => applyAutoClipboard(!autoClipboard)}
              aria-label={t('translate.autoClipboard')}
              data-tooltip={t('translate.autoClipboard')}
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <rect x="8" y="2" width="8" height="4" rx="1" ry="1"></rect>
                <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"
                ></path>
              </svg>
            </button>
          </div>
        </div>
        <!-- Controlled (issue #118): every edit goes through setSource, so it is recorded for
             Undo; a native undo or redo is cancelled and Kai's history walked instead. Always this
             one element (issue #144): it is never swapped for another view, so the caret and the
             scroll position survive leaving it, and undo writes into it in place. -->
        <textarea
          bind:this={sourceEl}
          class="min-h-0 flex-1 resize-none bg-transparent p-4 text-base leading-relaxed outline-none whitespace-pre-wrap"
          value={input}
          oninput={onSourceInput}
          oncompositionend={onSourceCompositionEnd}
          onbeforeinput={onSourceBeforeInput}
          onblur={endTypingRun}
          placeholder={t('translate.placeholder')}></textarea>
        <!-- Footer: Clear, Undo and Redo on the left; Copy, Cancel and Translate on the right. The
             row wraps when the pane is narrow (issue #118, the approved wireframe's question 7): the
             right group then takes its own row, right-aligned, and nothing is clipped. -->
        <div class="u-border-t flex flex-wrap items-center justify-between gap-2 px-3 py-2">
          <div class="flex items-center gap-2">
            <button class="u-btn u-btn--ghost u-no-drag px-3 py-1.5 text-sm" onclick={clearInput}>
              {t('translate.clearInput')}
            </button>
            <!-- Undo / Redo (issue #118): the source text only. Each is disabled while there is
                 nothing to undo or redo, a fresh launch included (the history starts empty).
                 Glyphs: Lucide "undo-2" / "redo-2" (ISC), inline like the swap button's. -->
            <button
              class="u-icon-btn u-no-drag"
              onclick={applyUndo}
              disabled={!canUndo(sourceHistory)}
              aria-label={t('translate.undo')}
              title={t('translate.undo')}
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M9 14 4 9l5-5" />
                <path d="M4 9h10.5a5.5 5.5 0 0 1 5.5 5.5a5.5 5.5 0 0 1-5.5 5.5H11" />
              </svg>
            </button>
            <button
              class="u-icon-btn u-no-drag"
              onclick={applyRedo}
              disabled={!canRedo(sourceHistory)}
              aria-label={t('translate.redo')}
              title={t('translate.redo')}
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="m15 14 5-5-5-5" />
                <path d="M20 9H9.5A5.5 5.5 0 0 0 4 14.5A5.5 5.5 0 0 0 9.5 20H13" />
              </svg>
            </button>
          </div>
          <div class="ml-auto flex flex-wrap items-center justify-end gap-2">
            <button
              class="u-icon-btn u-no-drag"
              onclick={() => copy(input)}
              aria-label={t('common.copy')}
              title={t('common.copy')}
              disabled={!input}
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
              </svg>
            </button>
            <!-- Request-level Cancel (issue #109): only while a request is open, left of Translate.
                 It asks the backend to stop every engine; the wait ends when they have reported. -->
            {#if awaiting}
              <button
                class="u-btn u-btn--ghost u-no-drag px-3 py-1.5 text-sm"
                onclick={cancelRequest}
              >
                <span aria-hidden="true">✕</span>
                {t('translate.cancel')}
              </button>
            {/if}
            <!-- Translate stays in its loading state until the request settles (awaiting), not
                 until the first result lands, so a second press cannot silently replace the
                 request that is still running. -->
            <button
              class="u-btn u-btn--primary u-no-drag flex items-center gap-1.5 px-5 py-1.5 text-sm"
              onclick={translateWithSwitch}
              disabled={awaiting || !input.trim() || correcting}
              title={t('translate.translateShortcutHint')}
            >
              {awaiting || correcting ? t('common.loading') : t('translate.button')}
              <!-- Cmd+Enter hint (issue #173), matching Claude Desktop's muted send-shortcut glyph:
                   Cmd+Enter already submits (issue #165); this only surfaces it visually, and only
                   while the button is actually actionable (not awaiting, not empty input). -->
              {#if !awaiting && input.trim() && !correcting}
                <span
                  class="u-shortcut-hint flex items-center gap-0.5 text-2xs opacity-70"
                  aria-hidden="true"
                >
                  <span>⌘</span><span>⏎</span>
                </span>
              {/if}
            </button>
          </div>
        </div>
      </section>

      <!-- Divider: dragging changes the left pane's ratio (see startDividerDrag / paneLayout.ts) -->
      <div
        role="separator"
        aria-orientation="vertical"
        class="w-1.5 shrink-0 cursor-col-resize rounded-full bg-[var(--app-muted)] opacity-40 transition-opacity hover:opacity-100 u-no-drag"
        onmousedown={startDividerDrag}
      ></div>

      <!-- Right pane: results (issue #9's active-engine pane: engine dropdown + status dots + the active engine's flat result text, #95) -->
      <section class="u-card u-card--panel flex min-w-0 flex-1 flex-col overflow-hidden">
        <div class="u-border-b u-pane-header flex items-center justify-between px-3 py-2">
          <span class="u-label">{t('translate.result')}</span>
          <div class="flex items-center gap-2">
            {#if activeEngines.length > 0}
              <!-- Result-pane engine dropdown (design §2): controlled display value = the active engine
                 (last-used/primary derivation, see activeEngineFor); onchange writes last-used
                 (#8's setLastUsedEngine) and resets edits. Disabled engines (just toggled in
                 settings, before EventEnginesChanged lands) are listed as disabled. -->
              <!-- pl-3 only (issue #165, re-investigated after a wrong first guess at #lang-sel):
                   .u-select reserves padding-right: 2.25rem for its custom arrow icon
                   (background-position right .75rem center), but Tailwind v4's utilities layer
                   always wins over the components layer regardless of source order, so a plain
                   px-3 here collapsed that to .75rem and let engine names (esp. the on-device
                   engine, labelled "System") sit right under the arrow — the "out of proportion"
                   dropdown. pl-3 supplies the left padding only, leaving u-select's own
                   right-padding uncontested. -->
              <select
                class="u-field u-select u-engine-select pl-3 py-2 text-sm"
                value={selectValue}
                onchange={handleEngineChange}
                aria-label={t('translate.engineActive')}
                title={t('translate.engineActive')}
              >
                {#each activeEngines as e}
                  <option value={e.value} disabled={isEngineOptionDisabled(e.value, allEngines)}>
                    {engineOptionLabel(
                      engineName(e.value),
                      isEngineOptionDisabled(e.value, allEngines),
                      t('translate.engineDisabled'),
                    )}
                  </option>
                {/each}
              </select>
              <!-- One status dot per engine (design §4): done/pending/failed/cancelled derived purely
                 from the fan-out's real output; the active engine's dot gets an accent ring so the
                 dropdown's selection is visible at a glance. A cancelled engine is a hollow muted
                 ring, neither filled nor red (issue #109). An idle window (nothing requested, issue
                 #81) has nothing to report: no fill, and the label is the engine name alone. -->
              <div class="flex items-center gap-1">
                {#each activeEngines as e (e.value)}
                  {@const st = dots[e.value] as DotState}
                  <!-- Issue #96: a failed engine that sent a payload says why in the tooltip and the
                     label (the headline only, never the raw detail); one that sent none keeps the
                     bare "Failed". failureMessage is called once per dot, for both attributes. -->
                  {@const dotFailure =
                    st === 'failed' && results[e.value]?.error
                      ? failureMessage(results[e.value], t, engineName(e.value))
                      : null}
                  <span
                    class="h-2 w-2 rounded-full"
                    class:bg-[var(--app-accent)]={st === 'done'}
                    class:bg-[var(--app-muted)]={st === 'pending'}
                    class:bg-[var(--app-danger)]={st === 'failed'}
                    style:border={st === 'cancelled' ? '1.5px solid var(--app-muted)' : null}
                    class:ring-2={e.value === activeEngine}
                    class:ring-[var(--app-accent)]={e.value === activeEngine}
                    title={dotLabel(e.value, st, dotFailure?.headline)}
                    aria-label={dotLabel(e.value, st, dotFailure?.headline)}
                  ></span>
                {/each}
              </div>
            {/if}
            <!-- Copy button: copies only the active engine's currently displayed text (including
                 manual edits), no longer concatenating all engines (design §6). -->
            {#if activeResult?.result || edited.has(activeEngine)}
              <button
                class="u-icon-btn u-no-drag"
                onclick={() => copy(activeDisplay)}
                aria-label={t('translate.copy')}
                title={t('translate.copy')}
              >
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
              </button>
            {/if}
          </div>
        </div>
        {#if noteCorrection}
          <!-- The text was corrected before it was translated (issue #208): says so in the result
             pane's own muted note style, lists what changed (word by word, computed by the
             backend), and offers ONE button: translate the original instead. The source pane
             still shows the original; nothing is saved. -->
          <div
            class="u-muted flex shrink-0 flex-col items-start gap-1 px-4 pt-3 text-2xs"
            data-testid="source-corrected-note"
          >
            <span>{t('translate.textCorrected')}</span>
            <ul class="m-0 list-none p-0">
              {#each noteCorrection.changes as c}
                <li>
                  <span class="line-through">{c.before}</span>
                  <span aria-hidden="true">→</span>
                  <span class="font-medium">{c.after}</span>
                </li>
              {/each}
            </ul>
            <button
              class="u-btn u-btn--ghost u-no-drag px-2 py-1 text-xs"
              data-testid="translate-original"
              onclick={translateOriginal}
            >
              {t('translate.translateOriginal')}
            </button>
          </div>
        {/if}
        <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
          <!-- One value decides the pane (paneState, issue #81): no-engine, loading, result, cancelled (issue #109),
             failed, or idle (nothing requested yet, or just cleared), which has no branch below and stays
             blank on purpose. This body adds no padding of its own (issue #95): as in the source
             pane, each branch owns its inset, so a line of result text lines up with a line of
             source text. -->
          {#if pane === 'no-engine'}
            <div class="flex h-full flex-col items-center justify-center gap-2 p-4 text-center">
              <svg
                class="u-muted"
                width="40"
                height="40"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M12 20h9" />
                <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
              </svg>
              <span class="u-muted text-sm">{t('translate.noActiveEngine')}</span>
            </div>
          {:else if pane === 'loading'}
            <!-- The active engine is still in flight (no result yet): flat loading placeholder
               (kai-dots + kai-loading-bar); the engine dropdown above names the engine. -->
            <div class="flex flex-col gap-2 p-4">
              <!-- The progress line (issue #109): which engine is being worked on and for how long,
                   from its started event; the plain loading text until that event has arrived. -->
              <p class="u-muted text-base leading-relaxed">
                {#if activeLine}
                  {progressText(activeLine)}
                {:else}
                  {t('common.loading')}<span class="kai-dots">{'.'.repeat(dotCount)}</span>
                {/if}
              </p>
              <div class="kai-loading-bar" aria-hidden="true"></div>
              <!-- Cancels only this engine; the others keep running and their dots keep updating. -->
              <div>
                <button
                  class="u-btn u-btn--ghost u-no-drag px-2 py-1 text-xs"
                  onclick={() => cancelEngine(activeEngine)}
                >
                  <span aria-hidden="true">✕</span>
                  {cancelActiveLabel}
                </button>
              </div>
            </div>
          {:else if pane === 'result' && activeResult}
            <!-- The active engine has a (non-empty) result: editable flat text (design §5) with the
               source pane's text look (issue #95: no card, no engine badge; the dropdown names the
               engine). The text is one real textarea that is always there (issue #144): selecting,
               clicking and typing act on it in place, and it shows the engine's string exactly,
               line breaks and blank lines included. It fills the pane, as the source textarea does.
               Edits write back to edited[activeEngine] when they are committed (the change event,
               on leaving the textarea); displayed text = edited ?? result. On engine switch edited
               is discarded wholesale and the new engine starts from its own result. The
               activeResult check is redundant at runtime (a result for the active engine implies
               it); it only narrows the type for the markup below. The detected-source, phonetic,
               cancelled and identity notes are small muted lines above the text: whichever comes
               first adds the top inset (first:pt-4), the text below brings its own p-4. -->
            {#if cueShown}
              <!-- The source and target were switched automatically (issue #200): says so, once, in
                 the result pane's own muted note style. -->
              <p class="u-muted px-4 text-2xs first:pt-4" data-testid="source-switched-note">
                {t('translate.sourceSwitched', { from: langName(fromLang), to: langName(toLang) })}
              </p>
            {/if}
            {#if activeResult.detected_from && !activeResult.identity}
              <!-- The language the engine auto-detected and translated from (issue #161): on an
                 auto source, or on a pinned source the backend corrected because the text was in
                 another language. The backend decides when it is set; nothing is compared here.
                 Display only: it never changes either select (the automatic switch of issue #200 does that,
                 before the request), and Copy never includes it. The
                 ignore keeps the call on one line: the #161 source-contract test reads it without
                 the trailing comma prettier adds when it wraps the line. -->
              <!-- prettier-ignore -->
              <p class="u-muted px-4 text-2xs first:pt-4" data-testid="detected-from-note">
                {t('translate.translatedFromDetected', { lang: langName(activeResult.detected_from) })}
              </p>
            {/if}
            {#if activeResult.phonetic}
              <span class="u-muted px-4 text-xs first:pt-4">{activeResult.phonetic}</span>
            {/if}
            {#if activeResult.cancelled}
              <!-- Cancelled with the parts already translated (the chunked translation, #84,
                 produces this): they stay on screen, marked. -->
              <span class="u-muted px-4 text-2xs first:pt-4">{t('translate.cancelled')}</span>
            {/if}
            {#if activeResult.identity}
              <!-- Same language on both sides (issue #80): the result is the source text, not a
                 translation; say so. Display only, never changes either select. -->
              <p class="u-muted px-4 text-xs first:pt-4" data-testid="identity-result">
                {t('translate.identity')}
              </p>
            {/if}
            <textarea
              class="min-h-0 flex-1 resize-none bg-transparent p-4 text-base leading-relaxed outline-none whitespace-pre-wrap"
              value={activeDisplay}
              onchange={(ev) => setEdited(activeEngine, ev.currentTarget.value)}
              placeholder={t('translate.noResult')}></textarea>
            {#if backShown}
              <!-- The result translated back into the source language (issue #56), under the
                 result and in a muted label: it only helps to check the translation; Copy copies
                 the translation above, never this. -->
              <div class="u-border-t shrink-0 px-4 py-3" data-testid="back-translation">
                <p class="u-muted text-2xs">
                  ↩ {t('translate.backLabel', { lang: langName(backShown.lang) })}
                </p>
                <p class="whitespace-pre-wrap text-sm leading-relaxed">{backShown.text}</p>
              </div>
            {:else if backBusy}
              <p
                class="u-border-t u-muted shrink-0 px-4 py-3 text-xs"
                data-testid="back-translation-pending"
              >
                {t('translate.backWorking')}
              </p>
            {/if}
          {:else if pane === 'cancelled'}
            <!-- The user cancelled the active engine and it produced nothing (issue #109): muted
               text, never the failure copy or the danger colour. There is no retry button either:
               pressing Translate again re-runs the whole fan-out. -->
            <div class="flex h-full flex-col items-center justify-center gap-2 p-4 text-center">
              <span class="u-muted text-sm">{t('translate.cancelled')}</span>
            </div>
          {:else if pane === 'failed'}
            <!-- A translation was requested and the active engine failed (a failure payload, or no
               report at all) or returned an empty result: failed state (design §5). No
               retry, no retry button — retrying means the user presses the translate button again
               (re-running the whole fan-out). Never shown for an idle window (issue #81).
               Issue #96: `failure` (derived above) is the reason, its muted detail and the optional
               Settings action; ShowSettings is the existing binding and this window stays open. -->
            <div class="flex h-full flex-col items-center justify-center gap-2 p-4 text-center">
              <span class="text-sm" style="color: var(--app-danger)">{failure.headline}</span>
              {#if failure.detail}
                <span class="u-muted max-w-[260px] break-words text-xs">{failure.detail}</span>
              {/if}
              {#if failure.action === 'settings'}
                <button
                  class="u-btn u-btn--ghost u-no-drag px-3 py-1 text-xs"
                  onclick={() => ShowSettings()}
                >
                  {t('titlebar.settings')}
                </button>
              {/if}
            </div>
          {/if}
        </div>
        {#if activeEngines.length > 0}
          <ContextChatPanel
            {chat}
            open={chatOpen}
            busy={chatBusy}
            active={hasContext(chat)}
            shortcutLabel={clearShortcut}
            ontoggle={() => (chatOpen = !chatOpen)}
            onsend={sendContext}
            onclear={clearContext}
            oncancel={cancelContext}
            note={retransNote}
            canShowOriginal={shownRetrans !== null}
            onshoworiginal={showOriginal}
          />
        {/if}
      </section>
    </div>
  </main>

  {#if toast}
    <div class="u-toast">{toast}</div>
  {/if}
</div>
