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
    <div class="sftp-page" id="sftp-root" style="display:flex;flex-direction:column;gap:12px;position:relative">
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
        <button class="btn btn-primary btn-sm" id="sftp-upload"><svg width="14" height="14"><use href="#icon-arrow-up"/></svg> 上传文件</button>
        <input type="file" id="sftp-file-input" multiple style="display:none"/>
      </div>
      <div id="sftp-breadcrumb" style="display:flex;gap:4px;align-items:center;flex-wrap:wrap;font-size:var(--fs-sm)"></div>
      <div id="sftp-body" class="card" style="padding:0;overflow:auto;min-height:200px">
        <div id="sftp-loading" style="padding:40px;text-align:center;color:var(--muted)">加载中…</div>
      </div>
      <div id="sftp-queue"></div>
      <div id="sftp-drop-hint" style="display:none;position:absolute;inset:0;z-index:30;border:2px dashed var(--accent);border-radius:var(--radius);background:color-mix(in srgb, var(--accent) 8%, transparent);align-items:center;justify-content:center;pointer-events:none;font-weight:600;color:var(--accent)">松开以上传到当前目录</div>
    </div>
  `, () => {
    document.getElementById('sftp-back').addEventListener('click', () => navigate('/nodes'));
    document.getElementById('sftp-up').addEventListener('click', () => load(parentPath(cwd)));
    document.getElementById('sftp-home').addEventListener('click', () => load(''));
    document.getElementById('sftp-refresh').addEventListener('click', () => load(cwd));
    document.getElementById('sftp-mkdir').addEventListener('click', createFolder);

    // —— M5: 拖拽/按钮上传 + 传输队列 + 冲突弹窗（批量决策） ——
    const CONCURRENCY = 3;
    let tasks = [];
    let taskSeq = 0;
    let activeUploads = 0;
    let batchDecision = null; // {mode, newName}：勾选"批量"后应用到后续冲突

    function joinRemote(dir, name) { return (dir === '/' ? '' : dir) + '/' + name; }

    function enqueueFiles(fileList, dir) {
      for (const f of fileList) {
        tasks.push({ id: ++taskSeq, file: f, name: f.name, dir, loaded: 0, total: f.size, status: 'queued', err: '', xhr: null });
      }
      renderQueue();
      runQueue();
    }

    function runQueue() {
      while (activeUploads < CONCURRENCY) {
        const t = tasks.find(x => x.status === 'queued');
        if (!t) break;
        activeUploads++;
        startTask(t);
      }
      renderQueue();
    }

    function doUpload(t, mode, newName) {
      return api.sftpUpload(nodeId, joinRemote(t.dir, t.name), t.file, {
        mode, newName,
        onXhr: x => { t.xhr = x; },
        onProgress: loaded => { t.loaded = loaded; updateQueueRow(t); },
      }).finally(() => { t.xhr = null; });
    }

    async function startTask(t) {
      t.status = 'uploading';
      updateQueueRow(t);
      try {
        if (batchDecision && batchDecision.mode !== 'skip') {
          await doUpload(t, batchDecision.mode, batchDecision.newName);
        } else if (batchDecision && batchDecision.mode === 'skip') {
          t.status = 'skipped';
        } else {
          await doUpload(t);
        }
        t.status = 'done';
      } catch (e) {
        if (e.status === 409) {
          const dec = await conflictModal(t);
          if (!dec) t.status = 'canceled';
          else {
            if (dec.batch) batchDecision = { mode: dec.mode, newName: dec.newName };
            if (dec.mode === 'skip') t.status = 'skipped';
            else {
              try { await doUpload(t, dec.mode, dec.newName); t.status = 'done'; }
              catch (e2) { t.status = 'error'; t.err = e2.aborted ? '已取消' : (e2.message || String(e2)); }
            }
          }
        } else if (e.aborted) {
          t.status = 'canceled';
        } else {
          t.status = 'error';
          t.err = e.message || String(e);
        }
      } finally {
        activeUploads--;
        runQueue();
        if (!tasks.some(x => x.status === 'queued' || x.status === 'uploading')) load(cwd);
      }
    }

    function queueStatusText(t) {
      switch (t.status) {
        case 'queued': return '等待中';
        case 'uploading': return Math.round((t.loaded / Math.max(t.total, 1)) * 100) + '%';
        case 'done': return '完成';
        case 'skipped': return '已跳过';
        case 'canceled': return '已取消';
        case 'error': return '失败: ' + t.err;
        default: return t.status;
      }
    }

    function renderQueue() {
      const el = document.getElementById('sftp-queue');
      if (tasks.length === 0) { el.innerHTML = ''; return; }
      el.innerHTML = `
        <div class="card" style="padding:10px 14px">
          <div style="display:flex;align-items:center;margin-bottom:6px">
            <strong style="font-size:var(--fs-sm)">传输队列</strong>
            <span style="flex:1"></span>
            <button class="btn btn-ghost btn-sm" id="sftp-queue-clear">清除已结束</button>
          </div>
          ${tasks.map(t => `
            <div class="sftp-qrow" data-qid="${t.id}" style="display:flex;align-items:center;gap:10px;padding:4px 0">
              <span style="width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:var(--fs-sm)" title="${esc(t.rel || t.name)}">${esc(t.rel || t.name)}</span>
              <div style="flex:1;height:6px;border-radius:3px;background:var(--border);overflow:hidden">
                <div class="sftp-qbar" style="height:100%;width:${t.status === 'uploading' ? Math.round((t.loaded / Math.max(t.total, 1)) * 100) : (t.status === 'done' ? 100 : 0)}%;background:var(--accent);transition:width .2s"></div>
              </div>
              <span class="sftp-qstatus" style="width:160px;font-size:var(--fs-xs);color:var(--muted);text-align:right">${esc(queueStatusText(t))}</span>
              ${t.status === 'uploading' || t.status === 'queued'
                ? `<button class="btn btn-ghost btn-sm sftp-qcancel" data-qid="${t.id}">×</button>`
                : (t.status === 'error' ? `<button class="btn btn-ghost btn-sm sftp-qretry" data-qid="${t.id}">重试</button>` : '')}
            </div>`).join('')}
        </div>`;
      el.querySelectorAll('.sftp-qcancel').forEach(b => b.addEventListener('click', () => {
        const t = tasks.find(x => x.id === Number(b.dataset.qid));
        if (!t) return;
        if (t.xhr) t.xhr.abort();
        else { t.status = 'canceled'; runQueue(); }
      }));
      el.querySelectorAll('.sftp-qretry').forEach(b => b.addEventListener('click', () => {
        const t = tasks.find(x => x.id === Number(b.dataset.qid));
        if (!t) return;
        t.status = 'queued'; t.err = ''; t.loaded = 0;
        runQueue();
      }));
      el.querySelector('#sftp-queue-clear').addEventListener('click', () => {
        tasks = tasks.filter(t => t.status === 'queued' || t.status === 'uploading');
        renderQueue();
      });
    }

    function updateQueueRow(t) {
      const row = document.querySelector(`.sftp-qrow[data-qid="${t.id}"]`);
      if (!row) return;
      const pct = t.status === 'uploading' ? Math.round((t.loaded / Math.max(t.total, 1)) * 100) : (t.status === 'done' ? 100 : 0);
      row.querySelector('.sftp-qbar').style.width = pct + '%';
      row.querySelector('.sftp-qstatus').textContent = queueStatusText(t);
    }

    function conflictModal(t) {
      return new Promise(resolve => {
        const old = document.getElementById('sftp-conflict-overlay');
        if (old) old.remove();
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay open';
        overlay.id = 'sftp-conflict-overlay';
        overlay.innerHTML = `
          <div class="modal" style="max-width:440px">
            <h3>同名文件已存在</h3>
            <p style="font-size:var(--fs-sm);color:var(--muted);margin:6px 0 12px;font-family:monospace">${esc(joinRemote(t.dir, t.name))}</p>
            <div style="display:flex;flex-direction:column;gap:8px">
              <button class="btn btn-primary" data-c="overwrite">覆盖</button>
              <button class="btn btn-secondary" data-c="auto_rename">自动重命名（加 _序号）</button>
              <button class="btn btn-secondary" data-c="rename">换名上传…</button>
              <button class="btn btn-secondary" data-c="skip">跳过此文件</button>
            </div>
            <label style="display:flex;gap:6px;align-items:center;margin-top:12px;font-size:var(--fs-sm)">
              <input type="checkbox" id="sftp-conflict-batch"/> 对后续冲突批量应用相同选择
            </label>
            <div class="modal-actions"><button class="btn btn-ghost" id="sftp-conflict-cancel">取消该任务</button></div>
          </div>`;
        document.body.appendChild(overlay);
        const done = (v) => { overlay.remove(); resolve(v); };
        const batch = () => overlay.querySelector('#sftp-conflict-batch').checked;
        overlay.querySelector('#sftp-conflict-cancel').addEventListener('click', () => done(null));
        overlay.querySelectorAll('[data-c]').forEach(b => b.addEventListener('click', async () => {
          const mode = b.dataset.c;
          if (mode === 'rename') {
            const newName = await promptModal('换名上传', '新文件名（不含路径）', t.name);
            if (newName) done({ mode: 'rename', newName, batch: batch() });
          } else {
            done({ mode, batch: batch() });
          }
        }));
      });
    }

    // 上传按钮 + 文件选择
    document.getElementById('sftp-upload').addEventListener('click', () => document.getElementById('sftp-file-input').click());
    document.getElementById('sftp-file-input').addEventListener('change', (e) => {
      enqueueFiles(Array.from(e.target.files), cwd);
      e.target.value = '';
    });

    // 拖放（本地文件 → 上传）
    const root = document.getElementById('sftp-root');
    const hint = document.getElementById('sftp-drop-hint');
    let dragDepth = 0;
    const hasFiles = e => e.dataTransfer && Array.from(e.dataTransfer.types || []).includes('Files');
    root.addEventListener('dragenter', (e) => { if (!hasFiles(e)) return; e.preventDefault(); dragDepth++; hint.style.display = 'flex'; });
    root.addEventListener('dragover', (e) => { if (!hasFiles(e)) return; e.preventDefault(); e.dataTransfer.dropEffect = 'copy'; });
    root.addEventListener('dragleave', () => { if (--dragDepth <= 0) { dragDepth = 0; hint.style.display = 'none'; } });
    // —— M6: 文件夹拖入 = 递归内容同步（同名目录默认覆盖，不删除远端多余文件；重名文件走冲突弹窗） ——
    function readAllEntries(dirEntry) {
      const reader = dirEntry.createReader();
      const all = [];
      return new Promise((res, rej) => {
        const batch = () => reader.readEntries(ents => {
          if (!ents.length) { res(all); return; }
          all.push(...ents);
          batch();
        }, rej);
        batch();
      });
    }

    async function walkDrop(entry, parentRemoteDir, relBase) {
      relBase = relBase || '';
      if (entry.isFile) {
        const file = await new Promise((res, rej) => entry.file(res, rej));
        tasks.push({ id: ++taskSeq, file, name: entry.name, rel: relBase + entry.name, dir: parentRemoteDir, loaded: 0, total: file.size, status: 'queued', err: '', xhr: null });
        renderQueue();
        runQueue();
        return;
      }
      if (entry.isDirectory) {
        const dirPath = joinRemote(parentRemoteDir, entry.name);
        try {
          await api.sftpMkdir(nodeId, dirPath);
        } catch (e) {
          if (e.status !== 409) { alert(`创建目录失败 ${dirPath}: ${e.message || e}`); return; }
        }
        const ents = await readAllEntries(entry);
        for (const child of ents) await walkDrop(child, dirPath, relBase + entry.name + '/');
      }
    }

    root.addEventListener('drop', (e) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      dragDepth = 0; hint.style.display = 'none';
      const dir = cwd;
      const items = e.dataTransfer.items ? Array.from(e.dataTransfer.items) : [];
      const entries = items.map(it => it.webkitGetAsEntry && it.webkitGetAsEntry()).filter(Boolean);
      if (entries.length > 0) {
        (async () => {
          for (const en of entries) {
            try { await walkDrop(en, dir); }
            catch (err) { alert('遍历拖入内容失败: ' + (err.message || err)); }
          }
          load(cwd);
        })();
      } else {
        enqueueFiles(Array.from(e.dataTransfer.files), dir);
      }
    });

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

    return () => { tasks.forEach(t => { if (t.xhr) t.xhr.abort(); }); };
  });
}
