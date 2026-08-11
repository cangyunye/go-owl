// AI conversation + API key storage
const AIStorage = {
  DB_NAME: 'owl_ai_chat',
  DB_VERSION: 2,

  async openDb() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(this.DB_NAME, this.DB_VERSION);
      req.onupgradeneeded = (e) => {
        const db = e.target.result;
        if (!db.objectStoreNames.contains('conversations')) {
          const store = db.createObjectStore('conversations', { keyPath: 'id' });
          store.createIndex('created_at', 'createdAt', { unique: false });
          store.createIndex('user_id', 'userId', { unique: false });
        } else {
          const store = req.transaction.objectStore('conversations');
          if (!store.indexNames.contains('user_id')) {
            store.createIndex('user_id', 'userId', { unique: false });
          }
        }
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
