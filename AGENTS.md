# 项目规则

没有写进 OpenSpec 的行为，就不实现。

- 开工前先读 `openspec/specs/` 与仍在活动中的 `openspec/changes/<change>/`。归档之后，长期事实来源是 `openspec/specs/`。
- 规约含糊或没写「不做」时，先改 change 里的 delta（`proposal.md`、`design.md`、`specs/<capability>/spec.md`），再改代码。不要直接改已经归档的主力规约来迁就一次实现。
- 访问控制放在 Go 服务端。前端不显示按钮，不算隔离或授权。
- 密钥、`.env`、上传文件和数据库文件不进 Git。仓库里只保留 `.env.example`。
- 一个 task 做完并跑过该条 verify，再勾选 `tasks.md`。
