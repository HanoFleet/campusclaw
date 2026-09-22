## Why

教研材料散落在群文件和网盘里，班级之间没有可判定的数据边界。CampusClaw 后面的检索和问答都要站在「谁登录、谁能上传、能看见哪一班」之上，所以这一版先把身份、角色、班级隔离和材料入库做成可以验收的底座。

## What Changes

- 教师和学生使用账号密码登录；未登录访问受保护页面时进入登录页，受保护接口返回 401，响应体不含材料标题或正文。
- 教师可以上传并查看、下载本班材料；学生对本班材料只读，调用上传接口返回 403，且数据库与上传目录都不变。
- 班级是数据边界。列表、详情、下载和入库使用的班级只来自服务端会话；A 班用户访问 B 班材料得到与「记录不存在」同形的 404。
- 教师上传 `.txt` / `.md` 后，正文写入知识库；本班列表来自数据库查询，上传成功后能看到新记录。
- 预置班级 A/B、教师 A、学生 A1/B1，以及两班标题可区分的材料。同时建立班级、用户、讲义、作业、助手、技能六类表。
- 密码只存 bcrypt；会话密钥只来自环境变量，缺失则进程启动失败。标准启动方式是单实例 Docker Compose，`GET /health` 不查库、不要求登录。

## Capabilities

### New Capabilities

- `auth-upload`: 登录与会话、角色授权、班级隔离、教师上传入库、预置数据、密码哈希与会话密钥、Compose 与 `/health`，以及材料页的行为与安全。

### Modified Capabilities

- 无。`openspec/specs/` 里还没有主力规约，本能力等归档后才写入。

## Impact

- 新增 Go API、React 前端、Nginx 与 MySQL 8，由 Docker Compose 启动 web、api、db。对外只映射 web 端口。
- 新增表：`classes`、`users`、`sessions`、`materials`、`knowledge_entries`，以及占位用的 `handouts`、`assignments`、`assistants`、`skills`。
- 仓库根目录增加 `Dockerfile`（位于前后端目录）、`docker-compose.yml`、`.env.example` 与 README。
- 可运行实现在 Apply 阶段完成。归档前不把本能力写进 `openspec/specs/`。

## Non-goals

本迭代明确不做下列事项。表里可以留空结构，但不提供入口，也不把它们当作验收项：

- 知识库问答、向量检索、RAG、跨班全文检索。列表上的本班关键字筛选不是检索。
- 对话助手、作业布置与批改、成绩与错题本。
- JWT、OAuth、SSO、校园统一身份。登录态是服务端会话 Cookie，不使用 localStorage 里的 Bearer token。
- 注册、改密、多校多租户、验证码或邮箱短信登录。
- PDF、Word、图片解析，以及在线预览编辑。
- Kubernetes、CI、公网域名、HTTPS 证书、多副本高可用。限流计数在进程内存里，因此只承诺单实例。
- 平台超级管理员。这个角色跨班可见，会让「跨班与记录不存在都返回 404」失去判定力。等隔离验收稳定后单独开一次变更，再设计它的越权路径和操作审计。
