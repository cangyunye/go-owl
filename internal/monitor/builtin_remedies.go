package monitor

// builtinRemedies 内置对策种子：随二进制发布，均为安全的人工排查指引（sop）。
// 可自动执行的脚本类对策由用户提供或 AI 生成（须审核）。
var builtinRemedies = []Remedy{
	{
		ID: "R-DSK-001-SOP", AlertTypeID: "OWL-DSK-001",
		Name: "磁盘使用率过高排查指引", Kind: "sop", Risk: "low", Source: "builtin", Reviewed: true,
		Content: "1. du -h --max-depth=1 / | sort -h 定位大目录\n" +
			"2. 清理应用日志（journalctl --vacuum-time=7d、logrotate）\n" +
			"3. 清理 /tmp 与旧内核（uname -r 对比 /boot）\n" +
			"4. 仍不足则评估挂载点扩容",
	},
	{
		ID: "R-DSK-002-SOP", AlertTypeID: "OWL-DSK-002",
		Name: "inode 使用率过高排查指引", Kind: "sop", Risk: "low", Source: "builtin", Reviewed: true,
		Content: "1. df -i 定位 inode 耗尽的挂载点\n" +
			"2. find / -xdev -type f | wc -l 统计小文件数量\n" +
			"3. 清理邮件队列/缓存小文件（如 /var/spool、/tmp 下大量小文件）",
	},
	{
		ID: "R-MEM-001-SOP", AlertTypeID: "OWL-MEM-001",
		Name: "内存使用率过高排查指引", Kind: "sop", Risk: "low", Source: "builtin", Reviewed: true,
		Content: "1. ps aux --sort=-%mem | head -20 定位高占用进程\n" +
			"2. 结合业务窗口判断是否需重启应用释放内存\n" +
			"3. 持续增长疑似泄漏：对比连续采样，评估重启或扩容",
	},
	{
		ID: "R-CPU-001-SOP", AlertTypeID: "OWL-CPU-001",
		Name: "负载持续过高排查指引", Kind: "sop", Risk: "low", Source: "builtin", Reviewed: true,
		Content: "1. top 确认 CPU/IO/进程状态（%wa 高则查磁盘 IO）\n" +
			"2. 结合业务高峰期判断是否正常波动\n" +
			"3. 长期偏高评估扩容或拆分任务",
	},
	{
		ID: "R-SVC-001-SOP", AlertTypeID: "OWL-SVC-001",
		Name: "服务停止排查指引", Kind: "sop", Risk: "medium", Source: "builtin", Reviewed: true,
		Content: "1. systemctl status <svc> 查看失败原因\n" +
			"2. journalctl -u <svc> -n 100 排查启动错误\n" +
			"3. 修复配置后 systemctl restart <svc>，确认 active\n" +
			"注意：涉及生产服务的重启需按变更流程执行",
	},
	{
		ID: "R-ERR-002-SOP", AlertTypeID: "OWL-ERR-002",
		Name: "OOM 事件排查指引", Kind: "sop", Risk: "low", Source: "builtin", Reviewed: true,
		Content: "1. dmesg | grep -i 'out of memory' 确认被杀进程\n" +
			"2. 检查该进程内存配置（JVM/容器 limit）\n" +
			"3. 评估调整进程内存参数或节点扩容",
	},
	{
		ID: "R-OSS-001-SOP", AlertTypeID: "OWL-OSS-001",
		Name: "节点失联排查指引", Kind: "sop", Risk: "low", Source: "builtin", Reviewed: true,
		Content: "1. ping 与 ssh 手动连通性验证\n" +
			"2. 检查网络/防火墙/SSH 服务（systemctl status sshd）\n" +
			"3. 确认节点凭据未变更；必要时现场/带外登录",
	},
}

// SeedBuiltinRemediesIfEmpty 首次打开时写入内置对策（幂等）。
func (s *Store) SeedBuiltinRemediesIfEmpty() error {
	if err := s.EnsureRemedyTables(); err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM remedies`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, r := range builtinRemedies {
		if err := s.UpsertRemedy(r); err != nil {
			return err
		}
	}
	return nil
}

// BuiltinRemedies 返回内置对策副本（测试用）。
func BuiltinRemedies() []Remedy {
	out := make([]Remedy, len(builtinRemedies))
	copy(out, builtinRemedies)
	return out
}
