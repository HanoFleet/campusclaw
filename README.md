CampusClaw 迭代 1 把教研材料按班级放进可登录的知识库底座，教师上传，学生只读本班内容。
场景：教师登录后上传 txt 或 md，本班列表和知识库立刻能查到；学生只能查看和下载本班材料。
迭代 2 的需求已经写进 OpenSpec 变更 `add-traceable-vector-retrieval`：本班知识库检索、三种模式、出处可回溯、没有依据不生成。实现按该变更的 `tasks.md` 进行。
不做：作业批改、注册改密、JWT/SSO、平台超级管理员、多副本和公网 HTTPS。检索的边界以该变更的 Non-goals 为准。

## 范围

这一版只做登录、教师/学生两种角色、按班级隔离、把 `.txt` / `.md` 上传进知识库，并用 Docker Compose 在本机跑起来。界面是否好看不作为安全验收；用 curl 仍能跨班读取或让学生上传成功，就不算完成。

## 不做

- 跨班全文检索。检索只在会话所属班级内进行。
- 流式长对话、作业布置与批改、成绩和错题本。
- 把 Qdrant 或模型网关暴露给浏览器。
- JWT、OAuth、校园 SSO、注册和改密。
- PDF / Word、Kubernetes、公网域名和 HTTPS。
- 平台超级管理员。这个角色会跨班看数据，和「跨班返回 404」冲突，留到以后单独做。

## 单实例

登录失败次数记在 api 进程内存里。只启动一个 api。多副本时锁定期会对不齐。会话本身在 MySQL，登出会删掉那一行。

## 跨班返回码

跨班访问材料详情或原文件返回 **404**，响应体和不存在的 id 相同，不写 403。403 会告诉对方这个 id 上确有材料。

## 启动

标准启动方式只有 Compose。本机直接跑 Go 二进制不算交付。

```bash
cp .env.example .env
# 填写 SESSION_SECRET、MYSQL_ROOT_PASSWORD、DB_PASSWORD 和三个 SEED_*_PASSWORD
# SESSION_SECRET 至少 16 个字符，每个口令至少 8 个字符
docker compose up --build -d
docker compose ps
```

首次启动 MySQL 可能要几十秒。在这之前打开页面可能短暂看到 502，等 api 连上数据库后即可登录。

- 登录页：`http://localhost:8080/`（端口由 `.env` 的 `WEB_PORT` 决定）
- 存活检查：`http://localhost:8080/health`，不需要登录，不查询数据库

数据库端口不映射到宿主机。要查表时进入容器：

```bash
docker compose exec db mysql -u"$DB_USER" -p"$DB_PASSWORD" campusclaw
```

`docker compose down` 之后再 `up`（不要加 `-v`）会保留数据库和已上传文件。`-v` 会清空卷，只在想重置时使用。

## 预置账号

| 用户名 | 角色 | 班级 | 口令 |
| --- | --- | --- | --- |
| `teacher_a` | 教师 | A | `.env` 里的 `SEED_TEACHER_PASSWORD` |
| `student_a1` | 学生 | A | `SEED_STUDENT_A_PASSWORD` |
| `student_b1` | 学生 | B | `SEED_STUDENT_B_PASSWORD` |

预置材料标题是「A班-函数单调性讲义」和「B班-牛顿定律笔记」。A 班搜索「牛顿定律」应当没有结果。

## 设计决策

完整记录在 `openspec/changes/archive/2026-09-23-add-auth-rbac-class-knowledge/design.md`。当前生效的行为在 `openspec/specs/auth-upload/spec.md`。四项发布时要对得上实现：

1. 班级只来自服务端会话。请求里自带的 `class_id` 不影响列表和入库。
2. 跨班一律 404，和记录不存在同形。
3. 材料行和知识库行在同一个事务里写入；失败时回滚并删除已经写出的文件。
4. `/health` 只表示进程还在，不检查数据库。数据库不可用时，业务接口返回 503，而不是 401。

## 验收

```bash
python3 scripts/verify.py
```

脚本会按上面的场景请求本机 Compose，并把不含口令的结果写到 `docs/verification.md`。

## 第 4 课变更

活动变更在 `openspec/changes/add-traceable-vector-retrieval/`：

- `proposal.md`：为什么做、做什么、不做什么
- `design.md`：MySQL 存切片正文，Qdrant 存向量，混合检索用 RRF
- `specs/knowledge-retrieval/spec.md`：可判定的 Requirement 与 Scenario
- `specs/auth-upload/spec.md`：上传成功后必须建索引
- `tasks.md`：实现顺序，目前全部未勾选

校验：`openspec validate add-traceable-vector-retrieval --strict`

## 提交

公开仓库：https://github.com/HanoFleet/campusclaw

