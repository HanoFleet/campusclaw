## MODIFIED Requirements

### Requirement: 材料上传与知识库入库

教师上传成功时，系统 MUST 在校验通过后把原文件写入上传目录，并在同一数据库事务中插入 `materials` 与 `knowledge_entries`。该事务提交之后，系统 MUST 按切分策略把正文写入 `knowledge_chunks` 并建立向量索引，规则见 `knowledge-retrieval`。扩展名 MUST 使用白名单 `.txt` 与 `.md`。超过配置上限 MUST 返回 413。空文件或非 UTF-8 MUST 返回 400。上传事务失败时 MUST NOT 留下材料行、知识库行或未关联文件。存储名 MUST 由服务端生成。

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

#### Scenario: 上传成功后本班可检索正文

- **WHEN** 教师上传成功且切片索引状态为 ready
- **THEN** 本班用户用原文中的词做 `keyword` 检索能得到该材料的命中
- **AND** 其他班用户用同一词检索时 `hits` 不含该材料
