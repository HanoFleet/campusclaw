from client import Case, one, student, teacher, upload


def run(base, env):
    c = Case("POST /api/materials")
    before = one(env, "SELECT COUNT(*) FROM materials")
    status, body = upload(base, "/api/materials", student(base, env), "笔记.md", "# 学生上传\n内容".encode())
    c.eq("学生上传返回 403", status, 403)
    c.eq("错误码 forbidden", body.get("error"), "forbidden")
    token = teacher(base, env)
    status, body = upload(base, "/api/materials", token, "脚本.exe", b"MZ\x90\x00")
    c.eq("不支持的类型返回 400", status, 400)
    c.eq("错误码 invalid_file", body.get("error"), "invalid_file")
    status, body = upload(base, "/api/materials", token, "乱码.md", b"\xff\xfe\x00bad")
    c.eq("不是 UTF-8 的文件返回 400", status, 400)
    c.eq("被拒的上传都没有入库", one(env, "SELECT COUNT(*) FROM materials"), before)
    return c.result()
