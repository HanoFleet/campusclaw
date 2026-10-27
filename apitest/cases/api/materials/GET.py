from client import Case, class_of, request, rows, student, student_b


def run(base, env):
    c = Case("GET /api/materials")
    for name, token in [("student_a1", student(base, env)), ("student_b1", student_b(base, env))]:
        status, body = request(base, "GET", "/api/materials", token=token)
        c.eq(f"{name} 返回 200", status, 200)
        got = [{"id": m["id"], "title": m["title"]} for m in body.get("materials", [])]
        want = rows(env, f"""SELECT JSON_OBJECT('id', id, 'title', title) FROM materials
            WHERE class_id={class_of(env, name)} ORDER BY created_at DESC, id DESC""")
        c.eq(f"{name} 看到的正好是本班材料，顺序也一致", got, want)

    token = student(base, env)
    status, body = request(base, "GET", "/api/materials?q=%E6%95%B0%E5%AD%A6", token=token)
    got = [m["id"] for m in body.get("materials", [])]
    want = [r["id"] for r in rows(env, f"""SELECT JSON_OBJECT('id', m.id) FROM materials m
        JOIN knowledge_entries k ON k.material_id=m.id
        WHERE m.class_id={class_of(env, 'student_a1')} AND (m.title LIKE '%数学%' OR k.body LIKE '%数学%')
        ORDER BY m.created_at DESC, m.id DESC""")]
    c.eq("按「数学」筛选的结果和库里一致", got, want)
    status, _ = request(base, "GET", "/api/materials")
    c.eq("不登录返回 401", status, 401)
    return c.result()
