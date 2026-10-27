from client import Case, class_of, request, rows, student, teacher


def run(base, env):
    c = Case("GET /api/skills")
    cls = class_of(env, "teacher_a")
    want = rows(env, f"SELECT JSON_OBJECT('id', id, 'name', name, 'enabled', enabled=1) FROM skills WHERE class_id={cls} ORDER BY id")
    for who, token in [("教师", teacher(base, env)), ("学生", student(base, env))]:
        status, body = request(base, "GET", "/api/skills", token=token)
        c.eq(f"{who}返回 200", status, 200)
        got = [{k: s[k] for k in ("id", "name", "enabled")} for s in body.get("skills", [])]
        c.eq(f"{who}看到的技能与库里一致", got, [dict(r, enabled=bool(r["enabled"])) for r in want])
    c.check("本班内置至少两条技能", len(want) >= 2)
    names = [r["name"] for r in want]
    c.check("含有解题引导类技能", any("解题引导" in n for n in names), str(names))
    return c.result()
