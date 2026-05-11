# GPTProxy

GPTProxy 是一个基于 Wails 构建的桌面代理编排工具，用于管理本地代理、全局二次代理、OpenAI/ChatGPT 账号授权，以及面向 Codex/ChatGPT 流量的账号切换转发。

项目由 Go 后端和 Vue 前端组成，配置与账号数据使用 SQLite 持久化到用户本地目录。软件启动后支持驻留系统托盘，关闭窗口时默认隐藏到托盘，避免代理服务被误关。

## 功能特性

- 本地代理管理：创建、启用、停用、删除本地监听代理。
- 二次代理链路：所有本地代理流量统一走一个全局出口代理。
- 账号管理：通过 OAuth + PKCE 添加 OpenAI/ChatGPT 账号，并保存账号关键 token。
- 激活账号切换：在前端选择账号后激活，代理流量会使用该账号的 token。
- Codex 流量转发：支持 `/v1` 前缀请求转发，用于 VS Code Codex 插件或 Codex CLI。
- 额度刷新：后台定时刷新账号额度信息，并通过 Wails 事件实时更新前端。
- SQLite 本地存储：代理配置、二次代理配置、账号信息均保存在本地数据库。
- 系统托盘：窗口关闭时隐藏到托盘，托盘菜单支持显示窗口和真正退出。
- 日志输出：关键代理、账号、额度刷新流程会写入日志，便于排查问题。

## 实现原理

### 代理链路

GPTProxy 的请求链路如下：

```plain text
客户端 / Codex / 其他软件
        |
        v
GPTProxy 本地代理
        |
        v
全局二次代理
        |
        v
目标服务
```

本地代理由 Go 的 `net/http` 实现，普通 HTTP 请求会通过 `http.Transport` 转发。全局二次代理支持 HTTP 和 SOCKS5 两种出口协议，配置会在程序启动时从 SQLite 加载到内存，避免每次代理请求重复读取数据库。

### 账号切换

账号通过 OpenAI OAuth 2.0 + PKCE 流程授权。授权成功后，后端会解析并保存账号的 `access_token`、`refresh_token`、`id_token`、`account_id`、`user_id`、邮箱和订阅信息。

前端账号列表可以选择一个账号并点击“激活账号”。后端会将该账号 token 缓存在代理管理器中，之后命中 `/v1` 前缀的代理请求会自动替换：

```http
Authorization: Bearer <active_account_access_token>
ChatGPT-Account-Id: <active_account_id>
```

这样可以在不修改客户端配置的情况下切换实际使用的账号。

### Codex 转发

推荐在 Codex 配置中将 OpenAI base URL 指向 GPTProxy 本地代理：

```toml
openai_base_url = "http://127.0.0.1:18080/v1"
```

其中端口需要和你在 GPTProxy 中创建并启用的本地代理端口一致。

当前 `/v1` 前缀流量会由 GPTProxy 处理并通过全局二次代理转发到真实上游服务。`/v1/responses` 会映射到 Codex 使用的 ChatGPT 后端接口，其他 `/v1/...` 请求会转发到 ChatGPT API 域名。

### 额度刷新

软件启动后会异步刷新所有账号额度，并定时刷新。额度信息来自 ChatGPT usage 接口，刷新结果会写入数据库并通过 Wails 事件推送给前端。

为降低过快刷新导致 403 的风险，额度刷新使用独立 HTTP Client、禁用 KeepAlive、补充浏览器请求头，并在多账号之间加入短暂错峰延迟。

### 数据存储

数据保存在用户 Local AppData 目录下：

```plain text
%LOCALAPPDATA%\GPTProxy
```

主要文件包括：

```plain text
gptproxy.db        SQLite 数据库
logs/app.log       应用日志
```

## 使用方法

### 1. 启动软件

开发模式：

```powershell
wails dev
```

生产构建：

```powershell
wails build
```

### 2. 配置二次代理

底部状态栏会显示当前二次代理地址和连接状态。点击齿轮按钮可以修改二次代理配置。

默认配置：

```plain text
http://127.0.0.1:1080
```

所有本地代理都会统一通过这个二次代理作为出口。

### 3. 创建本地代理

在“代理配置”中点击“新增代理”，填写监听 IP 和端口，例如：

```plain text
127.0.0.1:18080
```

启用后，其他软件就可以把请求指向这个本地代理地址。

### 4. 添加并激活账号

在“账号管理”中点击“添加账号”，软件会打开浏览器进行 OpenAI OAuth 授权。授权完成后账号会出现在列表中。

选择账号左侧的单选按钮，然后点击“激活账号”。之后命中 `/v1` 前缀的代理请求会使用该账号 token。

### 5. 配置 Codex

编辑 Codex 配置文件：

```plain text
C:\Users\<你的用户名>\.codex\config.toml
```

加入或修改：

```toml
openai_base_url = "http://127.0.0.1:18080/v1"
```

如果你的本地代理端口不是 `18080`，需要替换成实际启用的端口。

配置完成后，重启 VS Code 或执行 `Developer: Reload Window`。

## About

This is the official Wails Vue template.

You can configure the project by editing `wails.json`. More information about the project settings can be found
here: https://wails.io/docs/reference/project-config

## Live Development

To run in live development mode, run `wails dev` in the project directory. This will run a Vite development
server that will provide very fast hot reload of your frontend changes. If you want to develop in a browser
and have access to your Go methods, there is also a dev server that runs on http://localhost:34115. Connect
to this in your browser, and you can call your Go code from devtools.

## Building

To build a redistributable, production mode package, use `wails build`.
