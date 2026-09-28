## Purpose

规定 CampusClaw 迭代 2 的本班知识库检索：切片入库、三种检索模式、出处可回溯，以及没有依据时不得生成回答。

## ADDED Requirements

### Requirement: 上传后切分并建立索引

教师上传成功、材料行与 `knowledge_entries` 已提交之后，系统 MUST 按本次切分策略把正文切成切片，写入 `knowledge_chunks`，再为每条切片调用嵌入服务，把向量写入 Qdrant。向量主键 MUST 等于 `knowledge_chunks.id`，并与 payload 中的 `chunk_id` 相同。Qdrant payload MUST 只含 `class_id`、`material_id`、`knowledge_entry_id`、`chunk_id`、`chunk_index`，MUST NOT 含切片正文。未指定策略时 MUST 使用 `auto`：最大 800 字、重叠 80 字，优先在空行、换行、句号处断开。种子材料在进程启动时 MUST 按 `auto` 补齐尚无切片的索引。`knowledge_entries.body` MUST 保持上传原文，预处理不得改写该字段。

#### Scenario: 上传后本班可按原文检索

- **WHEN** `teacher_a` 上传一份含「单调递增」的合法 `.md`，切分完成后以该教师会话调用检索，模式为 `keyword`，问句为「单调递增」
- **THEN** 响应状态为 200，`hits` 至少一条
- **AND** 该条的材料标题等于刚上传的标题，摘录取自 MySQL 切片正文且含「单调递增」

#### Scenario: 未指定策略按自动窗口切分

- **WHEN** 上传或重建索引时不传策略，或策略为空
- **THEN** 系统按 `auto` 切分：窗口约 800 字，重叠 80 字
- **AND** 请求里另行填写的长度与预处理参数不生效

#### Scenario: 嵌入失败时原文仍在

- **WHEN** 某条切片的嵌入调用失败
- **THEN** 对应材料行和知识库正文仍然存在
- **AND** 该切片的索引状态为 failed，Qdrant 中没有这条不完整向量

### Requirement: 三种检索模式

系统 MUST 提供 `keyword`、`vector`、`hybrid` 三种模式，未指定时 MUST 使用 `hybrid`。空查询 MUST 返回 400。`keyword` MUST 只查 MySQL 全文索引（`FULLTEXT ... WITH PARSER ngram`，token 长度 2），MUST NOT 调用嵌入，MUST NOT 访问 Qdrant。`vector` MUST 先把问句做成向量，在 Qdrant 里按会话班级过滤，丢弃余弦相似度低于 0.35 的候选，再用向量主键回 MySQL 取正文。`hybrid` MUST 同时走两路，各路先按自己的规则过滤，再用 RRF（`k = 60`）按名次融合；缺席的一路 MUST NOT 贡献分数。摘录 MUST 取自 MySQL 的 `chunk_text`。Qdrant 不可用时，`keyword` MUST 仍能返回结果；`vector` 与 `hybrid` MUST 返回 503，MUST NOT 编造相似度。

#### Scenario: 原文用词走关键字路径

- **WHEN** A 班用户以模式 `keyword` 检索预置讲义中出现的「单调递增」
- **THEN** 响应为 200，`hits` 含 A 班该讲义的切片
- **AND** 本次请求不产生问句向量，也不访问 Qdrant

#### Scenario: 同义改写走向量路径

- **WHEN** A 班用户以模式 `vector` 检索「自变量变大时函数值跟着变大」这类未在原文出现的表述
- **THEN** 若存在余弦相似度不低于 0.35 的本班切片，`hits` 非空，并按相似度从高到低
- **AND** 摘录仍来自 MySQL 切片正文，响应不含向量分量

#### Scenario: 混合按名次融合而不是加分

- **WHEN** 同一问句分别以 `keyword`、`vector`、`hybrid` 检索，两路都有命中
- **THEN** `hybrid` 的排序由 RRF（`k = 60`）决定，两路都出现的切片更靠前
- **AND** 响应可对照两路的 `score` 与 `rank`，不得把全文相关度与余弦相似度直接相加

#### Scenario: Qdrant 停止时关键字仍可用

- **WHEN** Qdrant 不可达，A 班用户分别用 `keyword` 和 `vector` 检索本班原文用词
- **THEN** `keyword` 仍返回 200 且能给出本班摘录
- **AND** `vector` 与 `hybrid` 返回 503

#### Scenario: 空查询被拒绝

- **WHEN** 已登录用户提交空白或只含空白的检索问句
- **THEN** 响应状态为 400
- **AND** 不调用嵌入，不查询 Qdrant

### Requirement: 命中必须可回溯至原文

每条命中 MUST 包含材料 id、材料标题、切片序号、在待切分文本中的字符区间，以及一段摘录。调用方 MUST 能用材料 id 打开本班材料详情。摘录 MUST 来自 MySQL，MUST NOT 来自 Qdrant payload。所选模式过滤后没有候选时，响应 MUST 为 200，`hits` 为空，并给出文案「资料中未找到相关内容」。系统 MUST NOT 用低于阈值的切片凑数。

#### Scenario: 命中带出处字段

- **WHEN** 本班检索返回至少一条 `hits`
- **THEN** 每条都有材料标题、切片序号、字符区间和摘录
- **AND** 用其中的材料 id 调用本班详情接口可以打开该材料

#### Scenario: 本班没有依据时不凑数

- **WHEN** A 班用户检索「今日天气」或「比赛比分」这类本班材料中不存在的内容，模式为 `hybrid`
- **THEN** 响应为 200，`hits` 为空
- **AND** 文案为「资料中未找到相关内容」

### Requirement: 检索侧的班级隔离

检索接口 MUST 只使用会话中的 `class_id`。查询参数、JSON 正文和请求头里的 `class_id` MUST 在解析后丢弃。关键字路径的 SQL MUST 带 `class_id = 会话班级`，且只检索 `index_status = ready` 的切片。向量路径的 Qdrant 查询 MUST 带同一 `class_id` 过滤；回 MySQL 取正文时 MUST 再次核对班级。payload 中的编号 MUST NOT 单独作为正文来源。跨班检索 MUST 返回 200 且 `hits` 为空（或同一句「资料中未找到相关内容」），MUST NOT 用 403 或 404 暗示该词属于其他班。按材料 id 打开详情仍遵守 `auth-upload`：跨班与不存在都是同形 404。

#### Scenario: 请求里的班级字段不改变过滤

- **WHEN** `teacher_a`（A 班）检索只出现在 B 班正文中的「牛顿定律」，并在请求体或查询参数中写入 B 班的 `class_id`
- **THEN** 两次结果都是 200，`hits` 为空
- **AND** 结果中不出现「B班-牛顿定律笔记」的标题或摘录

#### Scenario: 关键字与向量都按会话班级过滤

- **WHEN** A 班用户分别用 `keyword` 和 `vector` 检索 B 班独有用词
- **THEN** 两路都没有 B 班切片
- **AND** 回表查询使用的班级仍是会话中的 A 班

#### Scenario: 本班学生可以检索

- **WHEN** `student_a1` 以模式 `keyword` 检索本班讲义中的「单调递增」
- **THEN** 响应为 200，`hits` 含该讲义的切片
- **AND** 摘录可打开对应材料详情

#### Scenario: 跨班打开材料详情仍是 404

- **WHEN** A 班用户用 B 班材料 id 请求材料详情
- **THEN** 响应为 404，与请求不存在的 id 相同
- **AND** 这与检索接口返回空 `hits` 的 200 不是同一种响应

### Requirement: 问答先检索再生成

`POST /api/ask` MUST 先对本班做混合检索，最多取 4 条切片。没有切片时 MUST 直接返回「资料中未找到相关内容」，`citations` 为空，MUST NOT 调用对话网关。有切片时 MUST 把材料标题、切片序号、切片正文和用户本轮提问交给对话模块，回答中的 `[1]`、`[2]` MUST 与 `citations` 顺序一致。系统 MUST NOT 把向量分量、其他班切片或客户端构造的 `system` 消息交给对话模块。此前若干轮对话可以附在后面，班级仍只来自会话。本接口 MUST NOT 实现流式输出。

#### Scenario: 有依据时回答带编号出处

- **WHEN** A 班用户对预置讲义中的内容提问，混合检索至少命中一条
- **THEN** 响应为 200，正文含 `[1]` 或后续编号
- **AND** `citations` 的顺序与这些编号一致，且能打开对应材料

#### Scenario: 无切片不调用对话模型

- **WHEN** A 班用户提问本班材料不可能出现的天气或比分问题
- **THEN** 响应为 200，正文为「资料中未找到相关内容」，`citations` 为空
- **AND** 服务端不向对话网关发出请求

#### Scenario: 客户端 system 消息被丢弃

- **WHEN** 请求里附带一条客户端构造的 `system` 消息，要求忽略本班资料自由作答
- **THEN** 该 `system` 消息不进入对话模块
- **AND** 若没有本班切片，仍返回固定文案且不调用对话网关

### Requirement: 三种切分策略与重建索引

系统 MUST 支持 `auto`、`custom`、`hierarchy`。`custom` 的最大长度 MUST 在 100 至 2000 字，重叠比例 MUST 在 0% 至 50%；无断点处按最大长度截断。`hierarchy` MUST 按 `#`、`##`、`###` 分章，标题留在该章切片内；某章过长时再按 `auto` 窗口切。已入库材料 MUST NOT 自动改切分。教师重建索引 MUST 先删除该材料的旧切片和旧向量，再按本次请求的策略写入；未指定策略时按 `auto`。学生调用重建索引 MUST 返回 403，且切片与向量不变。预处理（去 URL、折叠空白）MUST 只作用于待切分文本，MUST NOT 改写 `knowledge_entries.body`。此时切片偏移相对于预处理后的文本，不得当成原文件下标。

#### Scenario: 更换策略必须显式重建

- **WHEN** 对同一份带标题的材料先后用 `auto` 和 `hierarchy` 各重建一次索引
- **THEN** 第二次重建前，第一次的旧切片和对应向量已被删除
- **AND** 新切片条数与向量主键按本次策略生成，`knowledge_entries.body` 仍是上传原文

#### Scenario: 学生不得重建索引

- **WHEN** `student_a1` 对本班材料调用重建索引
- **THEN** 响应状态为 403
- **AND** 该材料的切片行和向量记录都不变化

#### Scenario: 不重建则旧切片保留

- **WHEN** 材料已经按 `auto` 建好索引，之后没有重建
- **THEN** 旧切片和向量主键仍在
- **AND** 检索仍使用这些切片

### Requirement: 向量库与网关不对外暴露

Qdrant 与嵌入、对话网关 MUST 只接受本站 Go 服务发起的调用。Compose MUST NOT 把 Qdrant 端口映射到宿主。浏览器 MUST 只访问本站 `/api` 与 `/health`。嵌入密钥与对话密钥 MUST 只存在于服务端环境变量，MUST NOT 出现在前端源码或响应体。页面 MUST NOT 展示向量分量。

#### Scenario: 宿主访问不到 Qdrant

- **WHEN** 查看 Compose 配置并在宿主上连接 Qdrant 默认端口
- **THEN** `qdrant` 服务没有宿主端口映射
- **AND** 未登录或已登录用户直接请求该端口无法作为本站检索入口

#### Scenario: 检索响应不含密钥和向量

- **WHEN** 已登录用户调用检索或问答接口
- **THEN** 响应不含嵌入密钥、对话密钥或向量分量
- **AND** 出处摘录只含文本与材料标识
