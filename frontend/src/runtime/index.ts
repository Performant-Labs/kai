import { Events, Window, System } from '@wailsio/runtime';

export async function getWindowName(): Promise<string> {
  return Window.Name();
}

export function onEvent(name: string, cb: (data: any) => void): () => void {
  return Events.On(name, (e: any) => cb(e.data));
}

export function emitEvent(name: string, data?: any): void {
  Events.Emit(name, data);
}

export { Window, System };

// Desktop notifications are sent natively by the backend Wails notifications service
// (macOS via UNUserNotificationCenter) instead of being relayed through the frontend Web
// Notification API (the frontend can't show notifications while in the background, and
// this path also bypasses system notification permission prompts).
