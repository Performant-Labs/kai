import { FrontendLog } from '@bindings/cnb.cool/dtapp/kai/internal/logutil/frontendlogservice.ts';
import { t } from '../i18n';

// Forwards frontend console logs and JS errors to Go, which writes them to logs/frontend.log.
// Keeps frontend error clues from being lost outside dev (packaged builds have no devtools).

let installed = false;

type Level = 'debug' | 'info' | 'warn' | 'error';

// Sends a single log entry to Go. Async, swallows exceptions so logging never loops
// back into or blocks the main flow.
function send(level: Level, msg: string) {
  FrontendLog(level, msg).catch(() => {
    /* Ignore forwarding failures; never block the frontend */
  });
}

function serialize(args: unknown[]): string {
  return args
    .map((a) => {
      if (typeof a === 'string') return a;
      try {
        return JSON.stringify(a);
      } catch {
        return String(a);
      }
    })
    .join(' ');
}

// Installs global error capture and console forwarding. Call once per window entry point (idempotent).
export function installFrontendLogging() {
  if (installed) return;
  installed = true;

  const orig = {
    log: console.log,
    info: console.info,
    warn: console.warn,
    error: console.error,
    debug: console.debug,
  };

  console.log = (...args: unknown[]) => {
    orig.log(...args);
    send('info', serialize(args));
  };
  console.info = (...args: unknown[]) => {
    orig.info(...args);
    send('info', serialize(args));
  };
  console.debug = (...args: unknown[]) => {
    orig.debug(...args);
    send('debug', serialize(args));
  };
  console.warn = (...args: unknown[]) => {
    orig.warn(...args);
    send('warn', serialize(args));
  };
  console.error = (...args: unknown[]) => {
    orig.error(...args);
    send('error', serialize(args));
  };

  // Uncaught sync/async errors
  window.addEventListener('error', (e: ErrorEvent) => {
    const detail = e.error?.stack || `${e.message} @ ${e.filename}:${e.lineno}:${e.colno}`;
    send('error', t('log.uncaughtError') + detail);
  });

  // Unhandled promise rejections
  window.addEventListener('unhandledrejection', (e: PromiseRejectionEvent) => {
    const reason = e.reason?.stack || e.reason?.message || String(e.reason);
    send('error', t('log.unhandledRejection') + reason);
  });
}
