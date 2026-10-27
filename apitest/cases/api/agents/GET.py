from client import Case, class_of, request, rows, student, teacher


def run(base, env):
    c = Case("GET /api/agents")
    cls = class_of(env, "teacher_a")
    query = "SELECT JSON_OBJECT('id', id, 'name', name, 'prompt', prompt, 'enabled', enabled=1) FROM tutor_agents WHERE class_id={} {} ORDER BY id"
    for who, token, extra in [("教师", teacher(base, env), ""), ("学生", student(base, env), "AND enabled=1")]:
        status, body = request(base, "GET", "/api/agents", token=token)
        c.eq(f"{who}返回 200", status, 200)
        got = [{k: a[k] for k in ("id", "name", "prompt", "enabled")} for a in body.get("agents", [])]
        want = [dict(r, enabled=bool(r["enabled"])) for r in rows(env, query.format(cls, extra))]
        c.eq(f"{who}看到的应用和提示词与库里一致" + ("（只含已启用）" if extra else "（含停用）"), got, want)
    c.check("内置至少两个应用", len(rows(env, query.format(cls, "AND enabled=1"))) >= 2)
    return c.result()
