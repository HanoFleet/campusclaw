## 1. 项目骨架与配置

- [x] 1.1 建立 `backend/cmd/server` 与 `internal/{config,db,auth,materials,knowledge}`，以及 Vite React TypeScript 前端 — verify: `go test ./...` 与 `npm run build` 通过
- [x] 1.2 配置只从环境变量读取；缺少 `SESSION_SECRET` 或数据库口令时进程退出 — verify: 清空 `SESSION_SECRET` 后启动 api，退出码非 0
- [x] 1.3 提交 `.env.example`、`.gitignore`、`.dockerignore`，排除 `.env` 与 `uploads/` — verify: `git ls-files` 不含 `.env`，且含 `.env.example`

## 2. 数据层与种子

- [x] 2.1 创建班级、用户、会话、材料、知识库，以及讲义、作业、助手、技能表；材料与知识库的 `class_id` 非空并有索引 — verify: 启动后 `SHOW TABLES` 含这些表，`materials.class_id` 为 NOT NULL
- [x] 2.2 幂等写入班级 A/B、`teacher_a`、`student_a1`、`student_b1` 和两班可区分标题 — verify: 重启 api 后用户仍为 3 行，标题不被改写
- [x] 2.3 种子口令来自环境变量并只存 bcrypt — verify: `password_hash` 以 `$2` 开头且不等于明文

## 3. 登录、会话与限流

- [x] 3.1 `POST /api/login` 校验 bcrypt，换发会话并使旧会话失效；Cookie 为 HttpOnly、SameSite=Lax — verify: 登录响应含 `HttpOnly` 与 `SameSite=Lax`
- [x] 3.2 未登录访问材料接口返回 401，响应体不含材料标题或正文 — verify: 无 Cookie 的 `GET /api/materials` 为 401，正文不含「函数单调性」「牛顿定律」
- [x] 3.3 `POST /api/logout` 删除会话行；旧 Cookie 再请求得到 401 — verify: 登出后用原 Cookie 调用 `/api/me` 为 401
- [x] 3.4 不存在账号、错误口令、锁定期的响应体一致；用户不存在时仍做 bcrypt 比较 — verify: 两种登录失败的状态码和响应体相同
- [x] 3.5 `GET /api/me` 返回角色与班级 — verify: `teacher_a` 的 `role` 为 `teacher`，`class_name` 为 `A`

## 4. 班级隔离

- [x] 4.1 列表只按会话班级过滤，忽略请求中的 `class_id` — verify: A 班请求 `?class_id=` B 班 id 时仍看不到 B 班标题
- [x] 4.2 详情和文件先取行再核对班级；跨班与不存在 id 的 404 响应体相同 — verify: 比较两次 404 的响应体
- [x] 4.3 上传表单中的 `class_id` 不改变落库班级 — verify: A 班教师带 B 班字段上传后，详情里的 `class_id` 仍是 A

## 5. 角色授权与上传入库

- [x] 5.1 学生 `POST /api/materials` 返回 403，表行数与上传目录不变 — verify: 学生上传前后材料行数相同
- [x] 5.2 教师上传 `.txt`/`.md` 后两表各增一行，列表与详情能读到正文 — verify: 201 之后详情正文等于文件内容
- [x] 5.3 非白名单扩展名、空文件、非 UTF-8 返回 4xx，超限返回 413，且无残留 — verify: `.exe` 得到 400，材料行数不变
- [x] 5.4 下载走鉴权接口；`/uploads/` 不返回文件 — verify: `GET /uploads/<name>` 不是 200

## 6. 材料读取 API

- [x] 6.1 详情返回标题、班级、时间和正文，不返回存储路径 — verify: 详情 JSON 没有 `storage_name` 和 `/uploads/`
- [x] 6.2 本班学生可查看和下载，其他班按 id 访问为 404 — verify: `student_a1` 下载成功，`student_b1` 请求同一 id 为 404

## 7. 前端页面

- [x] 7.1 登录页提交 `/api/login`；失败只显示统一错误 — verify: 页面不区分账号不存在和口令错误
- [x] 7.2 刷新时用 `/api/me` 恢复身份；401 回登录页；登出调用 `/api/logout` — verify: 刷新材料页会请求 `/api/me`
- [x] 7.3 上传入口只在 `/api/me` 的 role 为 teacher 时出现 — verify: 学生页面没有上传控件
- [x] 7.4 列表、详情、下载走鉴权 API；Markdown 使用 GFM 且不执行脚本 — verify: 含 `<script>` 的材料不会以 HTML 执行
- [x] 7.5 北大红色、深浅色、列表/网格、本班搜索、命令面板、上传进度和结果提示 — verify: 材料页具备这些控件，搜索「牛顿定律」在 A 班无结果

## 8. Docker Compose 与文档

- [x] 8.1 web、api、db 三服务；只有 web 映射宿主端口；上传目录和数据库使用 volume — verify: `docker compose config` 里 db 和 api 没有宿主端口
- [x] 8.2 api 用普通数据库账号；root 口令不进入 api 容器 — verify: `docker compose exec api printenv MYSQL_ROOT_PASSWORD` 为空
- [x] 8.3 api 在数据库可连接前重试 — verify: 冷启动后最终 `/health` 为 200 且可以登录
- [x] 8.4 `GET /health` 不查库；数据库停止时业务接口为 503 — verify: 停止 db 后 `/health` 仍为 200，`/api/materials` 为 503
- [x] 8.5 README 写明范围、不做项、单实例、跨班 404、启动步骤、预置账号和 health 地址 — verify: 只读 README 能回答这四项
- [x] 8.6 `down` 后再 `up`（不加 `-v`）上传的材料仍在 — verify: 往返重启后列表仍含该标题

## 9. 发布验收

- [x] 9.1 主路径留证：登录、上传、列表可见、学生只读 — verify: `python3 scripts/verify.py` 对应断言通过
- [x] 9.2 失败路径留证：未登录、学生上传 403、跨班 404 — verify: 同上，至少这三条有命令输出
- [x] 9.3 `openspec validate add-auth-rbac-class-knowledge --strict` 退出码为 0 — verify: 该命令
- [x] 9.4 用自己的话写清四项决策：班级来源、404、上传事务、`/health` 不查库 — verify: `design.md` 的 Decision 3、6、7 与 README 一致
- [x] 9.5 归档 change，主力规约出现 `auth-upload`，活动变更清零 — verify: `openspec list` 不再列出本 change，`openspec list --specs` 含 `auth-upload`
