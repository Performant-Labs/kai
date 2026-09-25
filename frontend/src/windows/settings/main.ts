import '../../app.css';
import { mount } from 'svelte';
import Settings from '../../components/Settings.svelte';
import { initTheme } from '../../stores/theme';
import { initWindow } from '../../stores/ui';
import { installFrontendLogging } from '../../runtime/frontendLog';
import { AppService } from '@bindings/cnb.cool/dtapp/kai/internal/service';

installFrontendLogging();
initTheme();
initWindow();

// Pass the WebView's UA to the backend, used as the default User-Agent for all HTTP requests.
AppService.SetUserAgent(navigator.userAgent);
console.debug('UA:', navigator.userAgent);

const app = mount(Settings, {
  target: document.getElementById('app')!,
});

export default app;
