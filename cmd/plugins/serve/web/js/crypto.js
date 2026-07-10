// Crypto wallet for AI page
const CryptoWallet = {
  async encryptApiKey(publicKeySpkiB64, apiKey) {
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
    const salt = crypto.randomUUID();
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
    const key = await this.deriveKey(userId, packet.salt);
    const iv = Uint8Array.from(atob(packet.iv), c => c.charCodeAt(0));
    const data = Uint8Array.from(atob(packet.data), c => c.charCodeAt(0));
    const decrypted = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, data);
    return JSON.parse(new TextDecoder().decode(decrypted));
  }
};
