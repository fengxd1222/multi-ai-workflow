# 错误处理和故障排除指南

本文档提供多 AI 协作工作流中常见错误的诊断和解决方案。

---

## 目录

1. [Codex 相关错误](#codex-相关错误)
2. [Gemini 相关错误](#gemini-相关错误)
3. [文件系统错误](#文件系统错误)
4. [网络相关错误](#网络相关错误)
5. [权限相关错误](#权限相关错误)
6. [配置相关错误](#配置相关错误)

---

## Codex 相关错误

### 错误 1: API 限流 (Rate Limit Exceeded)

**症状**:
```
Error: Request failed with status code 429
Rate limit exceeded. Please try again in 60 seconds.
```

**错误码**: 通常是 1 或 429

**可能原因**:
1. 短时间内发送了过多请求
2. API 配额已用完
3. 账户处于免费层,有请求限制

**解决方案**:
1. **等待重试**:
   - 等待错误信息中指定的时间 (通常 60 秒)
   - 然后选择 [重试] 选项

2. **检查配额**:
   ```bash
   # 检查 Codex 账户状态和配额
   codex account status
   ```

3. **升级账户**:
   - 如果经常遇到限流,考虑升级到付费计划

4. **减少请求频率**:
   - 避免频繁重新运行工作流
   - 在文档确认阶段仔细检查需求

---

### 错误 2: 认证失败 (Authentication Failed)

**症状**:
```
Error: Authentication failed
Invalid API key or token
```

**错误码**: 通常是 1 或 401

**可能原因**:
1. Codex API key 未配置或已过期
2. API key 配置错误
3. 账户被暂停

**解决方案**:
1. **检查 API key**:
   ```bash
   # 查看 Codex 配置
   codex config list
   ```

2. **重新配置 API key**:
   ```bash
   # 设置新的 API key
   codex config set api_key YOUR_API_KEY
   ```

3. **验证账户状态**:
   - 登录 Codex 网站检查账户状态
   - 确认账户没有被暂停或禁用

4. **生成新 API key**:
   - 在 Codex 控制台生成新的 API key
   - 更新配置

---

### 错误 3: 需求不明确导致失败

**症状**:
```
Error: Unable to generate code
The requirements are not clear enough
```

**错误码**: 通常是 1

**可能原因**:
1. 需求文档过于简略
2. 技术细节不足
3. 缺少关键信息 (如数据结构、接口定义)
4. 需求矛盾或模糊

**解决方案**:
1. **返回 Step 2 修改需求**:
   - 选择 [修改需求后重试]
   - 要求 Claude 生成更详细的文档

2. **增加技术细节**:
   - 在详细设计文档中添加:
     - 具体的接口定义
     - 数据结构示例
     - 伪代码或算法描述
     - 依赖关系说明

3. **提供示例**:
   - 添加输入/输出示例
   - 提供类似项目的参考

4. **简化需求**:
   - 如果项目过于复杂,分阶段实现
   - 先实现核心功能

---

### 错误 4: 网络超时

**症状**:
```
Error: Connection timeout
Unable to reach Codex API server
```

**错误码**: 通常是 1

**可能原因**:
1. 网络连接不稳定
2. Codex 服务器维护或故障
3. 防火墙阻止连接
4. 代理配置问题

**解决方案**:
1. **检查网络连接**:
   ```bash
   # 测试网络连接
   ping 8.8.8.8
   curl -I https://api.codex.example.com
   ```

2. **检查 Codex 服务状态**:
   - 访问 Codex 状态页面
   - 查看是否有维护公告

3. **配置代理** (如需要):
   ```bash
   export HTTP_PROXY=http://proxy.example.com:8080
   export HTTPS_PROXY=http://proxy.example.com:8080
   ```

4. **重试**:
   - 等待几分钟后重试
   - 如果持续失败,联系 Codex 支持

---

### 错误 5: 输出目录写入失败

**症状**:
```
Error: Permission denied
Unable to write to output directory
```

**错误码**: 通常是 1

**可能原因**:
1. 当前目录没有写入权限
2. 磁盘空间不足
3. 目录被其他进程锁定

**解决方案**:
1. **检查目录权限**:
   ```bash
   ls -la
   # 应该显示 drwxr-xr-x (可写)
   ```

2. **修改权限** (如需要):
   ```bash
   chmod u+w .
   ```

3. **检查磁盘空间**:
   ```bash
   df -h .
   # 确保有足够的可用空间
   ```

4. **切换到其他目录**:
   - cd 到有写入权限的目录
   - 重新运行工作流

---

## Gemini 相关错误

### 错误 6: Gemini API 认证失败

**症状**:
```
Error: Gemini authentication failed
Invalid API credentials
```

**错误码**: 通常是 1 或 401

**可能原因**:
1. Gemini API key 未配置
2. API key 已过期
3. 权限不足

**解决方案**:
1. **检查 Gemini 配置**:
   ```bash
   gemini config show
   ```

2. **配置 API key**:
   ```bash
   gemini config set api_key YOUR_API_KEY
   ```

3. **验证配置**:
   ```bash
   gemini test-connection
   ```

---

### 错误 7: 代码目录为空或不存在

**症状**:
```
Error: No code files found
The specified directory is empty or does not exist
```

**错误码**: 通常是 1

**可能原因**:
1. Codex 编码失败,未生成代码
2. 代码被删除或移动
3. 指定了错误的目录

**解决方案**:
1. **确认 Codex 执行成功**:
   - 检查 Step 3 是否成功完成
   - 查看当前目录是否有生成的文件

2. **列出文件**:
   ```bash
   ls -la
   find . -type f -name "*.py" -o -name "*.js"
   ```

3. **如果目录为空**:
   - 返回 Step 3 重新执行 Codex 编码
   - 或选择 [终止流程] 并手动检查问题

---

### 错误 8: 审查报告生成失败

**症状**:
```
Error: Failed to generate review report
Unable to save report to specified location
```

**错误码**: 通常是 1

**可能原因**:
1. 报告输出目录不存在
2. 没有写入权限
3. 磁盘空间不足
4. Gemini 无法分析代码 (格式不支持)

**解决方案**:
1. **检查输出目录**:
   ```bash
   ls -la requirements/项目名/
   ```

2. **手动创建目录** (如需要):
   ```bash
   mkdir -p requirements/项目名
   ```

3. **检查权限和空间**:
   ```bash
   df -h
   ls -la requirements/
   ```

4. **重试**:
   - 选择 [重试] 选项

---

## 文件系统错误

### 错误 9: 项目目录创建失败

**症状**:
```
✗ 目录创建失败: requirements/项目名
```

**错误码**: 1

**可能原因**:
1. 当前目录没有写入权限
2. 磁盘空间不足
3. 父目录不存在
4. 文件系统只读

**解决方案**:
1. **检查当前目录权限**:
   ```bash
   ls -la
   pwd
   ```

2. **确认可以创建目录**:
   ```bash
   mkdir test_dir && rmdir test_dir
   ```

3. **检查磁盘空间**:
   ```bash
   df -h .
   ```

4. **切换到其他目录**:
   ```bash
   cd ~/projects
   # 重新运行工作流
   ```

---

### 错误 10: 文件已存在冲突

**症状**:
```
警告: 目录已存在: requirements/项目名
将使用现有目录,可能会覆盖已有文件
```

**这不是错误**: 这是一个警告信息

**处理方式**:
1. **如果要保留旧文件**:
   - 在继续前备份现有文件
   ```bash
   cp -r requirements/项目名 requirements/项目名.backup
   ```

2. **如果要覆盖**:
   - 直接继续工作流
   - 旧文件会被新文件覆盖

3. **使用不同的项目名称**:
   - 取消当前工作流
   - 重新开始,使用新的项目名称

---

## 网络相关错误

### 错误 11: DNS 解析失败

**症状**:
```
Error: getaddrinfo ENOTFOUND api.codex.example.com
DNS resolution failed
```

**可能原因**:
1. DNS 服务器问题
2. 网络连接断开
3. 域名配置错误

**解决方案**:
1. **检查网络连接**:
   ```bash
   ping 8.8.8.8
   ping google.com
   ```

2. **测试 DNS**:
   ```bash
   nslookup api.codex.example.com
   dig api.codex.example.com
   ```

3. **更换 DNS 服务器**:
   - 临时使用公共 DNS (8.8.8.8, 1.1.1.1)
   - 或联系网络管理员

4. **检查 /etc/hosts**:
   ```bash
   cat /etc/hosts | grep codex
   # 确保没有错误的 hosts 映射
   ```

---

### 错误 12: SSL/TLS 证书错误

**症状**:
```
Error: certificate verification failed
SSL certificate problem
```

**可能原因**:
1. 系统时间不正确
2. CA 证书过期或缺失
3. 中间人攻击 (较少见)

**解决方案**:
1. **检查系统时间**:
   ```bash
   date
   # 确保时间正确
   ```

2. **更新 CA 证书**:
   ```bash
   # macOS
   brew install ca-certificates

   # Linux
   sudo apt-get update && sudo apt-get install ca-certificates
   ```

3. **临时跳过证书验证** (仅用于测试):
   ```bash
   # 不推荐在生产环境使用
   export NODE_TLS_REJECT_UNAUTHORIZED=0
   ```

---

## 权限相关错误

### 错误 13: 脚本执行权限不足

**症状**:
```
bash: ./scripts/call_codex.sh: Permission denied
```

**可能原因**:
1. 脚本没有执行权限
2. 文件系统挂载为 noexec

**解决方案**:
1. **添加执行权限**:
   ```bash
   chmod +x ~/.claude/skills/multi-ai-workflow/scripts/*.sh
   ```

2. **验证权限**:
   ```bash
   ls -la ~/.claude/skills/multi-ai-workflow/scripts/
   # 应该显示 -rwxr-xr-x
   ```

3. **直接用 bash 执行** (临时方案):
   ```bash
   bash ~/.claude/skills/multi-ai-workflow/scripts/call_codex.sh [参数]
   ```

---

### 错误 14: 无法读取配置文件

**症状**:
```
Error: Permission denied
Unable to read configuration file
```

**可能原因**:
1. 配置文件权限设置不正确
2. 配置文件被其他用户创建

**解决方案**:
1. **检查文件权限**:
   ```bash
   ls -la ~/.claude/
   ls -la ~/.codex/
   ```

2. **修改权限**:
   ```bash
   chmod 600 ~/.claude/config.json
   chmod 600 ~/.codex/config.json
   ```

3. **确认文件所有者**:
   ```bash
   ls -la ~/.claude/config.json
   # 应该是你的用户名
   ```

---

## 配置相关错误

### 错误 15: Codex CLI 未安装或未找到

**症状**:
```
bash: codex: command not found
```

**可能原因**:
1. Codex CLI 未安装
2. Codex CLI 不在 PATH 中
3. shell 配置未生效

**解决方案**:
1. **检查是否安装**:
   ```bash
   which codex
   ```

2. **安装 Codex CLI**:
   ```bash
   # 根据 Codex 官方文档安装
   # 示例 (实际命令可能不同):
   npm install -g @codex/cli
   # 或
   pip install codex-cli
   ```

3. **添加到 PATH**:
   ```bash
   # 找到 Codex 安装位置
   find /usr /opt ~ -name codex -type f 2>/dev/null

   # 添加到 PATH (在 ~/.bashrc 或 ~/.zshrc 中)
   export PATH="$PATH:/path/to/codex/bin"

   # 重新加载配置
   source ~/.bashrc  # 或 source ~/.zshrc
   ```

---

### 错误 16: Gemini CLI 未安装或未找到

**症状**:
```
bash: gemini: command not found
```

**解决方案**: 参考错误 15,将 Codex 替换为 Gemini

---

### 错误 17: 版本不兼容

**症状**:
```
Error: Incompatible CLI version
Please upgrade to version X.Y.Z or higher
```

**可能原因**:
1. Codex/Gemini CLI 版本过旧
2. 工作流使用了新功能

**解决方案**:
1. **检查版本**:
   ```bash
   codex --version
   gemini --version
   ```

2. **升级 CLI**:
   ```bash
   # Codex
   npm update -g @codex/cli
   # 或
   pip install --upgrade codex-cli

   # Gemini
   npm update -g @gemini/cli
   # 或
   pip install --upgrade gemini-cli
   ```

3. **验证升级**:
   ```bash
   codex --version
   gemini --version
   ```

---

## 通用故障排除步骤

### 诊断清单

当遇到未知错误时,按以下顺序检查:

1. **检查网络连接**:
   ```bash
   ping 8.8.8.8
   curl -I https://www.google.com
   ```

2. **检查 CLI 工具**:
   ```bash
   which codex
   which gemini
   codex --version
   gemini --version
   ```

3. **检查配置**:
   ```bash
   codex config list
   gemini config show
   ```

4. **检查权限**:
   ```bash
   ls -la ~/.claude/
   ls -la .
   ```

5. **检查磁盘空间**:
   ```bash
   df -h
   ```

6. **查看日志**:
   ```bash
   # 查看 Codex 日志 (如果有)
   cat ~/.codex/logs/latest.log

   # 查看 Gemini 日志 (如果有)
   cat ~/.gemini/logs/latest.log
   ```

---

## 获取帮助

### 如何报告问题

如果以上方法都无法解决问题:

1. **收集信息**:
   - 完整的错误信息
   - 错误发生的步骤
   - 系统信息:
     ```bash
     uname -a
     codex --version
     gemini --version
     ```

2. **联系支持**:
   - Codex 支持: [Codex 官方文档或支持渠道]
   - Gemini 支持: [Gemini 官方文档或支持渠道]
   - Claude Code skill 问题: 在 skill 仓库提 issue

3. **社区论坛**:
   - 搜索是否有人遇到相同问题
   - 发帖询问,提供详细信息

---

## 预防性维护

### 定期检查

建议定期执行以下检查:

1. **更新 CLI 工具**:
   ```bash
   npm update -g @codex/cli
   npm update -g @gemini/cli
   ```

2. **清理临时文件**:
   ```bash
   # 清理旧的工作流文件 (可选)
   find requirements/ -type f -mtime +30
   ```

3. **检查配额**:
   ```bash
   codex account status
   gemini account status
   ```

4. **备份重要文件**:
   - 定期备份 requirements/ 目录
   - 使用版本控制 (Git)

---

**最后更新**: 2025-12-12
**版本**: 1.0.0
