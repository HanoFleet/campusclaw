## 1. 规约与配置

- [x] 1.1 保持本 change 四件套可被 `openspec validate add-traceable-vector-retrieval --strict` 通过 — verify: 该命令退出码为 0
- [x] 1.2 `.env.example` 列出 `QDRANT_URL`、嵌入与对话网关地址和密钥占位，不含真实值 — verify: 文件存在这些键，值为空或占位

## 2. 切片表与切分

- [x] 2.1 新增 `knowledge_chunks`，`class_id` 非空并有索引，`chunk_text` 有 ngram 全文索引 — verify: 启动后 `SHOW CREATE TABLE knowledge_chunks` 含 FULLTEXT 与 ngram
- [x] 2.2 实现 `auto` / `custom` / `hierarchy`；未指定按 `auto`（800 字、重叠 80 字） — verify: 对同一带标题正文三种策略得到不同切片条数，且 `auto` 未指定时复现
- [x] 2.3 预处理不改写 `knowledge_entries` 正文 — verify: 折叠空白后查库，`body` 仍是上传原文

## 3. 入库与 Qdrant

- [x] 3.1 上传事务提交后切分、嵌入并写入 Qdrant，向量主键等于切片 id — verify: 取一条 ready 切片，Qdrant point id 与 `knowledge_chunks.id` 相同，payload 无正文
- [x] 3.2 种子材料启动时按 `auto` 补齐索引 — verify: 冷启动后两班预置材料都有 ready 切片
- [x] 3.3 嵌入失败标记 failed，不写残缺向量 — verify: 断开嵌入网关后上传，材料行仍在，Qdrant 无新点
- [x] 3.4 教师重建索引先删旧切片和旧向量 — verify: 连续两次 reindex，切片 id 集合与第一次不完全相同，旧 id 在 Qdrant 中不存在

## 4. 三种检索

- [x] 4.1 `POST /api/search` 支持 `keyword` / `vector` / `hybrid`，默认 `hybrid`；空查询 400 — verify: 不传 mode 时响应 `mode` 为 hybrid；空字符串返回 400
- [x] 4.2 `keyword` 只查 MySQL 全文，不调嵌入、不访问 Qdrant — verify: 停 Qdrant 后 keyword 仍 200 且命中本班原文用词
- [x] 4.3 `vector` 按班级过滤，余弦低于 0.35 丢弃，回表取摘录 — verify: 同义改写有命中时 excerpt 来自 MySQL；跨班向量检索 hits 为空
- [x] 4.4 `hybrid` 用 RRF（k=60），缺席一路不加分 — verify: 同时命中的切片排在只中一路的前面
- [x] 4.5 Qdrant 不可用时 `vector` 与 `hybrid` 返回 503 — verify: 停 Qdrant 后这两模式为 503

## 5. 出处与班级隔离

- [x] 5.1 每条 hit 含材料 id、标题、切片序号、区间和摘录 — verify: 任一条 hit 都能用 material_id 打开本班详情
- [x] 5.2 无候选时 200、空 hits、文案「资料中未找到相关内容」 — verify: 问天气得到该文案且 hits 为 []
- [x] 5.3 请求中的 `class_id` 不影响过滤 — verify: A 班带 B 班 class_id 搜「牛顿定律」，hits 为空
- [x] 5.4 学生可以检索、不能重建索引 — verify: `student_a1` 检索 200；`POST .../reindex` 为 403

## 6. 问答

- [x] 6.1 `POST /api/ask` 先混合检索最多 4 条；无切片不调对话网关 — verify: 问比分返回固定文案，citations 为 []，网关访问计数不增加
- [x] 6.2 有切片时回答中的 [1] 与 citations 顺序一致 — verify: 对预置讲义提问，citations[0] 对应 [1]
- [x] 6.3 丢弃客户端 system 消息 — verify: 请求夹带 system 仍不能在无切片时得到编造回答

## 7. Compose、页面与文档

- [x] 7.1 Compose 增加 qdrant，不映射宿主端口 — verify: `docker compose config` 里 qdrant 无 ports
- [x] 7.2 材料页提供本班检索；身份仍以 `/api/me` 为准 — verify: A 班搜「牛顿定律」页面无 B 班标题
- [x] 7.3 README 写明三种模式、跨班空 hits、Qdrant 不对外、无依据不生成 — verify: 只读 README 能回答这四项

## 8. 校验

- [x] 8.1 `openspec validate add-traceable-vector-retrieval --strict` 退出码为 0 — verify: 该命令
- [x] 8.2 实现完成后归档，主力规约出现 `knowledge-retrieval` — verify: `openspec list --specs` 含该能力
