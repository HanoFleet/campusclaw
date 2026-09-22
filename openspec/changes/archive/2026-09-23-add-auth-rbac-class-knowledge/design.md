## Context

仓库在 Apply 之前只有 OpenSpec 和说明文件，没有业务代码。验收标准在 `specs/auth-upload/spec.md`。这一版要让另一个人按 README 用 Compose 启动后，能登录、只能看见本班材料，并能把教师上传的文本写进知识库。

## Goals / Non-Goals

**Goals:**

- 一条请求链路：浏览器 → Nginx → Go → MySQL。认证、授权、隔离和上传校验都在 Go。
- 会话能回答 user id、角色和班级。这三项每次从数据库读取，不放进前端可改的存储。
- 跨班与不存在的 id 对外是同一种 404。
- 上传要么同时留下材料行、知识库行和文件，要么什么都不留。
- 第三方只靠 `.env.example` 和 README 就能启动。

**Non-Goals:**

- 见 `proposal.md` 的 Non-goals。这里不重复展开问答、作业流程、JWT 和超级管理员。

## Decisions

### Decision 1: 浏览器只访问 Nginx，Go 使用 MySQL

做法：Compose 里三个服务。web 用 Nginx 托管前端构建结果，并把 `/api` 和 `/health` 反代到 api。api 用 Go 标准库 `net/http`。数据在 MySQL 8。开发时 Vite 也把 `/api` 和 `/health` 代理到本机 api，路径和生产一致。

备选：第 2 课课表例子用过 Flask 加 SQLite，单进程就能跑。这个仓库不沿用它。SQLite 文件锁和「web / api / db 分开」对不齐，后面做检索时也要换连接方式。单体模板渲染能让 Cookie 更省事，但当前页面是独立的前端，接口和页面分开才好用 curl 验收越权。

### Decision 2: 服务端会话 Cookie，不用 JWT

做法：登录成功后生成新的随机会话 id，丢掉该用户的旧会话行，把 id 的 HMAC 放进 `HttpOnly`、`SameSite=Lax` 的 Cookie。HMAC 密钥是 `SESSION_SECRET`。角色和班级不写进 Cookie，每次用会话 id 查 `users`。本机 HTTP 不加 `Secure`，否则浏览器会丢掉 Cookie。

备选：JWT 放在 localStorage。否决。脚本能读到它，载荷里的角色也容易被前端当成权限；登出后令牌在过期前仍然有效。OAuth 和校园 SSO 把登录交给外部身份源，这一版要先把自己的会话、登出和角色做正确。

### Decision 3: 班级只来自会话，跨班返回 404

做法：列表语句带 `class_id = 会话班级`。按 id 先 `SELECT` 整行，再比较班级。不一致就写服务端日志并返回 404。不存在的 id 走同一个 404 响应体。上传时忽略表单、query 和 Header 里的 `class_id`。

备选：跨班返回 403。否决。403 等于告诉调用方「这个 id 有这条材料」，方便挨个试 id。返回 401 也不对，因为用户已经登录，失败的是对象授权，不是会话。只在前端隐藏其他班的链接同样否决，curl 改 id 仍然必须失败。

自增主键可以被猜到，所以更要用 404，而不是改用 UUID 来代替访问控制。

### Decision 4: 口令用 bcrypt，密钥缺失就退出

做法：种子口令和以后不会有的注册口令都只存 bcrypt。比较失败时，即使用户不存在，也做一次固定哈希的比较，避免响应用时暴露账号。`SESSION_SECRET`、数据库口令、种子口令缺任何一个，api 直接退出，源码里不写默认值。

备选：argon2。同样是加盐慢哈希，可以换，但这一版用 Go 扩展库里的 bcrypt，不存明文，也不做可逆加密。启动时内置一把开发密钥能让克隆后立刻运行，否决，因为任何人都能够造会话。

### Decision 5: 登录失败按用户名和 IP 限流，计数放在进程内存

做法：失败次数达到 `LOGIN_FAIL_THRESHOLD` 后，在 `LOGIN_LOCK_MINUTES` 内即使口令正确也拒绝，响应体仍是 `{"error":"invalid_credentials"}`。不返回 429。

备选：把限流放进 Redis，多副本才能共享。这一版只有一个 api 进程，内存计数够用。也因此 README 写明单实例：再跑一个 api，锁定期会对不齐。会话本身在 MySQL，不在内存里，所以登出删行后立刻失效，api 重启也不会把别人留在登录态之外——Cookie 仍要能对上数据库里的行。

### Decision 6: 上传白名单、服务端文件名、两表同一事务

做法：只接受 `.txt` 和 `.md`。先看扩展名、大小和 UTF-8，通过后才把内容写到上传目录，文件名是服务端随机串。随后在一个事务里插入 `materials` 和 `knowledge_entries`。事务失败或写文件失败就删除已写出的文件并回滚。知识库存原文，页面读取时再渲染 Markdown，不把 HTML 存进库。

备选：先落盘再人工审核，或丢进队列异步入库。失败时很难判定「现在库和磁盘是不是干净的」。对象存储留到多副本。两张表合成一张也能少一次写入，但第 4 课的检索需要单独的正文行，所以拆开，并用事务绑在一起。

下载只走 `GET /api/materials/{id}/file`。Nginx 对 `/uploads/` 直接 404，上传卷只挂在 api 上。

### Decision 7: `/health` 不查数据库

做法：`GET /health` 只要进程还能应答就返回 200，不读会话，也不 ping MySQL。数据库连不上时，登录和材料接口返回 503。

备选：在 `/health` 里顺便探活数据库，编排器一次请求就能看完依赖。否决。数据库抖动会被当成进程死亡，Compose 或以后的编排会重启 api，把故障放大。503 也不会伪装成 401，避免把「库挂了」说成「请重新登录」。

### Decision 8: 不做平台超级管理员

做法：`users.role` 只允许 `teacher` 和 `student`。没有跨班查询分支。

备选：顺手做管理员，方便改账号和看全校材料。否决。管理员天然跨班，本迭代就把 404 当作隔离成立的证据；先放进一个可以合法跨班的角色，这条证据就没了。管理员需要审计和二次确认，另开一次变更。

### Decision 9: 普通数据库账号，管理员口令不下发给 api

做法：MySQL 官方镜像用 root 口令初始化，并创建应用账号。root 口令只出现在 db 服务的环境里。api 只拿到应用账号。宿主不映射 3306。查库用 `docker compose exec` 进入 db 容器。

备选：把 3306 映射出来，用图形客户端更好查。否决作为默认，多开一个口给越权探测。

## 接口一览

| 方法与路径 | 鉴权 | 成功 | 失败 |
| --- | --- | --- | --- |
| `POST /api/login` | 无 | 200 + Set-Cookie | 401 同形；数据库不可用 503 |
| `POST /api/logout` | 会话 | 200 并删除会话 | 无会话 401 |
| `GET /api/me` | 会话 | 200，含 role 与 class | 401 |
| `GET /api/materials` | 会话 | 200，仅本班 | 401；忽略 query 里的 class_id |
| `GET /api/materials/{id}` | 会话 + 班级 | 200，含正文 | 跨班或不存在 404 |
| `GET /api/materials/{id}/file` | 会话 + 班级 | 200 原文件 | 跨班或不存在 404 |
| `POST /api/materials` | 会话 + 教师 | 201 | 学生 403；扩展名或编码 400；超限 413 |
| `GET /health` | 无 | 200 | 不因数据库停止而改变 |

## 数据模型

- `classes(id, name)`：种子 A、B。
- `users(id, username, password_hash, role, class_id)`：`class_id` 非空。
- `sessions(id, user_id, expires_at, created_at)`。
- `materials(id, class_id, uploader_id, title, storage_name, original_ext, byte_size, created_at, seed_key)`：`class_id` 非空并建索引。`seed_key` 只给预置行，用来保证种子幂等。
- `knowledge_entries(id, material_id, class_id, body, created_at)`：`class_id` 非空并建索引，`material_id` 唯一。
- `handouts`、`assignments`、`assistants`、`skills`：占位表。本迭代没有对应路由。

预置材料标题是「A班-函数单调性讲义」和「B班-牛顿定律笔记」。B 班那条由初始化直接写入，不经过上传接口，因为 A 班教师的会话不能把数据写进 B 班。

列表查询带班级条件。按 id 的查询不在 SQL 里提前过滤班级，查出后再比。

## 配置与启动

api 启动时必须读到：`SESSION_SECRET`、`DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME`、`UPLOAD_DIR`、`MAX_UPLOAD_BYTES`、`SESSION_TTL_HOURS`、`SEED_TEACHER_PASSWORD`、`SEED_STUDENT_A_PASSWORD`、`SEED_STUDENT_B_PASSWORD`、`LOGIN_FAIL_THRESHOLD`、`LOGIN_LOCK_MINUTES`、`API_ADDR`。

`.env` 不入库。`.env.example` 列出上述项和只给 db 容器的 `MYSQL_ROOT_PASSWORD`、`WEB_PORT`。`.dockerignore` 排除 `.env` 和 `uploads/`。

api 在数据库尚未接受连接时重试，不把「容器已启动」当成「MySQL 可写」。`depends_on` 只用来排启动顺序。

## Risks / Trade-offs

- 自增 id 可以被枚举。靠 404 同形和会话班级校验挡住，不靠保密 id。
- 限流在内存中，进程重启后计数清零。单实例课堂验收可以接受，多副本不行。
- bcrypt 默认成本让错误口令的响应用时更接近，也会让登录比明文比较慢。
- 正文整段读入内存。上限由 `MAX_UPLOAD_BYTES` 卡住，默认示例是 1 MiB。

## Migration Plan

全新库。api 启动时执行 `CREATE TABLE IF NOT EXISTS` 并按 `seed_key` 和用户名插入缺失种子。没有旧数据要迁移。`docker compose down -v` 会主动清空卷，只在需要重置时使用。

## Open Questions

没有留到实现阶段再选的项。跨班状态码定为 404。`/health` 不查库。技术栈定为 Go、React、MySQL、Nginx。
