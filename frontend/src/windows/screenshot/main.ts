import '../../app.css';
import { mount } from 'svelte';
import ScreenshotWindow from '../../components/ScreenshotWindow.svelte';
import { initTheme } from '../../stores/theme';
import { initFontSize } from '../../stores/fontSize';
import { initWindow } from '../../stores/ui';
import { installFrontendLogging } from '../../runtime/frontendLog';
import { AppService } from '@bindings/cnb.cool/dtapp/kai/internal/service';

installFrontendLogging();
initTheme();
initFontSize();
initWindow();

// Pass the WebView's UA to the backend, used as the default User-Agent for all HTTP requests.
AppService.SetUserAgent(navigator.userAgent);
console.debug('UA:', navigator.userAgent);

const app = mount(ScreenshotWindow, {
  target: document.getElementById('app')!,
});

export default app;
