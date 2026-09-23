export function renderSftp(render, navigate, user, api, nodeId) {
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  const IC = {
    folder: '<svg width="14" height="14"><use href="#icon-folder"/></svg>',
    file: '<svg width="14" height="14"><use href="#icon-file"/></svg>',
  };

  function fmtSize(n) {
    if (n == null) return '-';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB';
    return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
  }

  function parentPath(p) {
    if (p === '/' || !p) return '/';
    const idx = p.lastIndexOf('/');
    return idx <= 0 ? '/' : p.slice(0, idx);
  }

  let cwd = '';

  render(`
    <div class="sftp-page" style="display:flex;flex-direction:column;gap:12px">
      <div class="sftp-header" style="display:flex;align-items:center;gap:10px;flex-wrap:wrap">
        <button class="btn btn-ghost btn-sm" id="sftp-back">← 返回节点</button>
        <svg width="18" height="18" style="color:var(--accent)"><use href="#icon-hard-drive"/></svg>
        <strong>${esc(nodeId)}</strong>
        <span id="sftp-cwd" style="color:var(--muted);font-family:monospace;font-size:var(--fs-sm)"></span>
        <span style="flex:1"></span>
        <button class="btn btn-ghost btn-sm" id="sftp-up" data-tip="上级目录"><svg width="14" height="14"><use href="#icon-arrow-up"/></svg></button>
        <button class="btn btn-ghost btn-sm" id="sftp-home" data-tip="主目录"><svg width="14" height="14"><use href="#icon-home"/></svg></button>
        <button class="btn btn-ghost btn-sm" id="sftp-refresh" data-tip="刷新"><svg width="14" height="14"><use href="#icon-refresh"/></svg></button>
        <button class="btn btn-secondary btn-sm" id="sftp-mkdir">新建文件夹</button>
      </div>
      <div id="sftp-breadcrumb" style="display:flex;gap:4px;align-items:center;flex-wrap:wrap;font-size:var(--fs-sm)"></div>
      <div id="sftp-body" class="card" style="padding:0;overflow:auto;min-height:200px">
        <div id="sftp-loading" style="padding:40px;text-align:center;color:var(--muted)">加载中…</div>
      </div>
    </div>
  `, () => {
    document.getElementById('sftp-back').addEventListener('click', () => navigate('/nodes'));
    document.getElementById('sftp-up').addEventListener('click', () => load(parentPath(cwd)));
    document.getElementById('sftp-home').addEventListener('click', () => load(''));
    document.getElementById('sftp-refresh').addEventListener('click', () => load(cwd));
    document.getElementById('sftp-mkdir').addEventListener('click', createFolder);

    async function load(path) {
      const body = document.getElementById('sftp-body');
      body.innerHTML = '<div style="padding:40px;text-align:center;color:var(--muted)">加载中…</div>';
      let res;
      try {
        res = await api.sftpLs(nodeId, path);
      } catch (e) {
        body.innerHTML = `<div style="padding:40px;text-align:center;color:var(--danger)">读取失败: ${esc(e.message || e)}</div>`;
        return;
      }
      cwd = res.path || '/';
      document.getElementById('sftp-cwd').textContent = cwd;
      renderBreadcrumb();
      renderItems(res.items || []);
    }

    function renderBreadcrumb() {
      const el = document.getElementById('sftp-breadcrumb');
      const parts = cwd.split('/').filter(Boolean);
      let acc = '';
      const crumbs = [`<button class="btn btn-ghost btn-sm sftp-crumb" data-path="/">/</button>`];
      for (const p of parts) {
        acc += '/' + p;
        crumbs.push(`<span style="color:var(--muted)">/</span><button class="btn btn-ghost btn-sm sftp-crumb" data-path="${esc(acc)}">${esc(p)}</button>`);
      }
      el.innerHTML = crumbs.join('');
      el.querySelectorAll('.sftp-crumb').forEach(b => b.addEventListener('click', () => load(b.dataset.path)));
    }

    function renderItems(items) {
      const body = document.getElementById('sftp-body');
      const sorted = items.slice().sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : (a.is_dir ? -1 : 1)));
      if (sorted.length === 0) {
        body.innerHTML = '<div style="padding:40px;text-align:center;color:var(--muted)">空目录</div>';
        return;
      }
      body.innerHTML = `
        <table class="data-table" style="width:100%">
          <thead><tr><th>名称</th><th style="width:90px;text-align:right">大小</th><th style="width:150px">修改时间</th><th style="width:140px;text-align:right">操作</th></tr></thead>
          <tbody>
            ${sorted.map(it => `
              <tr data-path="${esc(it.path)}" data-dir="${it.is_dir ? 1 : 0}" data-name="${esc(it.name)}" style="cursor:pointer">
                <td>${it.is_dir ? IC.folder : IC.file} <span style="margin-left:6px">${esc(it.name)}</span></td>
                <td style="text-align:right;color:var(--muted)">${it.is_dir ? '-' : fmtSize(it.size)}</td>
                <td style="color:var(--muted);font-size:var(--fs-sm)">${it.mtime ? new Date(it.mtime * 1000).toLocaleString() : '-'}</td>
                <td style="text-align:right;white-space:nowrap">
                  <button class="btn btn-ghost btn-sm sftp-dl" data-tip="${it.is_dir ? '打包下载 (.tar.gz)' : '下载'}"><svg width="14" height="14"><use href="#icon-download"/></svg></button>
                  <button class="btn btn-ghost btn-sm sftp-ren" data-tip="重命名"><svg width="14" height="14"><use href="#icon-edit"/></svg></button>
                  <button class="btn btn-ghost btn-sm sftp-del" data-tip="删除" style="color:var(--danger)"><svg width="14" height="14"><use href="#icon-trash"/></svg></button>
                </td>
              </tr>`).join('')}
          </tbody>
        </table>`;
      body.querySelectorAll('tbody tr').forEach(tr => {
        const p = tr.dataset.path;
        tr.addEventListener('dblclick', () => { if (tr.dataset.dir === '1') load(p); });
        tr.querySelector('.sftp-dl').addEventListener('click', (e) => {
          e.stopPropagation();
          if (tr.dataset.dir === '1') api.downloadSftpArchive(nodeId, p).catch(err => alert('打包下载失败: ' + (err.message || err)));
          else api.downloadSftpFile(nodeId, p).catch(err => alert('下载失败: ' + (err.message || err)));
        });
        tr.querySelector('.sftp-ren').addEventListener('click', (e) => { e.stopPropagation(); renameEntry(p, tr.dataset.name); });
        tr.querySelector('.sftp-del').addEventListener('click', (e) => { e.stopPropagation(); deleteEntry(p, tr.dataset.dir === '1', tr.dataset.name); });
      });
    }

    async function createFolder() {
      const name = await promptModal('新建文件夹', '文件夹名称', '');
      if (!name) return;
      const target = (cwd === '/' ? '' : cwd) + '/' + name;
      try {
        await api.sftpMkdir(nodeId, target);
        load(cwd);
      } catch (e) {
        if (e.status === 409) alert('同名条目已存在');
        else alert('创建失败: ' + (e.message || e));
      }
    }

    async function renameEntry(p, oldName) {
      const newName = await promptModal('重命名', '新名称', oldName);
      if (!newName || newName === oldName) return;
      const target = (cwd === '/' ? '' : cwd) + '/' + newName;
      try {
        await api.sftpRename(nodeId, p, target);
        load(cwd);
      } catch (e) {
        if (e.status === 409) alert('目标名称已存在');
        else alert('重命名失败: ' + (e.message || e));
      }
    }

    async function deleteEntry(p, isDir, name) {
      if (!confirm(`确定删除「${name}」？${isDir ? '（目录）' : ''}`)) return;
      try {
        await api.sftpDelete(nodeId, p, false);
        load(cwd);
      } catch (e) {
        if (e.status === 409 && isDir) {
          if (confirm(`目录「${name}」非空，是否递归删除其全部内容？`)) {
            try { await api.sftpDelete(nodeId, p, true); load(cwd); }
            catch (e2) { alert('删除失败: ' + (e2.message || e2)); }
          }
          return;
        }
        alert('删除失败: ' + (e.message || e));
      }
    }

    load('');

    function promptModal(title, label, initial) {
      return new Promise(resolve => {
        const old = document.getElementById('sftp-prompt-overlay');
        if (old) old.remove();
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay open';
        overlay.id = 'sftp-prompt-overlay';
        overlay.innerHTML = `
          <div class="modal" style="max-width:400px">
            <h3>${esc(title)}</h3>
            <div style="margin:12px 0">
              <label style="font-size:var(--fs-sm);color:var(--muted)">${esc(label)}</label>
              <input class="input" id="sftp-prompt-input" style="width:100%;margin-top:6px" value="${esc(initial)}" autocomplete="off"/>
            </div>
            <div class="modal-actions">
              <button class="btn btn-secondary" id="sftp-prompt-cancel">取消</button>
              <button class="btn btn-primary" id="sftp-prompt-ok">确定</button>
            </div>
          </div>`;
        document.body.appendChild(overlay);
        const input = overlay.querySelector('#sftp-prompt-input');
        input.focus();
        input.select();
        const done = (v) => { overlay.remove(); resolve(v); };
        overlay.addEventListener('click', (e) => { if (e.target === overlay) done(null); });
        overlay.querySelector('#sftp-prompt-cancel').addEventListener('click', () => done(null));
        overlay.querySelector('#sftp-prompt-ok').addEventListener('click', () => done(input.value.trim() || null));
        input.addEventListener('keydown', (e) => {
          if (e.key === 'Enter') done(input.value.trim() || null);
          if (e.key === 'Escape') done(null);
        });
      });
    }
  });
}
