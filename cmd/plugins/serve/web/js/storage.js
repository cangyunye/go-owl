// AI conversation + API key storage
const AIStorage = {
  DB_NAME: 'owl_ai_chat',
  DB_VERSION: 3,

  async openDb() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(this.DB_NAME, this.DB_VERSION);
      req.onupgradeneeded = (e) => {
        const db = e.target.result;
        let store;
        if (!db.objectStoreNames.contains('conversations')) {
          store = db.createObjectStore('conversations', { keyPath: 'id' });
        } else {
          store = e.target.transaction.objectStore('conversations');
        }
        // 幂等确保各索引存在（老库升级到 v3 时补建复合索引）
        const ensure = (name, keyPath) => {
          if (!store.indexNames.contains(name)) {
            store.createIndex(name, keyPath, { unique: false });
          }
        };
        ensure('created_at', 'createdAt');
        ensure('user_id', 'userId');
        // 复合索引：按用户 + 时间倒序游标分页（浏览列表只读一页）
        ensure('user_created', ['userId', 'createdAt']);
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  },

  async saveConversation(conv, userId) {
    if (userId) conv.userId = userId;
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readwrite');
      tx.objectStore('conversations').put(conv);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  },

  async getConversations(userId, limit = 50, offset = 0) {
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readonly');
      const store = tx.objectStore('conversations');
      const index = store.index('user_id');
      const results = [];
      const req = index.openCursor(userId ? IDBKeyRange.only(userId) : null);
      req.onsuccess = () => {
        const cursor = req.result;
        if (!cursor) {
          results.sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt));
          resolve(results.slice(offset, offset + limit));
          return;
        }
        results.push(cursor.value);
        cursor.continue();
      };
      req.onerror = () => reject(req.error);
    });
  },

  async deleteConversation(id) {
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readwrite');
      tx.objectStore('conversations').delete(id);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  },

  // 按主键取单条（点开某会话时才读其消息）。
  async getConversation(id) {
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readonly');
      const req = tx.objectStore('conversations').get(id);
      req.onsuccess = () => resolve(req.result || null);
      req.onerror = () => reject(req.error);
    });
  },

  // 游标分页：用复合索引 [userId, createdAt] 倒序取一页（只读该页记录，不加载全部）。
  // beforeCreatedAt 为上一页末条的 createdAt（不含），用于取更早一页；null 表示第一页。
  // 返回 { items, nextCursor }：取满 limit 才给 nextCursor，否则为 null 表示到底。
  async getConversationsPage(userId, limit = 30, beforeCreatedAt = null) {
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readonly');
      const index = tx.objectStore('conversations').index('user_created');
      const lower = [userId, ''];
      const upper = [userId, beforeCreatedAt || '\uffff'];
      const range = IDBKeyRange.bound(lower, upper, false, !!beforeCreatedAt);
      const items = [];
      let nextCursor = null;
      const req = index.openCursor(range, 'prev');
      req.onsuccess = () => {
        const cursor = req.result;
        if (!cursor) {
          resolve({ items, nextCursor });
          return;
        }
        if (items.length >= limit) {
          nextCursor = items[items.length - 1].createdAt;
          resolve({ items, nextCursor });
          return;
        }
        items.push(cursor.value);
        cursor.continue();
      };
      req.onerror = () => reject(req.error);
    });
  },

  // 搜索：标题（首条消息）或任一消息正文命中（忽略大小写）。
  // 需要覆盖全部会话，故一次性全量扫描后过滤（偶发操作）。
  async searchConversations(userId, query) {
    const q = String(query || '').toLowerCase();
    const all = await this.getConversations(userId, Number.MAX_SAFE_INTEGER, 0);
    if (!q) return all;
    return all.filter((c) => {
      const first = c.messages && c.messages[0] && c.messages[0].content;
      if (first && String(first).toLowerCase().includes(q)) return true;
      return (c.messages || []).some(m => String(m.content || '').toLowerCase().includes(q));
    });
  },

  // API Key in localStorage (encrypted)
  async saveApiKey(userId, apiKey, provider, model, baseUrl, apiFormat) {
    const packet = await CryptoWallet.encryptLocal({ apiKey, provider, model, baseUrl, apiFormat }, userId);
    localStorage.setItem('owl_ai_key', JSON.stringify(packet));
  },

  async loadApiKey(userId) {
    const raw = localStorage.getItem('owl_ai_key');
    if (!raw) return null;
    try {
      const packet = JSON.parse(raw);
      return await CryptoWallet.decryptLocal(packet, userId);
    } catch {
      localStorage.removeItem('owl_ai_key');
      return null;
    }
  }
};

// Expose to global scope so ES modules (app.js) can access it
window.AIStorage = AIStorage;
