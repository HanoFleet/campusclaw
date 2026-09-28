## Context

迭代 1 已经归档在 `openspec/specs/auth-upload`。仓库里有登录、班级隔离、教师上传和 `knowledge_entries` 正文，列表上的搜索只过滤本班标题和正文，不是可追溯检索。本 change 在同一条浏览器 → Nginx → Go → MySQL 链路上增加切片、向量库和问答，不重做认证。

## Goals / Non-Goals

**Goals:**

- 上传或启动补齐后，本班正文变成可检索的切片。
- 关键字、向量、混合三条路径都能判定；默认混合。
- 每条命中能回到材料标题和切片位置。没有依据时不生成。
- 班级只来自会话。Qdrant 和模型网关不出现在宿主端口上。

**Non-Goals:**

- 见 `proposal.md` 的 Non-goals。流式对话、重排序框架和作业批改都不在这一版。

## Decisions

### Decision 1: 正文在 MySQL，向量在 Qdrant

做法：`knowledge_chunks` 存 `chunk_text`、序号、字符区间和 `index_status`。Qdrant 集合 `campusclaw_chunks` 只存向量和标识，主键等于切片 id。检索先命中向量或全文，再回表取摘录。

备选：把 `chunk_text` 写进 Qdrant payload，少一次回表。否决。正文会有两份，关键字检索和摘录都要迁就向量库。只用 MySQL 加自己算的余弦，课堂验收过得去，但和课件要求的向量库对不齐。pgvector 能少一个容器，但当前栈已经是独立 MySQL，继续用 Qdrant 更接近课件。

### Decision 2: 三种模式分开，混合用 RRF

做法：`keyword` 只走 `FULLTEXT (chunk_text) WITH PARSER ngram`，token 长度 2。`vector` 问句嵌入后按班级过滤，余弦低于 0.35 丢掉。`hybrid` 两路先过滤，再按名次做 RRF，`k = 60`。缺的一路不加分。默认 `hybrid`。

备选：只留混合。否决。Qdrant 一停，关键字也不能单独用。把全文分和余弦分加权相加，换嵌入模型就要重标定。交叉编码器重排序更准，延迟高，本课不做。

### Decision 3: 班级只来自会话，跨班检索表现为无命中

做法：认证中间件把会话用户放进上下文。检索和问答只读这个 `class_id`。请求体、query、Header 里的班级解析后丢掉。MySQL 和 Qdrant 都带这个值，回表再核一次。A 班搜 B 班独有词：200，空 `hits`。打开 B 班材料详情仍是 404。

备选：跨班检索返回 403。否决。等于告诉对方别的班有这份资料。按班拆 Qdrant 集合隔离更硬，集合数量会涨，课堂两班没必要。

### Decision 4: 先检索，没有切片就不调对话模型

做法：`POST /api/ask` 对本班做混合检索，最多 4 条。没有切片就返回固定文案，`citations` 为空。有切片才把标题、序号、正文和本轮问句交给对话网关。`[1]` 和 citations 对齐。客户端 `system` 丢掉。不流式。

备选：无依据时仍调模型，让它说不知道。否决。模型可能用训练记忆谈天气。返回 404 会把「没找到」和「材料不存在」混在一起。

### Decision 5: 切分三种策略，重建才换刀

做法：`auto` 800 字、重叠 80 字。`custom` 长度 100–2000、重叠 0–50%。`hierarchy` 按 Markdown 标题分章，过长再套 `auto`。未指定就是 `auto`。种子启动补齐也用 `auto`。重建先删旧切片和旧向量。预处理只作用于切分输入。

备选：按句子或语义边界切，长度不稳定，难验收。只对改过的段落重新嵌入，要先判断哪一段变了，这一版不做。

### Decision 6: 嵌入和对话走服务端网关，密钥不下发

做法：Go 检索模块在入库和向量检索时调嵌入网关，在有切片之后才调对话网关。浏览器只打本站 `/api`。`.env` 增加网关地址和密钥，以及 Qdrant 连接信息。缺这些值时，进程按「检索相关配置缺失」启动失败，不把密钥写进源码。

备选：页面直接调网关。否决。密钥会进浏览器。把 Qdrant 映射到宿主方便调试，客户端就能绕过登录扫向量。

### Decision 7: Compose 增加 qdrant，仍然只暴露 web

做法：`docker-compose.yml` 增加 `qdrant` 服务，不写 `ports`。api 用服务名访问。volume 保存 Qdrant 数据。`down` 再 `up`（不加 `-v`）切片向量还在。

备选：本机另起 Qdrant。不算发布形态。发布仍以 Compose 为准。

## 接口一览

| 方法与路径 | 鉴权 | 成功 | 失败 |
| --- | --- | --- | --- |
| `POST /api/search` | 会话 | 200，含 `mode` 与 `hits` | 空问句 400；未登录 401；`vector`/`hybrid` 且 Qdrant 不可用 503 |
| `POST /api/ask` | 会话 | 200，有切片则回答加 citations；无切片固定文案 | 空问句 400；未登录 401 |
| `POST /api/materials/{id}/reindex` | 会话 + 教师 + 本班 | 200，旧索引删除后重建 | 学生 403；跨班或不存在 404 |

`POST /api/search` 正文：`query`、可选 `mode`（`keyword` \| `vector` \| `hybrid`）。其中的 `class_id` 忽略。

`hits[]` 至少含：`material_id`、`title`、`chunk_index`、`start`、`end`、`excerpt`，以及可选的该路 `score`、`rank`。

## 数据模型

- `knowledge_chunks(id, class_id, material_id, knowledge_entry_id, chunk_index, chunk_text, start_offset, end_offset, index_status, strategy, created_at)`
  - `class_id` 非空并建索引
  - `FULLTEXT (chunk_text) WITH PARSER ngram`
  - `index_status`：`pending` / `ready` / `failed`
- Qdrant point：id = `knowledge_chunks.id`；payload 只有标识，没有正文
- `knowledge_entries.body` 仍是上传原文（课件里的 `body_text` 对应本仓库这个字段）

## 配置与启动

在迭代 1 的变量之外，api 还要读到：`QDRANT_URL`、`EMBEDDING_URL`、`EMBEDDING_API_KEY`、`CHAT_URL`、`CHAT_API_KEY`、`EMBEDDING_MODEL`、`CHAT_MODEL`。`.env.example` 列出这些项，不填真实密钥。

Qdrant 不可达时进程可以起来，以便 `keyword` 单独验收；`vector` 与 `hybrid` 在请求时返回 503。

## Risks / Trade-offs

- 余弦阈值 0.35 和窗口 800 字是课件给定的验收口径，换模型可能要重标定，本 change 不把阈值做成前端可改项。
- 嵌入失败时材料还在，但向量检索看不到它。教师可以重建索引。
- ngram=2 对短英文停用词不友好。本课语料是中文讲义，可以接受。
- 对话网关不可用且已有切片时，问答接口返回 503，不退回固定「未找到」，以免把服务故障说成资料里没有。

## Migration Plan

已有 `knowledge_entries` 行在 api 启动时按 `auto` 补切片。没有旧向量要迁。`docker compose down -v` 会清 MySQL 和 Qdrant 卷。

## Open Questions

没有留到实现再选的项。向量库用 Qdrant。混合用 RRF。跨班检索用空 `hits`。无切片不调对话模型。
