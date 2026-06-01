# Playbook 模板交互式创建命令 Spec

## Why
用户需要一个会话式的剧本模板创建工具，通过交互式问答逐步引导用户创建 Playbook 模板，降低学习成本和编写门槛。

## What Changes
- 新增 `owl playbook template` 子命令，用于交互式创建剧本模板
- 支持会话式依次设置：任务名、描述、版本等元数据
- 支持按序号选择 action 类型添加任务项
- 每添加一个任务后询问是否继续或结束

## Impact
- Affected code: `cmd/cli/cmd/playbook/` 目录
- 新增文件: `template.go`

## ADDED Requirements

### Requirement: 交互式模板创建命令
系统 SHALL 提供 `owl playbook template` 命令，通过会话式交互创建剧本模板。

#### Scenario: 启动交互式创建
- **WHEN** 用户执行 `owl playbook template`
- **THEN** 系统启动交互式问答流程，依次询问元数据

#### Scenario: 元数据收集
- **WHEN** 交互流程启动
- **THEN** 系统依次询问：
  1. 任务名（name）- 必填
  2. 描述（description）- 可选
  3. 版本（version）- 默认 "1.0"
  4. 变量（vars）- 可选，询问是否添加变量

#### Scenario: 任务项添加
- **WHEN** 元数据收集完成
- **THEN** 系统显示支持的 action 类型列表，按序号排列：
  ```
  请选择任务类型:
  1. command  - 执行 Shell 命令
  2. script   - 执行脚本文件
  3. upload   - 上传文件到节点
  4. download - 从节点下载文件
  5. include  - 包含其他剧本
  ```

#### Scenario: 选择 action 类型
- **WHEN** 用户输入序号选择 action
- **THEN** 系统生成对应 action 的任务模板，包含占位符参数

#### Scenario: 任务模板生成
- **WHEN** 用户选择某个 action 类型
- **THEN** 系统生成包含占位符的任务模板：
  - command: `cmd: "<命令内容>"`
  - script: `script: "<脚本路径>", dest: "/tmp/", args: ""`
  - upload: `src: "<本地路径>", dest: "<远程路径>", overwrite: true`
  - download: `src: "<远程路径>", dest: "<本地路径>", subdir: true`
  - include: `playbook: "<剧本路径>"`

#### Scenario: 继续添加询问
- **WHEN** 一个任务项添加完成
- **THEN** 系统询问 "是否继续添加任务？(y/n)"
- **IF** 用户选择 y
- **THEN** 继续显示 action 选择列表
- **IF** 用户选择 n
- **THEN** 结束任务添加，设置 post_tasks 为空列表

#### Scenario: 模板生成完成
- **WHEN** 所有问答完成
- **THEN** 系统生成完整的 YAML 模板文件，保存到指定路径或默认路径

### Requirement: Action 类型模板
系统 SHALL 为每种 action 类型提供预定义的任务模板结构。

#### Scenario: command 任务模板
```yaml
- name: "任务名称"
  action: command
  args:
    cmd: "<命令内容>"
```

#### Scenario: script 任务模板
```yaml
- name: "任务名称"
  action: script
  args:
    script: "<脚本路径>"
    dest: "/tmp/"
    args: ""
```

#### Scenario: upload 任务模板
```yaml
- name: "任务名称"
  action: upload
  args:
    src: "<本地路径>"
    dest: "<远程路径>"
    overwrite: true
```

#### Scenario: download 任务模板
```yaml
- name: "任务名称"
  action: download
  args:
    src: "<远程路径>"
    dest: "<本地路径>"
    subdir: true
```

#### Scenario: include 任务模板
```yaml
- name: "任务名称"
  action: include
  args:
    playbook: "<剧本路径>"
```

### Requirement: 输出路径参数
系统 SHALL 支持指定模板输出路径。

#### Scenario: 默认输出路径
- **WHEN** 用户未指定输出路径
- **THEN** 模板保存到 `./playbooks/<name>.yaml`

#### Scenario: 自定义输出路径
- **WHEN** 用户使用 `--output` 参数
- **THEN** 模板保存到指定路径