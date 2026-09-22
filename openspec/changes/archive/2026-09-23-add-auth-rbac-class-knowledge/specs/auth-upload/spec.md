## Purpose

规定 CampusClaw 迭代 1 的登录、角色、班级隔离、材料上传入库和 Compose 发布行为。每条场景都要能单独判定通过或不通过。

## ADDED Requirements

### Requirement: 用户登录

系统 MUST 允许教师与学生使用账号和密码登录，并签发服务端会话。未登录访问受保护接口时 MUST 返回 401，响应体 MUST NOT 含有材料标题、正文或磁盘路径。账号不存在、口令错误和登录限流 MUST 返回相同的状态码与响应体。登录成功 MUST 作废该用户此前的会话并换发新的会话标识。登出 MUST 删除服务端会话行，使旧 Cookie 失效。

#### Scenario: 教师与学生登录成功

- **WHEN** 使用 `teacher_a` 或 `student_a1` 的正确口令调用 `POST /api/login`
- **THEN** 响应状态为 200，并设置 `HttpOnly`、`SameSite=Lax` 的会话 Cookie
- **AND** 随后 `GET /api/me` 返回的 `role` 与 `class_name` 与该账号的预置身份一致

#### Scenario: 错误密码与不存在账号同形

- **WHEN** 分别使用不存在的用户名、以及存在用户的错误口令调用 `POST /api/login`
- **THEN** 两次响应的状态码都是 401，响应体相同，且都不包含「用户不存在」或「密码错误」这类可区分文案

#### Scenario: 未登录访问受保护接口

- **WHEN** 不带会话 Cookie 调用 `GET /api/materials`
- **THEN** 响应状态为 401
- **AND** 响应体不含任一预置材料的标题或正文

#### Scenario: 登出后旧 Cookie 失效

- **WHEN** 登录成功后调用 `POST /api/logout`，再用登出前的 Cookie 调用 `GET /api/me`
- **THEN** 第二次响应状态为 401，且服务端不再保留该会话行

#### Scenario: 限流响应与口令错误同形

- **WHEN** 同一用户名和来源 IP 的失败次数达到配置的阈值后，再使用正确口令调用 `POST /api/login`
- **THEN** 响应仍为 401，且响应体与口令错误时相同

### Requirement: 角色权限

系统 MUST 只承认会话中的 `teacher` 与 `student` 两种角色。教师 MUST 可以上传本班材料。学生调用上传接口 MUST 返回 403，且材料表、知识库表和上传目录都 MUST NOT 发生变化。客户端自行声明的角色 MUST NOT 被采信。

#### Scenario: 学生上传被拒绝

- **WHEN** 以 `student_a1` 的有效会话调用 `POST /api/materials` 提交一份合法 `.md` 文件
- **THEN** 响应状态为 403
- **AND** 材料表行数、知识库表行数和上传目录中的文件数与调用前相同

#### Scenario: 教师上传被允许

- **WHEN** 以 `teacher_a` 的有效会话上传一份非空 UTF-8 的 `.md` 或 `.txt`
- **THEN** 响应状态为 201，响应体含新材料的 id 与标题
- **AND** 本班列表随后能查到该标题

#### Scenario: 学生可以读取本班材料

- **WHEN** `student_a1` 请求本班材料列表、详情和原文件
- **THEN** 三个请求都成功
- **AND** 学生界面不提供上传入口；该限制不代替上面的 403

### Requirement: 班级隔离

系统 MUST 把班级当作数据边界。列表查询 MUST 只使用会话中的 `class_id`。按 id 读取详情或文件时 MUST 先取出整行再核对班级。跨班访问与 id 不存在 MUST 都返回 404，且响应体相同，MUST NOT 含有对方标题、正文或存储路径。请求里的 `class_id` MUST 被忽略。

#### Scenario: 跨班按 id 访问材料被拒绝

- **WHEN** A 班用户使用 B 班材料的 id 请求 `GET /api/materials/{id}` 和 `GET /api/materials/{id}/file`
- **THEN** 两个响应都是 404
- **AND** 响应体与请求一个不存在的 id 时相同，且不含 B 班标题、正文和存储路径

#### Scenario: 列表不泄露其他班级

- **WHEN** A 班用户调用 `GET /api/materials`，包括附带 `class_id` 指向 B 班的查询参数
- **THEN** 结果只含 A 班材料
- **AND** 不出现 B 班预置标题「B班-牛顿定律笔记」

#### Scenario: 上传时的班级字段不改变归属

- **WHEN** A 班教师上传文件时在表单中另外提交 `class_id` 为 B 班
- **THEN** 新材料和对应知识库行的 `class_id` 仍是该教师会话中的班级
- **AND** B 班列表不出现这条标题

### Requirement: 材料上传与知识库入库

教师上传成功时，系统 MUST 在校验通过后把原文件写入上传目录，并在同一数据库事务中插入 `materials` 与 `knowledge_entries`。扩展名 MUST 使用白名单 `.txt` 与 `.md`。超过配置上限 MUST 返回 413。空文件或非 UTF-8 MUST 返回 400。任一失败路径 MUST NOT 留下材料行、知识库行或未关联文件。存储名 MUST 由服务端生成。

#### Scenario: 上传后知识库与列表可查

- **WHEN** `teacher_a` 上传一份含正文的合法 `.md`
- **THEN** 本班 `GET /api/materials` 出现新标题
- **AND** `GET /api/materials/{id}` 返回的正文与文件内容一致，且归属该教师的班级
- **AND** 响应不含磁盘绝对路径

#### Scenario: 非法扩展名不留残留

- **WHEN** 教师上传扩展名为 `.exe` 的文件，或文件名为 `notes.md.exe`
- **THEN** 响应状态为 400
- **AND** 材料表、知识库表和上传目录都没有新增记录或文件

#### Scenario: 超限与非法内容不留残留

- **WHEN** 教师上传超过 `MAX_UPLOAD_BYTES` 的文件，或上传非 UTF-8 内容
- **THEN** 超限响应为 413，非法内容响应为 400
- **AND** 两张表与上传目录都没有新增残留

#### Scenario: 本班学生可见且其他班不可见

- **WHEN** 教师上传成功后，同班学生刷新列表，其他班用户按 id 访问该材料
- **THEN** 同班学生能在列表中看到该标题并下载原文件
- **AND** 其他班用户得到 404

### Requirement: 材料读取与下载

已登录用户 MUST 只能读取本班材料。详情 MUST 返回标题、班级、时间和知识库正文。下载 MUST 走 `GET /api/materials/{id}/file`，并先校验会话与班级。上传目录 MUST NOT 作为静态资源暴露。

#### Scenario: 直接猜测上传路径失败

- **WHEN** 客户端请求 `/uploads/` 下的存储文件名
- **THEN** 响应不是文件内容
- **AND** 状态码不是 200

#### Scenario: 详情不返回存储路径

- **WHEN** 本班用户请求材料详情
- **THEN** 响应含标题与正文
- **AND** 响应不含 `storage_name`、绝对路径或 `/uploads/` 路径

### Requirement: 预置核心数据

系统 MUST 建立班级、用户、讲义、作业、助手、技能六类表，以及材料、知识库和会话表。初始化 MUST 写入班级 A 与 B、用户 `teacher_a`、`student_a1`、`student_b1`，以及标题可区分的两班材料。种子 MUST 幂等：重复启动不得复制用户或材料，也不得覆盖启动之后新上传的内容。种子口令 MUST 来自环境变量并以 bcrypt 存储。

#### Scenario: 种子数据满足双班与用户

- **WHEN** 按 README 完成首次启动并查询数据库
- **THEN** 存在班级 A 与 B
- **AND** `teacher_a` 是 A 班教师，`student_a1` 是 A 班学生，`student_b1` 是 B 班学生
- **AND** 用户口令字段是 bcrypt 哈希，不是配置中的明文

#### Scenario: 两班材料标题可区分

- **WHEN** 分别以 A 班和 B 班用户搜索预置标题
- **THEN** A 班能查到「A班-函数单调性讲义」，查不到「B班-牛顿定律笔记」
- **AND** B 班能查到「B班-牛顿定律笔记」，查不到「A班-函数单调性讲义」

#### Scenario: 六类核心表已创建

- **WHEN** 初始化结束后查看表清单
- **THEN** `classes`、`users`、`handouts`、`assignments`、`assistants`、`skills` 都存在
- **AND** 讲义、作业、助手、技能表各有不少于 0 条、至多用于占位的记录，本迭代不提供这些表的业务接口

#### Scenario: 重复启动不复制种子

- **WHEN** api 进程再次启动
- **THEN** `users` 仍只有上述三个预置账号各一行
- **AND** 两个预置材料的标题和正文不被改写，教师后来上传的材料仍然还在

### Requirement: 密码哈希与会话密钥

系统 MUST 使用 bcrypt 存储口令，禁止明文。会话 Cookie MUST 带有用 `SESSION_SECRET` 计算的 HMAC。`SESSION_SECRET` 或数据库口令缺失时，进程 MUST 启动失败，MUST NOT 使用源码里的默认密钥。这些值 MUST NOT 被提交到 Git。

#### Scenario: 库中无明文密码

- **WHEN** 查询 `users.password_hash`
- **THEN** 每一行都是 bcrypt 字符串
- **AND** 没有任何一行等于 `.env` 中对应的明文口令

#### Scenario: 缺少会话密钥时拒绝启动

- **WHEN** 未设置 `SESSION_SECRET` 就启动 api
- **THEN** 进程以非零状态退出
- **AND** 不监听端口

#### Scenario: 真实密钥不在仓库中

- **WHEN** 检查被 Git 跟踪的文件
- **THEN** 不存在可用的 `SESSION_SECRET`、数据库口令或种子明文口令
- **AND** `.env.example` 只含空值或占位说明

### Requirement: Docker Compose 部署与健康检查

标准启动方式 MUST 是 Docker Compose 的 web、api、db 三个服务。只有 web MUST 映射宿主端口。数据库端口 MUST NOT 映射到宿主。`GET /health` MUST 不要求登录，且 MUST NOT 查询数据库。数据库不可用时，业务接口 MUST 返回 503，而不是 401。`docker compose down` 之后再 `up`（不带 `-v`）MUST 保留已上传的数据。

#### Scenario: Compose 启动后可访问

- **WHEN** 按 README 从 `.env.example` 复制并填好变量后执行 `docker compose up --build -d`
- **THEN** 浏览器可以打开 web 端口上的登录页
- **AND** 未登录 `GET /health` 返回 200 且响应体表示进程存活

#### Scenario: 数据库端口不对外

- **WHEN** 查看 Compose 配置并在宿主上连接 MySQL 默认端口
- **THEN** db 服务没有 `ports` 映射
- **AND** api 容器环境中没有 MySQL 管理员口令

#### Scenario: 数据库停止时健康检查与业务接口分离

- **WHEN** 停止 db 容器后访问 `/health` 和 `GET /api/materials`
- **THEN** `/health` 仍返回 200
- **AND** 材料接口返回 503，而不是 401

#### Scenario: 重建容器后数据仍在

- **WHEN** 教师上传一份材料后执行 `docker compose down` 再 `docker compose up -d`，且不使用 `-v`
- **THEN** 同一账号再次登录后仍能在列表中看到该材料

### Requirement: Web 行为与安全

前端 MUST 提供登录页和材料页。刷新后身份 MUST 以 `GET /api/me` 为准。收到 401 时 MUST 回到登录页。上传入口是否显示 MUST 只取决于 `/api/me` 的角色。材料正文 MUST 以 Markdown 渲染，且材料中的脚本 MUST NOT 被执行。

#### Scenario: 刷新后以服务端会话恢复身份

- **WHEN** 已登录用户刷新材料页
- **THEN** 页面调用 `GET /api/me`
- **AND** 不读取 localStorage 中的角色来决定上传入口

#### Scenario: 401 回到登录页

- **WHEN** 材料页收到 401
- **THEN** 页面进入登录页
- **AND** 不继续展示材料标题

#### Scenario: Markdown 不执行脚本

- **WHEN** 材料正文包含 `<script>` 标记
- **THEN** 详情区把它显示为文本
- **AND** 浏览器不执行该脚本

### Requirement: Web 视觉与交互

材料页 MUST 使用北大红色作为主色，并提供浅色与深色主题、列表与网格两种视图、仅限本班的搜索、Command 或 Ctrl 加 K 的命令面板、教师上传进度，以及操作结果提示。布局在窄屏上 MUST 仍可完成登录、查看和上传。

#### Scenario: 本班搜索不返回其他班

- **WHEN** A 班用户在搜索框输入 B 班预置标题中的「牛顿定律」
- **THEN** 列表为空或不含该标题
- **AND** 请求打到带会话班级过滤的列表接口

#### Scenario: 主题、视图和命令面板可用

- **WHEN** 用户切换深色主题、切换网格视图，并按下 Command+K 或 Ctrl+K
- **THEN** 主题和视图发生变化
- **AND** 命令面板打开，且其中不包含切换到其他班级的命令

#### Scenario: 教师看到上传进度

- **WHEN** 教师选择一份合法文件上传
- **THEN** 页面显示上传进度
- **AND** 成功或失败都有可见提示
