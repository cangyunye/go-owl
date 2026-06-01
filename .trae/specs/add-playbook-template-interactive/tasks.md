# Tasks

- [x] Task 1: 创建 template.go 命令文件
  - [x] SubTask 1.1: 创建 NewPlaybookTemplateCmd 函数，注册到 playbook 命令
  - [x] SubTask 1.2: 定义命令参数（--output 输出路径）

- [x] Task 2: 实现交互式问答流程
  - [x] SubTask 2.1: 实现 promptForMetadata 函数，收集 name、description、version
  - [x] SubTask 2.2: 实现 promptForVars 函数，询问是否添加变量

- [x] Task 3: 实现 action 选择与任务模板生成
  - [x] SubTask 3.1: 定义 actionTemplates 映射，包含 5 种 action 的模板结构
  - [x] SubTask 3.2: 实现 displayActionChoices 函数，显示序号选择列表
  - [x] SubTask 3.3: 实现 generateTaskTemplate 函数，根据选择生成任务模板

- [x] Task 4: 实现任务添加循环
  - [x] SubTask 4.1: 实现 promptForContinue 函数，询问是否继续添加
  - [x] SubTask 4.2: 实现任务收集循环逻辑

- [x] Task 5: 实现 YAML 生成与保存
  - [x] SubTask 5.1: 实现 buildPlaybookYAML 函数，组装完整 YAML 结构
  - [x] SubTask 5.2: 实现 savePlaybookFile 函数，保存到指定路径

- [x] Task 6: 添加单元测试
  - [x] SubTask 6.1: 测试命令注册和参数
  - [x] SubTask 6.2: 测试 action 模板生成

# Task Dependencies
- [Task 3] depends on [Task 2]
- [Task 4] depends on [Task 3]
- [Task 5] depends on [Task 4]
- [Task 6] depends on [Task 5]