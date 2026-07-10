// AI audit reporter (backup path — primary audit is server-side)
const AIAudit = {
  async report(record) {
    try {
      const res = await API.aiAudit(record);
      return res.ok;
    } catch {
      return false;  // silent fail
    }
  }
};
