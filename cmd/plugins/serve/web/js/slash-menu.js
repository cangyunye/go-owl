// SlashMenu — AI 会话输入框的斜杠命令补全。
// 仅当输入以 "/" 开头时弹出下拉,展示命令目录并支持键盘导航。
// 选中任务类命令 → 展开为提示词模板;导航/系统类命令 → 直接执行动作。
// 命令目录由调用方(ai.js)通过 { commands } 传入,本组件保持通用。

export class SlashMenu {
  constructor(textarea, { commands = [], container } = {}) {
    this.textarea = textarea;
    this.commands = commands;
    this.container = container || textarea.parentElement;
    this.menu = null;
    this.items = [];
    this.activeIdx = -1;

    this._onInput = this.refresh.bind(this);
    this._onKeyDown = this.onKeyDown.bind(this);
    this._onBlur = () => this.close();
    this._onDocClick = (e) => {
      if (this.menu && !this.menu.contains(e.target)) this.close();
    };

    textarea.addEventListener('input', this._onInput);
    textarea.addEventListener('keydown', this._onKeyDown);
    textarea.addEventListener('blur', this._onBlur);
    document.addEventListener('click', this._onDocClick);
  }

  destroy() {
    this.textarea.removeEventListener('input', this._onInput);
    this.textarea.removeEventListener('keydown', this._onKeyDown);
    this.textarea.removeEventListener('blur', this._onBlur);
    document.removeEventListener('click', this._onDocClick);
    this.close();
  }

  isOpen() {
    return !!this.menu;
  }

  refresh() {
    const value = this.textarea.value;
    if (!value.startsWith('/')) {
      this.close();
      return;
    }
    const query = value.slice(1).trim().toLowerCase();
    const items = query
      ? this.commands.filter((c) => c.name.toLowerCase().startsWith(query))
      : this.commands;
    if (items.length === 0) {
      this.close();
      return;
    }
    this.items = items;
    this.activeIdx = 0;
    this.render();
  }

  render() {
    if (this.menu) this.menu.remove();

    const menu = document.createElement('div');
    menu.className = 'ai-slash-menu';
    menu.setAttribute('role', 'listbox');
    menu.setAttribute('aria-label', '命令补全');
    this.menu = menu;
    this.container.appendChild(menu);

    this.items.forEach((cmd, i) => {
      const item = document.createElement('div');
      item.className = 'ai-slash-item' + (i === this.activeIdx ? ' active' : '');
      item.setAttribute('role', 'option');
      item.dataset.idx = String(i);
      item.innerHTML =
        '<span class="ai-slash-icon">' + cmd.icon + '</span>' +
        '<span class="ai-slash-name">/' + esc(cmd.name) + '</span>' +
        '<span class="ai-slash-label">' + esc(cmd.label) + '</span>' +
        '<span class="ai-slash-desc">' + esc(cmd.desc || '') + '</span>' +
        '<span class="ai-slash-tag">' + (cmd.category === 'task' ? '任务' : '导航') + '</span>';
      item.addEventListener('mousedown', (e) => {
        e.preventDefault();
        this.activeIdx = i;
        this.select();
      });
      item.addEventListener('mouseenter', () => {
        this.activeIdx = i;
        this.highlight();
      });
      menu.appendChild(item);
    });
  }

  highlight() {
    if (!this.menu) return;
    Array.from(this.menu.children).forEach((el, i) => {
      el.classList.toggle('active', i === this.activeIdx);
    });
  }

  onKeyDown(e) {
    if (!this.menu) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.activeIdx = (this.activeIdx + 1) % this.items.length;
      this.highlight();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this.activeIdx = (this.activeIdx - 1 + this.items.length) % this.items.length;
      this.highlight();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      this.select();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      this.close();
    }
  }

  select() {
    const cmd = this.items[this.activeIdx];
    this.close();
    if (!cmd) return;
    if (cmd.template) {
      this.applyTemplate(cmd);
    } else if (typeof cmd.action === 'function') {
      cmd.action(this.textarea);
    }
  }

  applyTemplate(cmd) {
    const ta = this.textarea;
    ta.value = cmd.template;
    ta.focus();
    const firstArg = cmd.args && cmd.args[0];
    if (firstArg) {
      const token = '{' + firstArg + '}';
      const idx = ta.value.indexOf(token);
      if (idx >= 0) {
        ta.setSelectionRange(idx, idx + token.length);
      }
    }
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }

  close() {
    if (this.menu) {
      this.menu.remove();
      this.menu = null;
    }
    this.items = [];
    this.activeIdx = -1;
  }
}

function esc(s) {
  return String(s).replace(/[&<>"]/g, (m) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[m]));
}
