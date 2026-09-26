<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../../i18n';
  import {
    GetAllEngines,
    GetKnownEngines,
    GetEngineSchema,
    GetEngineConfig,
    AddEngine,
    UpdateEngineConfig,
    ToggleEngineEnabled,
    RemoveEngine,
    CheckTesseract,
    GetOcrLangs,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts';
  import type {
    AllEngineItem,
    EngineListItem,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import type {
    EngineSchema,
    EngineFieldSchema,
    EngineConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/engine/models.ts';
  import {
    SystemLanguages,
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { Dialogs } from '@wailsio/runtime';
  import { emitEvent } from '../../runtime';
  import { EventEnginesChanged } from '../../utils/events';
  let engines = $state<AllEngineItem[]>([]);

  // Grouped display by Kind: translate engines / OCR engines (TTS's apple engine is grouped under translate)
  type EngineGroup = { kind: string; title: string; items: AllEngineItem[] };
  const engineGroups = $derived.by<EngineGroup[]>(() => {
    const order = ['translate', 'ocr'];
    const map = new Map<string, EngineGroup>();
    for (const k of order) {
      map.set(k, {
        kind: k,
        title: t(k === 'ocr' ? 'settings.engineGroupOCR' : 'settings.engineGroupTranslate'),
        items: [],
      });
    }
    for (const e of engines) {
      const g = map.get(e.kind) ?? map.get('translate')!;
      g.items.push(e);
    }
    return order.map((k) => map.get(k)!).filter((g) => g.items.length > 0);
  });
  // Primary (default) translation engine identifier: settings.default_engine. The star button
  // highlights based on it; read initially from GetConfig(), re-read by loadPrimary() after
  // engines are added/removed or enabled/disabled.
  let primaryEngine = $state<string>('');
  let selectedId = $state<number | null>(null);
  let schema = $state<EngineSchema | null>(null);
  let configValues = $state<Record<string, string>>({});

  // OCR engines (vision / tesseract) keep their dedicated params unified in Extra(JSON):
  //   - ocrLangs:     multi-select of language codes (joined with "+", e.g. "chi_sim+eng")
  //   - ocrCorrect:   whether language correction is on (vision semantics only; tesseract ignores it)
  //   - ocrTimeoutSec: OCR timeout in seconds (default 60)
  //   - ocrRetry:     Vision OCR failure fallback retry count (vision semantics only; tesseract ignores it, default 2)
  let ocrLangs = $state<string[]>([]);
  let ocrCorrect = $state(true);
  let ocrTimeoutSec = $state(60);
  let ocrRetry = $state(2);
  // OCR language-code candidates (from the backend's GetOcrLangs, decoupled from Extra(JSON))
  let ocrLangOptions = $state<string[]>([]);

  function toggleOcrLang(code: string) {
    const set = new Set(ocrLangs);
    if (set.has(code)) set.delete(code);
    else set.add(code);
    ocrLangs = [...set];
  }
  // Parse the OCR params out of the engine's extra(JSON), populating local state (shared by vision / tesseract).
  // Legacy-data compatibility: when extra is a plain string of language codes (not JSON), the
  // whole string is used as the langs fallback.
  function loadOcrOpts(extra: string | undefined) {
    ocrLangs = [];
    ocrCorrect = true;
    ocrTimeoutSec = 60;
    ocrRetry = 2;
    if (!extra) return;
    // Try JSON first (the unified approach)
    try {
      const o = JSON.parse(extra);
      if (typeof o.langs === 'string' && o.langs) {
        ocrLangs = o.langs
          .split('+')
          .map((s: string) => s.trim())
          .filter(Boolean);
      }
      if (typeof o.correct_text === 'boolean') ocrCorrect = o.correct_text;
      if (typeof o.timeout_sec === 'number' && o.timeout_sec > 0) ocrTimeoutSec = o.timeout_sec;
      if (typeof o.retry_count === 'number' && o.retry_count > 0) ocrRetry = o.retry_count;
      return;
    } catch {
      /* Not JSON; fall through to the legacy plain-string language-code compat below */
    }
    ocrLangs = extra
      .split('+')
      .map((s) => s.trim())
      .filter(Boolean);
  }
  // Merge the OCR params into extra(JSON). langs is written only for tesseract (vision uses the
  // system Vision framework and needs no language codes); correct is written only for vision
  // (isVision=true).
  function buildExtraWithOcr(extra: string | undefined, isVision: boolean): string {
    let o: Record<string, unknown> = {};
    if (extra) {
      try {
        o = JSON.parse(extra);
      } catch {
        o = {};
      }
    }
    if (!isVision) o.langs = ocrLangs.join('+');
    o.timeout_sec = ocrTimeoutSec;
    if (isVision) {
      o.correct_text = ocrCorrect;
      o.retry_count = ocrRetry;
    }
    return JSON.stringify(o);
  }
  // LLM translation engines (openai / anthropic / gemini) keep their params unified in Extra(JSON):
  //   - llmModel: model name (e.g. gpt-4o-mini)
  // There is no timeout setting (issue #109): a request runs until it answers or the user cancels
  // it. A timeout_sec that an older version stored is left in the row as it is and ignored.
  let llmModel = $state('');
  // Parse the LLM params out of the engine's extra(JSON), populating local state.
  // Legacy-data compatibility: when extra is a plain model-name string (not JSON), the whole
  // string is used as the model fallback.
  function loadLlmOpts(extra: string | undefined) {
    llmModel = '';
    if (!extra) return;
    try {
      const o = JSON.parse(extra);
      if (typeof o.model === 'string' && o.model) llmModel = o.model;
      return;
    } catch {
      /* Not JSON; fall through to the legacy plain model-name string compat below */
    }
    llmModel = extra;
  }
  // Merge the LLM params into extra(JSON). When model is empty, the backend falls back to its default model.
  function buildExtraWithLlm(extra: string | undefined): string {
    let o: Record<string, unknown> = {};
    if (extra) {
      try {
        o = JSON.parse(extra);
      } catch {
        o = {};
      }
    }
    o.model = llmModel;
    return JSON.stringify(o);
  }
  // The system engine's supported-language list (read-only display; the backend reads it from Translation.framework)
  let systemLangs = $state<string[]>([]);
  let systemLangsLoading = $state(false);
  // Tesseract install probe result (refreshed when tesseract is selected; the right side shows
  // "installed / not installed" + path/version)
  let tesseract = $state<{ installed: boolean; path: string; version: string; os: string } | null>(
    null,
  );

  // Add-engine modal
  let showAdd = $state(false);
  let knownEngines = $state<EngineListItem[]>([]);
  let addName = $state('');
  let addSchema = $state<EngineFieldSchema[]>([]);
  let addValues = $state<Record<string, string>>({});

  // Secret-field plaintext/masked toggle state (tracked per field key) so saved secret values can be inspected.
  let revealed = $state<Record<string, boolean>>({});
  function toggleReveal(key: string) {
    revealed = { ...revealed, [key]: !revealed[key] };
  }

  onMount(() => {
    loadEngines();
    loadPrimary();
  });

  async function loadOcrLangs() {
    try {
      ocrLangOptions = (await GetOcrLangs()) ?? [];
    } catch (e) {
      console.error(t('log.engineLoadOcrLangsFailed'), e);
      ocrLangOptions = [];
    }
  }

  async function loadEngines() {
    try {
      engines = (await GetAllEngines()) ?? [];
      if (engines.length && selectedId === null) selectEngine(engines[0].id);
    } catch (e) {
      console.error(t('log.engineLoadListFailed'), e);
      engines = [];
    }
  }

  // Read the primary-engine identifier from settings (the basis for star highlighting). Re-read
  // after engines are added/removed or enabled/disabled, so the "primary disabled → star not
  // highlighted" demotion reflects in the UI immediately (without rewriting the saved value).
  async function loadPrimary() {
    try {
      const cfg = await GetConfig();
      primaryEngine = cfg?.default_engine ?? '';
    } catch (e) {
      console.error(t('log.setPrimaryFailed'), e);
    }
  }

  // Set as primary engine: click the star → SaveConfig read-modify-write (only changes
  // default_engine, doesn't zero other fields) → broadcast EventEnginesChanged so the translate
  // window re-resolves → the local star state updates immediately.
  // Clicking again while already primary = clear it (empties default_engine).
  async function setPrimary(e: AllEngineItem) {
    const wasPrimary = primaryEngine === e.value;
    const next = wasPrimary ? '' : e.value;
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, default_engine: next });
      primaryEngine = next;
      emitEvent(EventEnginesChanged);
      if (wasPrimary) {
        await Dialogs.Info({
          Title: t('settings.engineSavedTitle'),
          Message: t('settings.enginePrimaryCleared'),
        });
      }
    } catch (err) {
      // Failure: roll back the local star state + show an error (same shape as toggleEngine's error handling).
      console.error(t('log.setPrimaryFailed'), err);
      primaryEngine = wasPrimary ? '' : primaryEngine;
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(err),
      });
    }
  }

  async function selectEngine(id: number) {
    selectedId = id;
    configValues = {};
    const eng = engines.find((e) => e.id === id);
    if (!eng) return;
    // Fetch the fully persisted config and populate the form (endpoint / API key etc. are no longer empty)
    let saved: EngineConfig | null = null;
    try {
      const s: EngineSchema = await GetEngineSchema(eng.value);
      schema = s;
      try {
        saved = await GetEngineConfig(id);
      } catch (e) {
        console.error(t('log.engineReadConfigFailed'), e);
        saved = null;
      }
      for (const f of schema.fields ?? []) {
        configValues[f.field] = (saved?.[f.field as keyof typeof saved] as string) ?? '';
      }
    } catch (e) {
      console.error(t('log.engineLoadFieldsFailed'), e);
      schema = null;
    }
    // When the system translation engine is selected, fetch and display its supported languages (read-only)
    if (eng.value === 'apple' && eng.supported) {
      loadSystemLangs();
    } else {
      systemLangs = [];
    }
    // When tesseract is selected, probe whether it's installed locally, for the right side to show install status
    if (eng.value === 'tesseract') {
      checkTesseract();
    } else {
      tesseract = null;
    }
    // When any OCR engine (vision / tesseract) is selected, populate its dedicated OCR params from extra(JSON)
    if (eng.kind === 'ocr') {
      await loadOcrLangs();
      loadOcrOpts(saved?.extra);
    }
    // When an LLM translation engine (openai / anthropic / gemini) is selected, populate the model from extra(JSON)
    if (['openai', 'anthropic', 'gemini'].includes(eng.value)) {
      loadLlmOpts(saved?.extra);
    }
  }

  // checkTesseract asks the backend to probe the local tesseract installation
  async function checkTesseract() {
    try {
      tesseract = await CheckTesseract();
    } catch (e) {
      console.error(t('log.engineProbeTesseractFailed'), e);
      tesseract = { installed: false, path: '', version: '', os: '' };
    }
  }

  // loadSystemLangs reads the languages supported by system translation from the backend
  // (Translation.framework's installed language packs).
  async function loadSystemLangs() {
    systemLangsLoading = true;
    systemLangs = [];
    try {
      const langs = await SystemLanguages();
      systemLangs = Array.isArray(langs) ? langs : [];
    } catch (e) {
      console.error(t('log.engineReadSystemLangsFailed'), e);
      systemLangs = [];
    } finally {
      systemLangsLoading = false;
    }
  }

  // parseErr converts an error thrown by the backend into displayable copy.
  // The backend's required-field validation errors have the localized "missing required field"
  // prefix (settings.engineMissing) followed by a settings.engine_field.xxx key; that key part
  // goes through i18n.
  function parseErr(e: unknown): string {
    const msg = e instanceof Error ? e.message : String(e);
    const prefix = t('settings.engineMissing');
    if (msg.startsWith(prefix)) {
      const key = msg.slice(prefix.length);
      return prefix + t(key);
    }
    return msg;
  }

  async function saveConfig() {
    const eng = engines.find((e) => e.id === selectedId);
    if (!eng) return;
    // OCR engines (vision / tesseract): write the dedicated OCR params back into extra(JSON) before submitting
    let extra = configValues['extra'] || undefined;
    if (eng.kind === 'ocr') {
      extra = buildExtraWithOcr(configValues['extra'], eng.value === 'vision');
    }
    // LLM engines (openai / anthropic / gemini): write the model name back into extra(JSON) before submitting
    if (['openai', 'anthropic', 'gemini'].includes(eng.value)) {
      extra = buildExtraWithLlm(configValues['extra']);
    }
    try {
      await UpdateEngineConfig({
        id: eng.id,
        engine: eng.value,
        enabled: eng.enabled,
        api_key: configValues['api_key'] || undefined,
        secret: configValues['secret'] || undefined,
        endpoint: configValues['endpoint'] || undefined,
        extra,
      });
      await Dialogs.Info({
        Title: t('settings.engineSavedTitle'),
        Message: t('settings.engineSaved'),
      });
      // Saving may change enabled state/config: re-fetch and broadcast the change to the translate window etc.
      await loadEngines();
      emitEvent(EventEnginesChanged);
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
    }
  }

  async function toggleEngine(id: number, enabled: boolean, el?: HTMLInputElement) {
    const eng = engines.find((x) => x.id === id);
    // Built-in engines (e.g. vision system OCR / apple system translation) can toggle enabled but
    // cannot be deleted; when a built-in OCR item toggles, the backend enforces OCR single-select
    // (auto-disabling other OCR engines).
    // Engines not supported on the current platform (e.g. apple is macOS-only) cannot be toggled.
    if (eng && !eng.supported) {
      if (el) el.checked = eng.enabled;
      engines = engines.map((x) => (x.id === id ? { ...x, enabled: eng.enabled } : x));
      return;
    }
    try {
      await ToggleEngineEnabled(id, enabled);
      // Success: re-fetch to stay consistent with the backend
      await loadEngines();
      // Enable/disable may change "whether the primary engine is still usable": re-read the
      // primary identifier so a disabled primary's star demotes immediately (resolution fallback
      // is handled by the backend/translate window; this only updates the visuals, never the
      // saved value).
      await loadPrimary();
      // Broadcast the engine change, telling the translate window etc. to re-fetch their engine lists
      emitEvent(EventEnginesChanged);
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
      // Roll back local state: on validation failure the user has already toggled the checkbox,
      // and Svelte's keyed each reusing the same DOM node won't proactively undo the browser's
      // flipped checked, leaving the visual stuck.
      // So sync the checked state back via the DOM directly, and sync the engines data source too.
      const prev = !enabled;
      if (el) el.checked = prev;
      engines = engines.map((x) => (x.id === id ? { ...x, enabled: prev } : x));
    }
  }

  async function removeEngine(id: number) {
    try {
      await RemoveEngine(id);
      selectedId = null;
      await loadEngines();
      // Broadcast the engine change, telling the translate window etc. to re-fetch their engine lists
      emitEvent(EventEnginesChanged);
      await Dialogs.Info({
        Title: t('settings.engineSavedTitle'),
        Message: t('settings.engineRemoved'),
      });
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
    }
  }

  // Open the add-engine modal (the dropdown comes from GetKnownEngines; the list renders only engines already in the database)
  async function openAdd() {
    showAdd = true;
    addName = '';
    addSchema = [];
    addValues = {};
    try {
      knownEngines = (await GetKnownEngines()) ?? [];
    } catch (e) {
      console.error(t('log.engineLogOptionalListFailed'), e);
      knownEngines = [];
    }
  }

  function closeAdd() {
    showAdd = false;
  }

  function onAddOverlayKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeAdd();
    }
  }

  // Load the engine's field schema dynamically after selecting the engine type
  async function onAddNameChange(name: string) {
    addName = name;
    addValues = {};
    ocrLangs = [];
    ocrCorrect = true;
    ocrTimeoutSec = 60;
    ocrRetry = 2;
    llmModel = '';
    if (!name) {
      addSchema = [];
      return;
    }
    try {
      const s: EngineSchema = await GetEngineSchema(name);
      addSchema = s.fields ?? [];
      for (const f of addSchema) addValues[f.field] = f.default ?? '';
      if (s.kind === 'ocr') {
        await loadOcrLangs();
      }
    } catch (e) {
      console.error(t('log.engineLoadAddFieldsFailed'), e);
      addSchema = [];
    }
  }

  async function submitAdd() {
    if (!addName) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: t('settings.engineAddSelect'),
      });
      return;
    }
    // OCR engine: assemble the dedicated OCR params (langs/timeout) into extra(JSON) before submitting
    let extra = addValues['extra'] || undefined;
    const addSchemaKind = addSchema.length ? addSchema[0] : null;
    if (addSchemaKind && (await addEngineIsOcr(addName))) {
      extra = buildExtraWithOcr(addValues['extra'], addName === 'vision');
    }
    // LLM engines (openai / anthropic / gemini): assemble the model name into extra(JSON) before submitting
    if (['openai', 'anthropic', 'gemini'].includes(addName)) {
      extra = buildExtraWithLlm(addValues['extra']);
    }
    try {
      await AddEngine({
        id: 0,
        engine: addName,
        enabled: false,
        api_key: addValues['api_key'] || undefined,
        secret: addValues['secret'] || undefined,
        endpoint: addValues['endpoint'] || undefined,
        extra,
      });
      showAdd = false;
      await loadEngines();
      // Broadcast the engine change, telling the translate window etc. to re-fetch their engine lists
      emitEvent(EventEnginesChanged);
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
    }
  }

  // addEngineIsOcr decides whether the engine type being added is OCR (per the backend KnownEngines' kind).
  async function addEngineIsOcr(name: string): Promise<boolean> {
    try {
      const s: EngineSchema = await GetEngineSchema(name);
      return s.kind === 'ocr';
    } catch {
      return false;
    }
  }
</script>

<header class="mb-6">
  <h1 class="text-2xl font-semibold">{t('settings.enginesTitle')}</h1>
</header>

<div class="flex min-h-[360px] gap-5">
  <!-- Engine list -->
  <div class="u-card u-card--panel flex w-56 flex-col p-4">
    <div class="mb-3 flex items-center justify-between px-1">
      <span class="u-label">{t('settings.engineList')}</span>
      <button
        class="u-btn u-btn--ghost px-2 py-0.5 text-xs"
        onclick={openAdd}
        aria-label={t('settings.engineAdd')}>＋</button
      >
    </div>
    <ul class="flex-1 space-y-3 overflow-y-auto">
      {#each engineGroups as group (group.kind)}
        <li class="space-y-1">
          <div class="u-label px-1 pb-0.5 text-[11px] opacity-70">{group.title}</div>
          {#each group.items as e (e.id)}
            <div class="u-list-item" class:is-active={selectedId === e.id}>
              {#if e.kind === 'translate'}
                <!-- Primary-engine star: click to set as the primary translation engine (persists
                     immediately + broadcasts); click again to clear. Rendered only for translate
                     engines; a disabled engine's star is disabled (cannot be set as primary).
                     Highlight condition = currently primary AND enabled (a disabled primary's star
                     auto-demotes). -->
                <button
                  class="u-icon-btn u-icon-btn--sm"
                  class:u-icon-btn--active={primaryEngine === e.value && e.enabled}
                  disabled={!e.enabled}
                  aria-label={t('settings.engineSetPrimary')}
                  title={t('settings.engineSetPrimary')}
                  onclick={() => setPrimary(e)}
                >
                  ★
                </button>
              {/if}
              <button
                class="flex-1 bg-transparent text-left text-sm font-medium"
                onclick={() => selectEngine(e.id)}
              >
                {engineName(e.value)}
              </button>
              {#if !e.supported}
                <span class="u-muted text-[10px] leading-tight text-right max-w-[3.5rem]"
                  >{t('settings.engineUnsupported')}</span
                >
              {:else}
                <label class="u-switch" aria-label={t('settings.engineEnabled')}>
                  <input
                    type="checkbox"
                    checked={e.enabled}
                    onchange={(ev) =>
                      toggleEngine(
                        e.id,
                        (ev.target as HTMLInputElement).checked,
                        ev.target as HTMLInputElement,
                      )}
                  />
                  <span class="u-switch__track">
                    <span class="u-switch__thumb"></span>
                  </span>
                </label>
              {/if}
            </div>
          {/each}
        </li>
      {/each}
    </ul>
    {#if engines.length === 0}
      <p class="u-muted px-1 pt-2 text-xs">{t('settings.engineListEmpty')}</p>
    {/if}
  </div>

  <!-- Engine config -->
  <div class="u-card u-card--panel flex flex-1 flex-col p-5">
    <div class="u-label mb-4">
      {t('settings.engineConfig')}
    </div>
    {#if schema?.builtin}
      <div class="flex items-start gap-2 rounded-md border u-border-ok px-3 py-2 text-xs">
        <span class="mt-0.5 inline-block h-2 w-2 shrink-0 rounded-full u-bg-ok"></span>
        <div>
          <p class="font-medium">
            {#if schema?.kind === 'ocr'}
              {t('settings.engine_tip.vision_builtin')}
            {:else}
              {t('settings.engine_tip.apple_builtin')}
            {/if}
          </p>
          <p class="u-muted">
            {#if schema?.kind === 'ocr'}
              {t('settings.engine_tip.vision_builtin_desc')}
            {:else}
              {t('settings.engine_tip.apple_builtin_desc')}
            {/if}
          </p>
        </div>
      </div>
    {/if}
    {#if (schema?.fields?.length ?? 0) === 0}
      <div class="flex flex-1 flex-col items-center justify-center text-center">
        <p class="u-muted text-sm">{t('settings.engineNoSchema')}</p>
      </div>
    {:else}
      <div class="u-card mt-4 space-y-4 px-4 py-3">
        {#if schema?.kind === 'translate' && schema?.builtin}
          <div>
            <p class="u-label mb-2">{t('settings.engineSystemLangs')}</p>
            {#if systemLangsLoading}
              <p class="u-muted text-xs">{t('common.loading')}</p>
            {:else if systemLangs.length}
              <div class="flex flex-wrap gap-1.5">
                {#each systemLangs as code}
                  <span class="u-tag">{langName(code)}</span>
                {/each}
              </div>
            {:else}
              <p class="u-muted text-xs">{t('settings.engineSystemLangsEmpty')}</p>
            {/if}
          </div>
        {/if}
        {#each schema?.fields ?? [] as f}
          {#if f.widget === 'ocr_status'}
            <!-- Tesseract install-status probe card (with an editable custom binary path endpoint); declared only in the tesseract schema -->
            {#if tesseract}
              <div
                class="flex flex-col gap-2 rounded-md border px-3 py-2 text-xs"
                class:u-border-danger={!tesseract.installed}
                class:u-border-ok={tesseract.installed}
              >
                <div class="flex items-start gap-2">
                  <span
                    class="mt-0.5 inline-block h-2 w-2 shrink-0 rounded-full"
                    class:u-bg-danger={!tesseract.installed}
                    class:u-bg-ok={tesseract.installed}
                  ></span>
                  <div>
                    {#if tesseract.installed}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_installed')}</p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_path')}{tesseract.path}
                      </p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_version')}{tesseract.version || '-'}
                      </p>
                    {:else}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_missing')}</p>
                      {#if tesseract.os === 'darwin'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_mac')}
                        </p>
                      {:else if tesseract.os === 'windows'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_windows')}
                        </p>
                      {:else}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_linux')}
                        </p>
                      {/if}
                    {/if}
                  </div>
                </div>
                <div class="mt-1">
                  <label class="mb-1 block text-xs font-medium" for={'ef-' + f.field}>
                    {f.label_key ? t(f.label_key as any) : f.field}
                  </label>
                  <input
                    id={'ef-' + f.field}
                    type="text"
                    class="u-field w-full px-3 py-1.5 text-xs"
                    placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                    bind:value={configValues[f.field]}
                  />
                </div>
              </div>
            {/if}
          {:else if f.widget === 'ocr_langs'}
            <!-- OCR recognition-language multi-select (tesseract only); candidates from ocrLangOptions -->
            <div>
              <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
              {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              <div class="mt-2 flex flex-wrap gap-1.5">
                {#each ocrLangOptions as code}
                  <button
                    type="button"
                    class="u-chip"
                    class:is-on={ocrLangs.includes(code)}
                    onclick={() => toggleOcrLang(code)}
                  >
                    {langName(code)}
                  </button>
                {/each}
              </div>
            </div>
          {:else if f.widget === 'ocr_timeout'}
            <!-- OCR timeout (seconds) -->
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="5"
                max="300"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrTimeoutSec}
              />
            </div>
          {:else if f.widget === 'ocr_retry'}
            <!-- OCR failure retry count (vision only) -->
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="0"
                max="10"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrRetry}
              />
            </div>
          {:else if f.widget === 'ocr_correct'}
            <!-- OCR language-correction toggle (vision only) -->
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <label class="u-switch" aria-label={f.label_key ? t(f.label_key as any) : f.field}>
                <input type="checkbox" bind:checked={ocrCorrect} />
                <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
              </label>
            </div>
          {:else if f.widget === 'llm_model'}
            <!-- LLM translation engine: model name -->
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <input
                id={'ef-' + f.field}
                type="text"
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={llmModel}
              />
            </div>
          {:else if f.type === 'secret'}
            <!-- Secret-type field: plaintext/masked toggle so saved values can be inspected -->
            {@const revealKey = 'ef-reveal-' + f.field}
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <div class="relative">
                <input
                  id={'ef-' + f.field}
                  type={revealed[revealKey] ? 'text' : 'password'}
                  class="u-field w-full px-3 py-2 pr-10 text-sm"
                  placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                  bind:value={configValues[f.field]}
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 u-muted px-1 text-xs"
                  aria-label={revealed[revealKey]
                    ? t('settings.hideSecret')
                    : t('settings.showSecret')}
                  onclick={() => toggleReveal(revealKey)}
                >
                  {revealed[revealKey] ? '🙈' : '👁'}
                </button>
              </div>
            </div>
          {:else}
            <!-- Plain text field -->
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <input
                id={'ef-' + f.field}
                type="text"
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={configValues[f.field]}
              />
            </div>
          {/if}
        {/each}
        <div class="u-border-t pt-4">
          <button class="u-btn u-btn--primary px-5 py-2 text-sm" onclick={saveConfig}>
            {t('settings.engineSave')}
          </button>
        </div>
      </div>
    {/if}
    {#if selectedId !== null}
      {@const sel = engines.find((x) => x.id === selectedId)}
      <div class="mt-auto pt-4">
        {#if sel?.builtin}
          <span class="u-muted text-xs">{t('settings.engineBuiltinHint')}</span>
        {:else}
          <button
            class="u-btn u-btn--link-danger text-sm"
            onclick={() => {
              if (selectedId !== null) removeEngine(selectedId);
            }}
          >
            {t('settings.engineRemove')}
          </button>
        {/if}
      </div>
    {/if}
  </div>
</div>

{#if showAdd}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    role="button"
    tabindex="0"
    aria-label={t('settings.engineAddClose')}
    onclick={(e) => {
      if (e.target === e.currentTarget) closeAdd();
    }}
    onkeydown={onAddOverlayKeydown}
  >
    <div
      class="u-card u-card--panel w-[420px] max-w-[90vw] p-5"
      role="dialog"
      aria-modal="true"
      aria-label={t('settings.engineAddTitle')}
    >
      <div class="mb-4 flex items-center justify-between">
        <h2 class="text-base font-semibold">{t('settings.engineAddTitle')}</h2>
        <button class="text-sm" onclick={() => (showAdd = false)} aria-label={t('common.close')}
          >✕</button
        >
      </div>
      <div class="space-y-4">
        <div>
          <label class="mb-1.5 block text-sm font-medium" for="add-engine-type"
            >{t('settings.engineAddType')}</label
          >
          <select
            id="add-engine-type"
            class="u-field u-select w-full px-3 py-2 text-sm"
            value={addName}
            onchange={(e) => onAddNameChange((e.target as HTMLSelectElement).value)}
          >
            <option value="">{t('settings.engineAddSelectPlaceholder')}</option>
            {#each knownEngines as k}
              <option value={k.value}>{engineName(k.value)}</option>
            {/each}
          </select>
        </div>
        {#each addSchema as f}
          {#if f.widget === 'ocr_status'}
            {#if tesseract}
              <div
                class="flex flex-col gap-2 rounded-md border px-3 py-2 text-xs"
                class:u-border-danger={!tesseract.installed}
                class:u-border-ok={tesseract.installed}
              >
                <div class="flex items-start gap-2">
                  <span
                    class="mt-0.5 inline-block h-2 w-2 shrink-0 rounded-full"
                    class:u-bg-danger={!tesseract.installed}
                    class:u-bg-ok={tesseract.installed}
                  ></span>
                  <div>
                    {#if tesseract.installed}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_installed')}</p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_path')}{tesseract.path}
                      </p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_version')}{tesseract.version || '-'}
                      </p>
                    {:else}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_missing')}</p>
                      {#if tesseract.os === 'darwin'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_mac')}
                        </p>
                      {:else if tesseract.os === 'windows'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_windows')}
                        </p>
                      {:else}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_linux')}
                        </p>
                      {/if}
                    {/if}
                  </div>
                </div>
                <div class="mt-1">
                  <label class="mb-1 block text-xs font-medium" for={'add-ef-' + f.field}>
                    {f.label_key ? t(f.label_key as any) : f.field}
                  </label>
                  <input
                    id={'add-ef-' + f.field}
                    type="text"
                    class="u-field w-full px-3 py-1.5 text-xs"
                    placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                    bind:value={addValues[f.field]}
                  />
                </div>
              </div>
            {/if}
          {:else if f.widget === 'ocr_langs'}
            <div>
              <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
              {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              <div class="mt-2 flex flex-wrap gap-1.5">
                {#each ocrLangOptions as code}
                  <button
                    type="button"
                    class="u-chip"
                    class:is-on={ocrLangs.includes(code)}
                    onclick={() => toggleOcrLang(code)}
                  >
                    {langName(code)}
                  </button>
                {/each}
              </div>
            </div>
          {:else if f.widget === 'ocr_timeout'}
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="5"
                max="300"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrTimeoutSec}
              />
            </div>
          {:else if f.widget === 'ocr_retry'}
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="0"
                max="10"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrRetry}
              />
            </div>
          {:else if f.widget === 'ocr_correct'}
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <label class="u-switch" aria-label={f.label_key ? t(f.label_key as any) : f.field}>
                <input type="checkbox" bind:checked={ocrCorrect} />
                <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
              </label>
            </div>
          {:else if f.widget === 'llm_model'}
            <!-- Add modal: LLM model name -->
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'add-ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <input
                id={'add-ef-' + f.field}
                type="text"
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={llmModel}
              />
            </div>
          {:else if f.type === 'secret'}
            <!-- Secret-type field: plaintext/masked toggle so saved values can be inspected -->
            {@const revealKey = 'add-ef-reveal-' + f.field}
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'add-ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <div class="relative">
                <input
                  id={'add-ef-' + f.field}
                  type={revealed[revealKey] ? 'text' : 'password'}
                  class="u-field w-full px-3 py-2 pr-10 text-sm"
                  placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                  bind:value={addValues[f.field]}
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 u-muted px-1 text-xs"
                  aria-label={revealed[revealKey]
                    ? t('settings.hideSecret')
                    : t('settings.showSecret')}
                  onclick={() => toggleReveal(revealKey)}
                >
                  {revealed[revealKey] ? '🙈' : '👁'}
                </button>
              </div>
            </div>
          {:else}
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'add-ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <input
                id={'add-ef-' + f.field}
                type="text"
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={addValues[f.field]}
              />
            </div>
          {/if}
        {/each}
        <div class="flex justify-end gap-2 pt-1">
          <button class="u-btn u-btn--ghost px-4 py-1.5 text-sm" onclick={() => (showAdd = false)}
            >{t('common.cancel')}</button
          >
          <button class="u-btn u-btn--primary px-4 py-1.5 text-sm" onclick={submitAdd}
            >{t('settings.engineAddConfirm')}</button
          >
        </div>
      </div>
    </div>
  </div>
{/if}
