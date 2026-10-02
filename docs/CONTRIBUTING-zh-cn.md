<p align="center">
  <a href="./CONTRIBUTING.md">English</a> /
  <a href="./CONTRIBUTING-ru.md">Русский</a> /
  <a href="./CONTRIBUTING-zh-cn.md">简体中文</a>
</p>

# 参与开发 Gamaj Bot

感谢你的帮助。本项目有一小套不可妥协的规则；写代码之前请先阅读——违反这些规则的拉取请求不会被合并。

## 红线

1. **只使用 Gamaj 这个名字。** 代码、注释、界面文本、文档或资源中不得出现任何其他机器人或面板项目的名称。自动化的环境变量命名守卫会在出现此类名称时使构建失败。
2. **不留代理痕迹。** 代码、提交信息或文档中不得提及 AI 助手、代理或自动化工具。提交必须看起来像普通开发者的工作。
3. **唯一的安装路径：原生二进制 + systemd。** 不使用 Docker 和安装模式开关。`scripts/install.sh` 安装器需要一个已构建的 `gamajbot` 二进制，并配置 `gamaj-bot` systemd 服务。
4. **每个环境变量都以 `GAMAJ_` 开头。** 运行时配置存放在 JSON 配置文件中（`panel_url`、`api_key`、`bot_token`、`admin_id`），而不是自由命名的环境变量；从进程环境读取的任何内容都必须通过命名守卫。
5. **机密信息绝不进入仓库。** 安装器创建的配置带有占位符和 `chmod 600`；请保持这一状态。
6. **文档自成体系。** 文档不得让读者“去 GitHub 或其他网站阅读更多”；内容应属于本仓库（以及 `Web/` 文档站）。

## 工作流程

1. 从 `Asli` 分叉或创建分支。
2. 保持改动聚焦；每个拉取请求只做一个主题。
3. 运行与你改动相关的检查：

```bash
cd Bot
gofmt -l .                       # 无输出
go build ./... && go vet ./...
go test ./internal/platform/envguard/

# 修改了安装器时：
bash -n scripts/install.sh scripts/manage.sh
```

## 提交信息

简短、祈使语气、说明改动本身，例如：

```
Reject webhook updates with an unknown secret
Rate-limit admin commands per chat
```

不要添加工具页脚、generated-by 行或 co-author 尾注。提交的作者与提交者必须是您自己的 Git 身份。

## 代码风格

- Go：`gofmt`、`go vet`、用 `%w` 包装错误、表驱动测试放在所覆盖的包旁边。
- Shell：`set -euo pipefail`，除常见 POSIX 工具外不依赖外部程序。
- Telegram UX：面向用户的字符串与面板仪表盘的措辞保持一致；键盘布局必须在窄屏和宽屏上都正常。

## 报告问题

请附上：机器人版本（发布标签或 `gamajbot -version`）、操作系统与架构、日志输出（`journalctl -u gamaj-bot -n 100`）以及最小复现步骤。绝不要在 issue 中粘贴真实的机器人令牌、API 密钥或管理员聊天 ID——请轮换令牌并提供脱敏后的日志。安全问题请私下联系维护者。
