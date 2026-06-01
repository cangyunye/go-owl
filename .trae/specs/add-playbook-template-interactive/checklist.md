# Checklist

- [x] `owl playbook template` 命令可正常执行
- [x] 交互式问答依次收集 name、description、version
- [x] version 默认值为 "1.0"
- [x] action 类型按序号显示：1.command, 2.script, 3.upload, 4.download, 5.include
- [x] 选择 action 后生成对应的任务模板（包含占位符）
- [x] 每添加任务后询问是否继续
- [x] 选择 n 后结束任务添加
- [x] post_tasks 默认为空列表 []
- [x] 生成的 YAML 结构正确
- [x] 模板文件保存到指定路径或默认路径
- [x] 单元测试通过