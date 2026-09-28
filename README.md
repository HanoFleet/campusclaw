CampusClaw 把教研材料按班级放进可登录的知识库。教师上传 txt 或 md，本班学生只读；上传后正文切成切片，本班可以用关键字、向量或混合检索，命中能回到原文。没有依据时不生成回答。

## 范围

这一版做登录、教师/学生两种角色、按班级隔离、材料入库，以及本班可追溯检索。界面是否好看不作为安全验收；用 curl 仍能跨班读到材料，或检索到其他班的切片，就不算完成。

## 检索

- 三种模式：`keyword` 只查 MySQL 全文；`vector` 问句嵌入后查 Qdrant；`hybrid` 两路先过滤再用 RRF（k=60）。默认混合。
- 班级只来自登录会话。请求里的 `class_id` 丢掉。A 班搜 B 班独有词得到 200 和空 `hits`，不返回 403。
- 切片正文在 `knowledge_chunks`，向量在 Qdrant，主键相同。摘录取自 MySQL。
- 没有候选切片时返回「资料中未找到相关内容」，不凑数，也不调用对话模型。
- Qdrant 和模型网关不映射到宿主，浏览器只打本站 `/api`。
- `EMBEDDING_URL=local`、`CHAT_URL=local` 时在进程内完成嵌入和摘录式回答，不把密钥写进源码。换成课程网关时只改 `.env`。

## 单实例

登录失败次数记在 api 进程内存里。只启动一个 api。多副本时锁定期会对不齐。会话本身在 MySQL，登出会删掉那一行。

## 跨班返回码

跨班访问材料详情或原文件返回 **404**，响应体和不存在的 id 相同，不写 403。403 会告诉对方这个 id 上确有材料。

## 启动

标准启动方式只有 Compose。本机直接跑 Go 二进制不算交付。

```bash
cp .env.example .env
# 填写 SESSION_SECRET、MYSQL_ROOT_PASSWORD、DB_PASSWORD 和三个 SEED_*_PASSWORD
# 检索相关项可先保持 .env.example 里的 local / Qdrant 地址
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

`docker compose down` 之后再 `up`（不要加 `-v`）会保留数据库、已上传文件和 Qdrant 向量。`-v` 会清空卷，只在想重置时使用。Qdrant 没有宿主端口。

## 预置账号

| 用户名 | 角色 | 班级 | 口令 |
| --- | --- | --- | --- |
| `teacher_a` | 教师 | A | `.env` 里的 `SEED_TEACHER_PASSWORD` |
| `student_a1` | 学生 | A | `SEED_STUDENT_A_PASSWORD` |
| `student_b1` | 学生 | B | `SEED_STUDENT_B_PASSWORD` |

预置材料标题是「A班-函数单调性讲义」和「B班-牛顿定律笔记」。A 班检索「牛顿定律」应当 `hits` 为空。接口是 `POST /api/search` 和 `POST /api/ask`。

## 设计决策

完整记录在 `openspec/changes/archive/2026-09-23-add-auth-rbac-class-knowledge/design.md`。当前生效的行为在 `openspec/specs/auth-upload/spec.md` 与 `openspec/specs/knowledge-retrieval/spec.md`。四项发布时要对得上实现：

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

变更 `add-traceable-vector-retrieval` 已实现并归档到 `openspec/changes/archive/2026-09-28-add-traceable-vector-retrieval/`。长期规约在 `openspec/specs/knowledge-retrieval/spec.md`。

## 提交

公开仓库：https://github.com/HanoFleet/campusclaw

