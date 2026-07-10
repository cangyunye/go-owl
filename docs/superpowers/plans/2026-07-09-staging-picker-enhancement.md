# 文件中转站增强 Implement Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the staging area (文件中转站) with full path display, single-click path fill, search, and multi-select batch transfer mode.

**Architecture:** All changes are frontend-only in `cmd/plugins/serve/web/js/pages/files.js`. The backend already returns `StagingFile` with `name`, `size`, `mod_time`. `staging_dir` is available from `diskInfo.staging_dir`. Multi-select mode switches the staging table row click behavior: normal mode → fill path input, multi-select mode → toggle checkbox. When user triggers a transfer, checked files are sent sequentially as individual transfer tasks.

**Tech Stack:** Vanilla JS, no dependencies. Existing `api.staging.delete`/`upload`/`files`/`disk` endpoints unchanged. Existing `api.transfer` endpoint reused for multi-file dispatch.

## Global Constraints

- No new CSS classes — use existing `.data-table`, `.btn`, `.btn-ghost`, `.btn-icon`, `.tag`, `input[type="checkbox"]` styles
- All changes in one file: `cmd/plugins/serve/web/js/pages/files.js`
- Backend unchanged
- `staging_dir` path from `diskInfo.staging_dir` (already returned by `api.staging.disk()`)
- `esc()` helper already exists for HTML escaping
- `fmtSize()` helper already exists
- `fmtTime()` helper already exists (added in previous task)

---

### Task 1: Add path column to staging file list

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/files.js:359-367`

**Interfaces:**
- Consumes: `diskInfo.staging_dir` (string, already loaded in `loadStaging`)
- Produces: Staging table with 4 columns: checkbox, filename (+ full path subtitle), mod time, size, actions

- [ ] **Step 1: Update the table header and rows in `renderStaging()`**

Replace the existing table header `<th>文件名</th><th>修改时间</th><th>大小</th><th></th>` with `<th></th><th>文件名</th><th>修改时间</th><th>大小</th><th></th>`.

Replace the table body row template to:
- Prepend a checkbox column (empty for single mode, hidden when not in multi-select)
- Change filename cell to use `.cell-name` pattern: name in bold, full path as `.sub` element below
- Full path = `diskInfo.staging_dir + "/" + f.name`

Code change in `cmd/plugins/serve/web/js/pages/files.js`:

Replace:
```js
list.innerHTML = `<table class="data-table" style="font-size:12px">
  <thead><tr><th>文件名</th><th>修改时间</th><th>大小</th><th></th></tr></thead>
  <tbody>${stagingFiles.map(f => `<tr>
    <td style="font-family:var(--font-mono);max-width:200px;overflow:hidden;text-overflow:ellipsis" title="${esc(f.name)}">${esc(f.name)}</td>
    <td style="color:var(--muted);white-space:nowrap">${fmtTime(f.mod_time)}</td>
    <td style="color:var(--muted);white-space:nowrap;text-align:right">${fmtSize(f.size)}</td>
    <td class="action-cell">${canDelete ? `<button class="btn btn-ghost btn-icon btn-sm staging-delete-btn" data-name="${esc(f.name)}" title="删除"><svg width="14" height="14" aria-hidden="true" style="color:var(--danger)"><use href="#icon-x"/></svg></button>` : ''}</td>
  </tr>`).join('')}</tbody>
</table>`;
```

With:
```js
const stagingDir = diskInfo ? diskInfo.staging_dir : '';
list.innerHTML = `<table class="data-table" style="font-size:12px">
  <thead><tr><th style="width:36px"></th><th>文件名</th><th>修改时间</th><th>大小</th><th></th></tr></thead>
  <tbody>${stagingFiles.map(f => {
    const fullPath = stagingDir ? stagingDir + '/' + f.name : f.name;
    return `<tr class="staging-file-row" data-name="${esc(f.name)}">
      <td class="checkbox-col" style="display:none"><input type="checkbox" class="staging-checkbox" data-name="${esc(f.name)}"></td>
      <td><div class="cell-name"><span>${esc(f.name)}</span><span class="sub">${esc(fullPath)}</span></div></td>
      <td style="color:var(--muted);white-space:nowrap">${fmtTime(f.mod_time)}</td>
      <td style="color:var(--muted);white-space:nowrap;text-align:right">${fmtSize(f.size)}</td>
      <td class="action-cell">${canDelete ? `<button class="btn btn-ghost btn-icon btn-sm staging-delete-btn" data-name="${esc(f.name)}" title="删除"><svg width="14" height="14" aria-hidden="true" style="color:var(--danger)"><use href="#icon-x"/></svg></button>` : ''}</td>
    </tr>`;
  }).join('')}</tbody>
</table>`;
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/files.js
git commit -m "feat(staging): add full path column to staging file list"
```

---

### Task 2: Single-click row to fill local path input

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/files.js` (event handlers in `renderStaging`)

**Interfaces:**
- Consumes: `document.getElementById('src-path')` — the local path input (used when direction is 'push')
- Produces: Row click handler that fills the source path input with the full staging path

- [ ] **Step 1: Add row click handler after rendering staging table**

After the existing `list.querySelectorAll('.staging-delete-btn')` block, add:

```js
list.querySelectorAll('.staging-file-row').forEach(row => {
  row.addEventListener('click', function(e) {
    if (e.target.closest('.staging-delete-btn') || e.target.closest('.staging-checkbox')) return;
    const name = this.dataset.name;
    const stagingDir = diskInfo ? diskInfo.staging_dir : '';
    const fullPath = stagingDir ? stagingDir + '/' + name : name;
    const srcInput = document.getElementById('src-path');
    if (srcInput) {
      srcInput.value = fullPath;
      srcInput.focus();
    }
  });
});
```

This goes inside `renderStaging()`, right after the `list.querySelectorAll('.staging-delete-btn')` loop.

- [ ] **Step 2: Build and verify**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/files.js
git commit -m "feat(staging): click row to fill source path"
```

---

### Task 3: Add file search within staging

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/files.js`

- [ ] **Step 1: Add `stagingSearch` state variable and search input in the staging card**

Add state variable after `let diskInfo = null;`:
```js
let stagingSearch = '';
```

Replace the staging card header section in the `render()` call. Find:
```html
<div class="card">
  <div class="card-header"><h3>文件中转站</h3></div>
```

Change the card header to include a search input:
```html
<div class="card">
  <div class="card-header" style="display:flex;align-items:center;gap:8px">
    <h3 style="flex:1">文件中转站</h3>
    <input type="text" id="staging-search" class="exec-input" placeholder="搜索文件名..." style="width:140px;font-size:12px">
  </div>
```

- [ ] **Step 2: Update `renderStaging()` to filter by `stagingSearch`**

At the top of `renderStaging()`, before rendering the table, add:
```js
const filtered = stagingSearch
  ? stagingFiles.filter(f => f.name.toLowerCase().includes(stagingSearch.toLowerCase()))
  : stagingFiles;
```

Then change all references from `stagingFiles` to `filtered` inside the rendering logic:
- `stagingFiles.map(f => ...)` → `filtered.map(f => ...)`
- `stagingFiles.length === 0` → `filtered.length === 0`

- [ ] **Step 3: Add search input event handler in the init callback**

In the init callback (the function passed to `render()` as second arg), after the staging upload button handler, add:
```js
document.getElementById('staging-search').addEventListener('input', function() {
  stagingSearch = this.value.trim();
  renderStaging();
});
```

- [ ] **Step 4: Build and verify**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/files.js
git commit -m "feat(staging): add file search"
```

---

### Task 4: Multi-select mode for batch transfer

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/files.js`

**Interfaces:**
- Consumes: `api.transfer(payload)` — existing endpoint for single transfer
- Produces: Multi-select mode toggle, checkbox column, batch transfer button

- [ ] **Step 1: Add `stagingMulti` state variable and `multiSelectMode` flag**

Add after `let stagingSearch = '';`:
```js
let stagingMulti = false;
let stagingSelected = new Set();
```

- [ ] **Step 2: Add multi-select toggle button next to upload controls**

Find the upload controls div:
```html
<div style="display:flex;align-items:center;gap:10px;margin-bottom:10px">
  <div style="flex:1;min-width:0">
    ...
  </div>
  <button class="btn btn-secondary btn-sm" id="staging-pick-btn" ...>
```

Add a new button before the pick button:
```html
<button class="btn btn-ghost btn-sm" id="staging-multi-btn" style="white-space:nowrap;flex-shrink:0" title="多选模式">
  <svg width="13" height="13" aria-hidden="true" style="margin-right:3px;vertical-align:-2px"><use href="#icon-check"/></svg>
  多选
</button>
```

Then after the upload button, add a hidden batch transfer button:
```html
<button class="btn btn-primary btn-sm" id="staging-batch-btn" style="white-space:nowrap;flex-shrink:0;display:none">批量传输</button>
```

- [ ] **Step 3: Update `renderStaging()` to show/hide checkboxes based on `stagingMulti`**

In the table template, change the checkbox column visibility:
```js
const showCheck = stagingMulti ? '' : 'style="display:none"';
```

And in the `<td class="checkbox-col">`, replace `style="display:none"` with `${showCheck}`.

At the very end of `renderStaging()`, after the delete button handlers, add checkbox change handler:
```js
list.querySelectorAll('.staging-checkbox').forEach(cb => {
  cb.addEventListener('change', function() {
    const name = this.dataset.name;
    if (this.checked) stagingSelected.add(name);
    else stagingSelected.delete(name);
    const batchBtn = document.getElementById('staging-batch-btn');
    if (batchBtn) {
      const count = stagingSelected.size;
      batchBtn.textContent = count ? `批量传输 (${count})` : '批量传输';
      batchBtn.style.display = stagingMultiSelect && count > 0 ? 'inline-flex' : 'none';
    }
  });
});
```

- [ ] **Step 4: Add event handlers in the init callback**

Add after the staging-search handler:
```js
document.getElementById('staging-multi-btn').addEventListener('click', function() {
  stagingMulti = !stagingMulti;
  if (!stagingMulti) stagingSelected.clear();
  this.classList.toggle('active');
  renderStaging();
});

document.getElementById('staging-batch-btn').addEventListener('click', async function() {
  if (stagingSelected.size === 0) return;
  const srcInput = document.getElementById('src-path');
  const dstInput = document.getElementById('dst-path');
  const dst = dstInput ? dstInput.value.trim() : '';
  const src = srcInput ? srcInput.value.trim() : '';
  if (!dst) { alert('请填写节点路径'); return; }
  if (selectedNodes.size === 0) { alert('请选择目标节点'); return; }
  const stagingDir = diskInfo ? diskInfo.staging_dir : '';
  let success = 0, fail = 0;
  for (const name of stagingSelected) {
    const fullPath = stagingDir ? stagingDir + '/' + name : name;
    try {
      await api.transfer({
        action: 'push',
        node_ids: Array.from(selectedNodes),
        source_path: fullPath,
        dest_path: dst,
        direction: 'push',
      });
      success++;
    } catch (e) {
      fail++;
    }
  }
  alert(`批量传输完成：${success} 成功, ${fail} 失败`);
  stagingSelected.clear();
  stagingMulti = false;
  loadTransfers();
  renderStaging();
});
```

Also update the multi button state on render. After `renderStaging()` in the init callback, add a class toggle:
```js
const multiBtn = document.getElementById('staging-multi-btn');
if (multiBtn && stagingMulti) multiBtn.classList.add('active');
```

- [ ] **Step 5: Build and verify**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/files.js
git commit -m "feat(staging): multi-select mode for batch transfer"
```