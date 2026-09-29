/* go-owl landing — 交互逻辑：语言切换 / 版本号拉取 / 终端动画 / 截图 tab / 复制按钮 / 导航 */
(function () {
  'use strict';

  var DICT = window.OWL_I18N;
  var LANG_KEY = 'owl_site_lang';
  var FALLBACK_VERSION = 'v1.10.0';

  var state = { lang: 'zh' };

  /* ---------- 语言 ---------- */

  function detectLang() {
    var stored = null;
    try { stored = localStorage.getItem(LANG_KEY); } catch (e) { /* 隐私模式 */ }
    if (stored === 'zh' || stored === 'en') return stored;
    return (navigator.language || 'en').toLowerCase().indexOf('zh') === 0 ? 'zh' : 'en';
  }

  function applyLang(lang) {
    state.lang = lang;
    document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
    var dict = DICT[lang] || DICT.zh;

    document.querySelectorAll('[data-i18n]').forEach(function (el) {
      var key = el.getAttribute('data-i18n');
      var val = dict[key];
      if (val == null) return;
      if (val.indexOf('<') !== -1) el.innerHTML = val;
      else el.textContent = val;
    });

    var toggle = document.getElementById('lang-toggle');
    if (toggle) toggle.textContent = lang === 'zh' ? 'EN' : '中文';

    updateShotAlt();
    updateReleaseVersions(); // ctaDownload 按 innerHTML 注入后需刷新版本占位
    try { localStorage.setItem(LANG_KEY, lang); } catch (e) { /* ignore */ }
  }

  /* ---------- 版本号（GitHub API，失败回退硬编码） ---------- */

  function updateReleaseVersions() {
    var ver = window.__owlLatestVersion || FALLBACK_VERSION;
    document.querySelectorAll('[data-release-version]').forEach(function (el) {
      el.textContent = ver;
    });
  }

  function fetchLatestVersion() {
    if (location.protocol === 'file:') return; // 本地 file:// 预览时跳过
    fetch('https://api.github.com/repos/cangyunye/go-owl/releases/latest')
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (data) {
        if (data && data.tag_name && /^v/.test(data.tag_name)) {
          window.__owlLatestVersion = data.tag_name;
          updateReleaseVersions();
        }
      })
      .catch(function () { /* 网络受限时保持回退版本 */ });
  }

  /* ---------- 终端打字动画 ---------- */

  var SCRIPT = [
    { t: 'cmd', text: 'owl exec run "uptime" --groups web' },
    { t: 'out', text: '' },
    { t: 'ok',  text: '✔ web-01   14:32:01 up 42 days,  load: 0.08' },
    { t: 'ok',  text: '✔ web-02   14:32:01 up 128 days, load: 0.31' },
    { t: 'ok',  text: '✔ db-01    14:32:02 up 89 days,  load: 0.12' },
    { t: 'out', text: '' },
    { t: 'cmd', text: 'owl ai "哪些节点磁盘使用率超过 80%？"' },
    { t: 'out', text: '' },
    { t: 'accent', text: '→ web-03: /dev/sda1 使用率 87%，建议清理 /var/log/*.gz' }
  ];

  var REDUCED_MOTION = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  function renderStaticTerminal() {
    var body = document.getElementById('terminal-body');
    if (!body) return;
    body.innerHTML = SCRIPT.map(function (line) {
      if (line.t === 'cmd') {
        return '<span class="t-prompt">$ </span><span class="t-cmd">' + line.text + '</span>';
      }
      return '<span class="t-' + line.t + '">' + line.text + '</span>';
    }).join('\n');
  }

  function runTerminal() {
    var body = document.getElementById('terminal-body');
    if (!body) return;
    if (REDUCED_MOTION) { renderStaticTerminal(); return; }

    var li = 0, ci = 0, lineEls = [], paused = false;

    function newLine(cls) {
      var span = document.createElement('span');
      if (cls) span.className = cls;
      body.appendChild(span);
      body.appendChild(document.createTextNode('\n'));
      return span;
    }

    function step() {
      if (li >= SCRIPT.length) {
        setTimeout(function () {
          body.innerHTML = '';
          lineEls = []; li = 0; ci = 0;
          paused = false;
          setTimeout(step, 600);
        }, 5200);
        return;
      }

      var line = SCRIPT[li];
      if (line.t === 'cmd') {
        if (ci === 0) {
          lineEls[li] = newLine('t-cmd');
          var prompt = document.createElement('span');
          prompt.className = 't-prompt';
          prompt.textContent = '$ ';
          body.insertBefore(prompt, lineEls[li]);
          var caret = document.createElement('span');
          caret.className = 't-caret';
          lineEls[li].after(caret);
          lineEls[li]._caret = caret;
        }
        ci++;
        lineEls[li].textContent = line.text.slice(0, ci);
        if (ci >= line.text.length) {
          if (lineEls[li]._caret) lineEls[li]._caret.remove();
          li++; ci = 0;
          setTimeout(step, 420);
        } else {
          setTimeout(step, 34 + Math.random() * 46);
        }
      } else {
        newLine(line.t === 'out' ? 't-out' : 't-' + line.t).textContent = line.text;
        li++;
        setTimeout(step, line.t === 'out' && line.text === '' ? 160 : 260);
      }
    }

    // 页面不可见时暂停，避免无效计时堆积
    document.addEventListener('visibilitychange', function () {
      paused = document.hidden;
    });
    (function loop() {
      if (!paused) step();
      setTimeout(loop, 400);
    })();
  }

  /* ---------- 截图 tab ---------- */

  var SHOTS = [
    { src: 'assets/screenshots/02-dashboard.png',      alt: { zh: 'go-owl 控制台仪表盘', en: 'go-owl console dashboard' } },
    { src: 'assets/screenshots/08-exec-command.png',   alt: { zh: 'go-owl 批量命令执行', en: 'go-owl batch execution' } },
    { src: 'assets/screenshots/14-playbook-detail.png', alt: { zh: 'go-owl 剧本编排', en: 'go-owl playbooks' } },
    { src: 'assets/screenshots/18-ai-chat.png',        alt: { zh: 'go-owl AI 助手', en: 'go-owl AI assistant' } },
    { src: 'assets/screenshots/19-alerts.png',         alt: { zh: 'go-owl 告警中心', en: 'go-owl alerts' } },
    { src: 'assets/screenshots/07-node-detail.png',    alt: { zh: 'go-owl 节点详情', en: 'go-owl node detail' } }
  ];

  function setupShots() {
    var img = document.getElementById('shot-img');
    if (!img) return;
    // 预加载其余截图，切换零闪烁
    SHOTS.slice(1).forEach(function (s) { new Image().src = s.src; });

    document.querySelectorAll('.shot-tabs [data-shot]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var idx = parseInt(btn.getAttribute('data-shot'), 10);
        if (!SHOTS[idx]) return;
        document.querySelectorAll('.shot-tabs [data-shot]').forEach(function (b) {
          b.setAttribute('aria-selected', String(b === btn));
        });
        img.src = SHOTS[idx].src;
        img.alt = SHOTS[idx].alt[state.lang];
      });
    });
  }

  function updateShotAlt() {
    var img = document.getElementById('shot-img');
    if (!img) return;
    var idx = parseInt(
      (document.querySelector('.shot-tabs [aria-selected="true"]') || {}).dataset
        ? document.querySelector('.shot-tabs [aria-selected="true"]').dataset.shot : '0', 10);
    if (SHOTS[idx]) img.alt = SHOTS[idx].alt[state.lang];
  }

  /* ---------- 复制按钮 ---------- */

  function setupCopy() {
    document.querySelectorAll('.copy-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var code = btn.parentElement.querySelector('code');
        if (!code) return;
        var text = code.textContent;
        var done = function () {
          var dict = DICT[state.lang] || DICT.zh;
          btn.textContent = dict['common.copied'];
          btn.classList.add('copied');
          setTimeout(function () {
            btn.textContent = dict['common.copy'];
            btn.classList.remove('copied');
          }, 1600);
        };
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(done, function () { fallbackCopy(text, done); });
        } else {
          fallbackCopy(text, done);
        }
      });
    });
  }

  function fallbackCopy(text, done) {
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); done(); } catch (e) { /* ignore */ }
    ta.remove();
  }

  /* ---------- 导航：移动端菜单 + 滚动高亮 ---------- */

  function setupNav() {
    var burger = document.getElementById('nav-burger');
    var links = document.getElementById('nav-links');
    if (burger && links) {
      burger.addEventListener('click', function () {
        var open = links.classList.toggle('open');
        burger.setAttribute('aria-expanded', String(open));
      });
      links.addEventListener('click', function (e) {
        if (e.target.tagName === 'A') {
          links.classList.remove('open');
          burger.setAttribute('aria-expanded', 'false');
        }
      });
    }

    var navAnchors = Array.prototype.slice.call(document.querySelectorAll('#nav-links a'));
    var sections = navAnchors
      .map(function (a) { return document.querySelector(a.getAttribute('href')); })
      .filter(Boolean);

    if ('IntersectionObserver' in window && sections.length) {
      var spy = new IntersectionObserver(function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          navAnchors.forEach(function (a) {
            a.classList.toggle('active', a.getAttribute('href') === '#' + entry.target.id);
          });
        });
      }, { rootMargin: '-40% 0px -55% 0px' });
      sections.forEach(function (s) { spy.observe(s); });
    }
  }

  /* ---------- 启动 ---------- */

  document.addEventListener('DOMContentLoaded', function () {
    applyLang(detectLang());
    fetchLatestVersion();
    setupShots();
    setupCopy();
    setupNav();
    runTerminal();

    var toggle = document.getElementById('lang-toggle');
    if (toggle) {
      toggle.addEventListener('click', function () {
        applyLang(state.lang === 'zh' ? 'en' : 'zh');
      });
    }
  });
})();
