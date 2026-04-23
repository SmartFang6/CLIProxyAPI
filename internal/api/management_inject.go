package api

import "strings"

const managementUpgradeInjectionMarker = "data-cli-proxy-upgrade-injected"

const managementUpgradeInjectionScript = `
<script ` + managementUpgradeInjectionMarker + `="true">
(function () {
  const AUTH_STORE_KEY = 'cli-proxy-auth';
  const UPGRADE_ROUTE = '/v0/management/upgrade';
  const VERSION_ROUTE = '/v0/management/latest-version';

  function readPersistedAuth() {
    try {
      const raw = localStorage.getItem(AUTH_STORE_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        const state = parsed && parsed.state ? parsed.state : parsed;
        if (state && typeof state === 'object') {
          return {
            apiBase: typeof state.apiBase === 'string' ? state.apiBase : '',
            managementKey: typeof state.managementKey === 'string' ? state.managementKey : ''
          };
        }
      }
    } catch (error) {
      console.warn('read cli-proxy-auth failed', error);
    }

    return {
      apiBase: localStorage.getItem('apiBase') || '',
      managementKey: localStorage.getItem('managementKey') || ''
    };
  }

  function normalizeBase(base) {
    if (!base) return '';
    const cleaned = String(base).trim().replace(/\/+$/, '');
    if (!cleaned) return '';
    if (cleaned.endsWith('/v0/management')) {
      return cleaned.slice(0, -'/v0/management'.length);
    }
    return cleaned;
  }

  function showMessage(message, type) {
    const background = type === 'error' ? '#4b1f1f' : type === 'success' ? '#183d2a' : '#2d251d';
    const border = type === 'error' ? '#c65746' : type === 'success' ? '#16a34a' : '#8b8680';
    const toast = document.createElement('div');
    toast.textContent = message;
    toast.style.cssText = [
      'position:fixed',
      'top:18px',
      'right:18px',
      'z-index:99999',
      'max-width:420px',
      'padding:14px 16px',
      'border-radius:12px',
      'border:1px solid ' + border,
      'background:' + background,
      'color:#f5f1ea',
      'box-shadow:0 12px 32px rgba(0,0,0,.28)',
      'font-size:14px',
      'line-height:1.5'
    ].join(';');
    document.body.appendChild(toast);
    setTimeout(function () { toast.remove(); }, type === 'success' ? 8000 : 5000);
  }

  async function fetchJSON(base, path, managementKey, method) {
    const response = await fetch(base + path, {
      method: method || 'GET',
      headers: {
        'Authorization': 'Bearer ' + managementKey,
        'Content-Type': 'application/json'
      }
    });

    let data = {};
    try {
      data = await response.json();
    } catch (_) {}

    return { response, data };
  }

  function bindUpgradeButton(button) {
    if (!button || button.dataset.cliProxyUpgradeBound === 'true') return;
    button.dataset.cliProxyUpgradeBound = 'true';

    button.addEventListener('click', async function (event) {
      event.preventDefault();
      event.stopImmediatePropagation();

      const auth = readPersistedAuth();
      const base = normalizeBase(auth.apiBase) || window.location.origin;
      const managementKey = (auth.managementKey || '').trim();

      if (!managementKey) {
        showMessage('未找到当前登录的管理密钥，请重新登录后再尝试升级。', 'error');
        return;
      }

      const originalText = button.textContent || '检查更新';
      button.disabled = true;
      button.textContent = '检查中...';

      try {
        const latest = await fetchJSON(base, VERSION_ROUTE, managementKey, 'GET');
        if (!latest.response.ok) {
          throw new Error(latest.data && latest.data.message ? latest.data.message : '检查更新失败');
        }

        const currentVersion = latest.response.headers.get('x-cpa-version') || '';
        const latestVersion = latest.data['latest-version'] || '';

        if (!latestVersion) {
          throw new Error('未获取到最新版本号');
        }

        if (currentVersion && currentVersion === latestVersion) {
          showMessage('当前已经是最新版本：' + currentVersion, 'success');
          return;
        }

        const confirmed = window.confirm(
          '检测到新版本：' + latestVersion +
          (currentVersion ? '\n当前版本：' + currentVersion : '') +
          '\n\n确认立即一键升级吗？升级会拉取代码、重建 Docker，并导致服务短暂重启。'
        );
        if (!confirmed) {
          return;
        }

        button.textContent = '升级中...';
        const upgrade = await fetchJSON(base, UPGRADE_ROUTE, managementKey, 'POST');
        if (!upgrade.response.ok) {
          throw new Error(upgrade.data && upgrade.data.message ? upgrade.data.message : '触发升级失败');
        }

        showMessage(
          '已开始升级到 ' + latestVersion + '。服务会短暂重启，请等待 20-60 秒后刷新页面。',
          'success'
        );
      } catch (error) {
        console.error(error);
        showMessage(error && error.message ? error.message : '一键升级失败', 'error');
      } finally {
        button.disabled = false;
        button.textContent = originalText;
      }
    }, true);
  }

  function scan() {
    document.querySelectorAll('button').forEach(function (button) {
      const text = (button.textContent || '').trim();
      if (text === '检查更新') {
        bindUpgradeButton(button);
      }
    });
  }

  const observer = new MutationObserver(scan);
  observer.observe(document.documentElement, { childList: true, subtree: true });
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', scan, { once: true });
  } else {
    scan();
  }
})();
</script>
`

func injectManagementUpgradeScript(html string) string {
	if strings.TrimSpace(html) == "" || strings.Contains(html, managementUpgradeInjectionMarker) {
		return html
	}

	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", managementUpgradeInjectionScript+"</body>", 1)
	}

	return html + managementUpgradeInjectionScript
}
