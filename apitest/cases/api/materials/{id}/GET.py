from client import MISSING, Case, class_of, request, rows, student, student_b


def run(base, env):
    c = Case("GET /api/materials/{id}")
    mine = rows(env, f"""SELECT JSON_OBJECT('id', m.id, 'title', m.title, 'class_id', m.class_id, 'body', k.body)
        FROM materials m JOIN knowledge_entries k ON k.material_id=m.id
        WHERE m.class_id={class_of(env, 'student_a1')} ORDER BY m.id LIMIT 1""")[0]
    token = student(base, env)
    status, body = request(base, "GET", f"/api/materials/{mine['id']}", token=token)
    c.eq("本班材料返回 200", status, 200)
    c.eq("标题、班级、正文与库里一致", {k: body.get(k) for k in mine}, mine)

    other = student_b(base, env)
    cross = request(base, "GET", f"/api/materials/{mine['id']}", token=other)
    missing = request(base, "GET", f"/api/materials/{MISSING}", token=other)
    c.eq("他班材料返回 404", cross[0], 404)
    c.eq("他班材料和不存在的材料响应完全一样", cross, missing)
    return c.result()
