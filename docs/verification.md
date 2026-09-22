# 验收记录

目标：`http://127.0.0.1:8080`

## 启动与健康检查
PASS GET /health 200 {"status":"ok"}
PASS 未登录 GET /api/materials 401 且不含材料标题
PASS GET /uploads/secret.md -> 404
## 登录失败同形
PASS 不存在账号与错误口令同为 401 {"error":"invalid_credentials"}
## 主路径
PASS teacher_a role=teacher class=A
PASS A 班搜索「牛顿定律」且带 class_id 参数时没有 B 班标题
PASS 教师上传 201 id=4
PASS 详情正文与文件一致，班级仍是会话班级，且没有存储路径
PASS 本班列表能看到新上传和预置讲义
PASS 同班学生可以下载
PASS 学生上传 403，材料行数保持 4
PASS 跨班详情与不存在 id 的 404 响应体相同
PASS B 班看不到 A 班材料，能看到自己的预置笔记
PASS notes.md.exe 返回 400，材料行数不变
## 种子与密钥
PASS 三个预置账号的 password_hash 都是 bcrypt，且不等于明文
PASS 九张表存在，materials.class_id 为 NOT NULL
PASS api 容器没有 MYSQL_ROOT_PASSWORD
## 登出
PASS 登出后旧 Cookie 得到 401
## 数据库停止时 /health 与业务接口分离
PASS 停库后 /health 200，/api/materials 503
PASS 数据库恢复后可以重新登录
## 限流与口令错误同形
PASS 锁定期内正确口令仍返回与错误口令相同的 401
PASS 重启 api 以清掉内存中的限流计数
## 重建容器后数据仍在
PASS down/up 后仍能看到 验收-上传-1790096811.md，用户数仍为 3
PASS 前端使用 ReactMarkdown，没有 rehype-raw 或 dangerouslySetInnerHTML

全部断言通过。
