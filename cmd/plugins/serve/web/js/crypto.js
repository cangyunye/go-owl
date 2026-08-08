// Crypto wallet for AI page
const CryptoWallet = {
  async encryptApiKey(publicKeySpkiB64, apiKey) {
    // crypto.subtle requires a secure context (HTTPS or localhost)
    if (!crypto.subtle) {
      // Fallback: return plaintext with a marker so the server can detect it
      return '__plain__:' + btoa(apiKey);
    }
    const spkiBytes = Uint8Array.from(atob(publicKeySpkiB64), c => c.charCodeAt(0));
    const publicKey = await crypto.subtle.importKey(
      'spki', spkiBytes, { name: 'RSA-OAEP', hash: 'SHA-256' }, false, ['encrypt']
    );
    const encrypted = await crypto.subtle.encrypt(
      { name: 'RSA-OAEP' }, publicKey, new TextEncoder().encode(apiKey)
    );
    return btoa(String.fromCharCode(...new Uint8Array(encrypted)));
  },

  async deriveKey(userId, salt) {
    if (!crypto.subtle) {
      throw new Error('crypto.subtle unavailable: use HTTPS or localhost');
    }
    const enc = new TextEncoder();
    const keyMaterial = await crypto.subtle.importKey(
      'raw', enc.encode(userId + ':' + salt),
      { name: 'PBKDF2' }, false, ['deriveKey']
    );
    return crypto.subtle.deriveKey(
      { name: 'PBKDF2', salt: enc.encode(salt), iterations: 100000, hash: 'SHA-256' },
      keyMaterial, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt']
    );
  },

  async encryptLocal(data, userId) {
    if (!crypto.subtle) {
      // Fallback: store unencrypted when crypto.subtle unavailable
      return { __plain__: true, data: data };
    }
    // Generate a random UUID (crypto.randomUUID() not available everywhere)
    const salt = crypto.randomUUID ? crypto.randomUUID() : (() => {
      const bytes = crypto.getRandomValues(new Uint8Array(16));
      bytes[6] = (bytes[6] & 0x0f) | 0x40;
      bytes[8] = (bytes[8] & 0x3f) | 0x80;
      const h = (b) => b.toString(16).padStart(2, '0');
      return h(bytes[0])+h(bytes[1])+h(bytes[2])+h(bytes[3])+'-'+
             h(bytes[4])+h(bytes[5])+'-'+h(bytes[6])+h(bytes[7])+'-'+
             h(bytes[8])+h(bytes[9])+'-'+h(bytes[10])+h(bytes[11])+h(bytes[12])+h(bytes[13])+h(bytes[14])+h(bytes[15]);
    })();
    const key = await this.deriveKey(userId, salt);
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const encoded = new TextEncoder().encode(JSON.stringify(data));
    const encrypted = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, encoded);
    return {
      salt,
      iv: btoa(String.fromCharCode(...iv)),
      data: btoa(String.fromCharCode(...new Uint8Array(encrypted)))
    };
  },

  async decryptLocal(packet, userId) {
    if (packet.__plain__) {
      return packet.data;
    }
    const key = await this.deriveKey(userId, packet.salt);
    const iv = Uint8Array.from(atob(packet.iv), c => c.charCodeAt(0));
    const data = Uint8Array.from(atob(packet.data), c => c.charCodeAt(0));
    const decrypted = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, data);
    return JSON.parse(new TextDecoder().decode(decrypted));
  }
};

// Expose to global scope so ES modules (app.js) can access it
window.CryptoWallet = CryptoWallet;
